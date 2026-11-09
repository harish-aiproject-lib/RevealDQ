package handlers

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/golang-jwt/jwt/v5"
	"github.com/harish-aiproject-lib/AgenticDataGov/backend/internal/config"
	"github.com/harish-aiproject-lib/AgenticDataGov/backend/internal/middleware"
	"github.com/harish-aiproject-lib/AgenticDataGov/backend/internal/secretbox"
	"golang.org/x/crypto/bcrypt"
)

type Handler struct {
	ctrl      *sql.DB
	src       *sql.DB
	cfg       config.Config
	box       *secretbox.Box
	handoffMu sync.Mutex
	handoffs  map[string]connectionHandoff
}

func New(ctrl, src *sql.DB, cfg config.Config) *Handler {
	box, err := secretbox.New(cfg.DataKey, cfg.JWTSecret)
	if err != nil {
		log.Fatalf("secretbox: %v", err)
	}
	return &Handler{ctrl: ctrl, src: src, cfg: cfg, box: box, handoffs: map[string]connectionHandoff{}}
}

func writeJSON(w http.ResponseWriter, code int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

func writeErr(w http.ResponseWriter, code int, msg string) {
	writeJSON(w, code, map[string]string{"error": msg})
}

func wsID(r *http.Request) (int64, error) {
	return strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
}

// ---------------- Auth ----------------

type authReq struct {
	Email       string `json:"email"`
	Password    string `json:"password"`
	DisplayName string `json:"displayName"`
}

func (h *Handler) Register(w http.ResponseWriter, r *http.Request) {
	var req authReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, 400, "invalid body")
		return
	}
	if req.Email == "" || req.Password == "" {
		writeErr(w, 400, "email and password required")
		return
	}
	if req.DisplayName == "" {
		req.DisplayName = strings.Split(req.Email, "@")[0]
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		writeErr(w, 500, "hash failed")
		return
	}
	res, err := h.ctrl.Exec(`INSERT INTO users (email, password_hash, display_name) VALUES (?,?,?)`,
		req.Email, string(hash), req.DisplayName)
	if err != nil {
		writeErr(w, 409, "email already registered")
		return
	}
	uid, _ := res.LastInsertId()
	writeJSON(w, 201, map[string]interface{}{"userId": uid, "email": req.Email})
}

func (h *Handler) Login(w http.ResponseWriter, r *http.Request) {
	var req authReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, 400, "invalid body")
		return
	}
	var uid int64
	var hash, name string
	err := h.ctrl.QueryRow(`SELECT user_id, password_hash, display_name FROM users WHERE email=? AND is_active=1`,
		req.Email).Scan(&uid, &hash, &name)
	if err != nil {
		writeErr(w, 401, "invalid credentials")
		return
	}
	if bcrypt.CompareHashAndPassword([]byte(hash), []byte(req.Password)) != nil {
		writeErr(w, 401, "invalid credentials")
		return
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"sub":   uid,
		"email": req.Email,
		"exp":   time.Now().Add(24 * time.Hour).Unix(),
	})
	signed, err := token.SignedString([]byte(h.cfg.JWTSecret))
	if err != nil {
		writeErr(w, 500, "token failed")
		return
	}
	writeJSON(w, 200, map[string]interface{}{
		"token":       signed,
		"userId":      uid,
		"email":       req.Email,
		"displayName": name,
	})
}

func (h *Handler) Me(w http.ResponseWriter, r *http.Request) {
	uid := middleware.UserID(r.Context())
	var email, name string
	err := h.ctrl.QueryRow(`SELECT email, display_name FROM users WHERE user_id=?`, uid).Scan(&email, &name)
	if err != nil {
		writeErr(w, 404, "user not found")
		return
	}
	writeJSON(w, 200, map[string]interface{}{"userId": uid, "email": email, "displayName": name})
}

// ---------------- Product modules (Reality Reveal platform) ----------------

type productModule struct {
	Code      string `json:"productCode"`
	Name      string `json:"productName"`
	ShortName string `json:"shortName"`
	Tagline   string `json:"tagline"`
	Status    string `json:"status"` // ACTIVE | PREVIEW | COMING_SOON
	SortOrder int    `json:"sortOrder"`
}

func (h *Handler) loadProducts() ([]productModule, error) {
	rows, err := h.ctrl.Query(`
		SELECT product_code, product_name, short_name, COALESCE(tagline,''), status, sort_order
		FROM product_modules ORDER BY sort_order, product_code`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	list := []productModule{}
	for rows.Next() {
		var p productModule
		if err := rows.Scan(&p.Code, &p.Name, &p.ShortName, &p.Tagline, &p.Status, &p.SortOrder); err == nil {
			list = append(list, p)
		}
	}
	return list, nil
}

// ListProducts returns the product-module catalog (Reveal DQ, Reveal VoC, ...).
func (h *Handler) ListProducts(w http.ResponseWriter, r *http.Request) {
	list, err := h.loadProducts()
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, list)
}

// resolveProduct validates a requested product code against the catalog.
// Empty defaults to DQ (the original product) so older clients keep working.
func (h *Handler) resolveProduct(code string) (productModule, error) {
	code = strings.ToUpper(strings.TrimSpace(code))
	if code == "" {
		code = "DQ"
	}
	list, err := h.loadProducts()
	if err != nil {
		return productModule{}, err
	}
	for _, p := range list {
		if p.Code == code {
			if p.Status == "COMING_SOON" {
				return p, fmt.Errorf("product %s is not yet available", p.Name)
			}
			return p, nil
		}
	}
	return productModule{}, fmt.Errorf("unknown product module: %s", code)
}

// ---------------- Workspaces ----------------

func (h *Handler) ListWorkspaces(w http.ResponseWriter, r *http.Request) {
	uid := middleware.UserID(r.Context())
	rows, err := h.ctrl.Query(`
		SELECT w.workspace_id, w.workspace_name, w.customer_name, w.description, w.created_at, r.role_code,
		       w.product_code, COALESCE(pm.product_name, w.product_code), COALESCE(pm.status, 'ACTIVE')
		FROM workspaces w
		JOIN user_workspace_roles uwr ON uwr.workspace_id = w.workspace_id
		JOIN roles r ON r.role_id = uwr.role_id
		LEFT JOIN product_modules pm ON pm.product_code = w.product_code
		WHERE uwr.user_id = ?
		ORDER BY w.created_at DESC`, uid)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	defer rows.Close()
	list := []map[string]interface{}{}
	for rows.Next() {
		var id int64
		var name, customer, desc, role, pcode, pname, pstatus string
		var created time.Time
		_ = rows.Scan(&id, &name, &customer, &desc, &created, &role, &pcode, &pname, &pstatus)
		list = append(list, map[string]interface{}{
			"workspaceId": id, "workspaceName": name, "customerName": customer,
			"description": desc, "createdAt": created, "role": role,
			"productCode": pcode, "productName": pname, "productStatus": pstatus,
		})
	}
	writeJSON(w, 200, list)
}

func (h *Handler) CreateWorkspace(w http.ResponseWriter, r *http.Request) {
	uid := middleware.UserID(r.Context())
	var body struct {
		WorkspaceName string `json:"workspaceName"`
		CustomerName  string `json:"customerName"`
		Description   string `json:"description"`
		ProductCode   string `json:"productCode"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.WorkspaceName == "" {
		writeErr(w, 400, "workspaceName required")
		return
	}
	if body.CustomerName == "" {
		body.CustomerName = body.WorkspaceName
	}
	product, err := h.resolveProduct(body.ProductCode)
	if err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	res, err := h.ctrl.Exec(`
		INSERT INTO workspaces (workspace_name, customer_name, description, product_code, created_by)
		VALUES (?,?,?,?,?)`,
		body.WorkspaceName, body.CustomerName, body.Description, product.Code, uid)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	wsID, _ := res.LastInsertId()
	var roleID int64
	_ = h.ctrl.QueryRow(`SELECT role_id FROM roles WHERE role_code='ADMIN'`).Scan(&roleID)
	_, _ = h.ctrl.Exec(`INSERT INTO user_workspace_roles (user_id, workspace_id, role_id) VALUES (?,?,?)`, uid, wsID, roleID)

	// Product-specific bootstrap. Reveal DQ seeds its sample warehouse jobs; other
	// modules start empty until their own bootstrap is defined.
	if product.Code == "DQ" {
		procs := []struct{ n, p, d string }{
			{"CDR Unrated Completeness", "sp_dq_cdr_unrated", "Detects CDRs without rated_usage"},
			{"Invoice Balance Consistency", "sp_dq_invoice_balance", "Invoice total vs line items"},
			{"MSISDN Validity", "sp_dq_msisdn_validity", "Invalid service identifiers"},
		}
		for _, p := range procs {
			_, _ = h.ctrl.Exec(`INSERT INTO dq_pipelines (workspace_id, pipeline_name, proc_name, description) VALUES (?,?,?,?)`,
				wsID, p.n, p.p, p.d)
		}
	}
	writeJSON(w, 201, map[string]interface{}{
		"workspaceId": wsID, "workspaceName": body.WorkspaceName,
		"productCode": product.Code, "productName": product.Name,
	})
}

func (h *Handler) GetWorkspace(w http.ResponseWriter, r *http.Request) {
	id, err := wsID(r)
	if err != nil {
		writeErr(w, 400, "bad id")
		return
	}
	var name, customer, desc, pcode, pname, pstatus string
	var created time.Time
	err = h.ctrl.QueryRow(`
		SELECT w.workspace_name, w.customer_name, w.description, w.created_at,
		       w.product_code, COALESCE(pm.product_name, w.product_code), COALESCE(pm.status, 'ACTIVE')
		FROM workspaces w
		LEFT JOIN product_modules pm ON pm.product_code = w.product_code
		WHERE w.workspace_id=?`, id).
		Scan(&name, &customer, &desc, &created, &pcode, &pname, &pstatus)
	if err != nil {
		writeErr(w, 404, "not found")
		return
	}
	writeJSON(w, 200, map[string]interface{}{
		"workspaceId": id, "workspaceName": name, "customerName": customer,
		"description": desc, "createdAt": created,
		"productCode": pcode, "productName": pname, "productStatus": pstatus,
	})
}

// ---------------- Connections ----------------

func (h *Handler) ListConnections(w http.ResponseWriter, r *http.Request) {
	id, _ := wsID(r)
	h.ensureConnectionSchema()
	rows, err := h.ctrl.Query(`
		SELECT connection_id, connection_name, db_type, host, port, database_name, username,
		       COALESCE(http_path,''), COALESCE(extra_config,'{}'), is_active, created_at
		FROM data_connections WHERE workspace_id=? ORDER BY connection_id`, id)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	defer rows.Close()
	list := []map[string]interface{}{}
	for rows.Next() {
		var cid int64
		var cname, dbType, host, dbName, user, httpPath, extra string
		var port int
		var active bool
		var created time.Time
		_ = rows.Scan(&cid, &cname, &dbType, &host, &port, &dbName, &user, &httpPath, &extra, &active, &created)
		list = append(list, map[string]interface{}{
			"connectionId": cid, "connectionName": cname, "dbType": dbType,
			"host": host, "port": port, "databaseName": dbName, "username": user,
			"httpPath": httpPath, "extra": jsonRaw(sql.NullString{String: extra, Valid: extra != ""}),
			"isActive": active, "createdAt": created,
		})
	}
	writeJSON(w, 200, list)
}

var defaultPorts = map[string]int{"MYSQL": 3306, "SQLSERVER": 1433, "DATABRICKS": 443}

func (h *Handler) CreateConnection(w http.ResponseWriter, r *http.Request) {
	id, _ := wsID(r)
	h.ensureConnectionSchema()
	var body struct {
		ConnectionName string          `json:"connectionName"`
		DbType         string          `json:"dbType"`
		Host           string          `json:"host"`
		Port           int             `json:"port"`
		DatabaseName   string          `json:"databaseName"`
		Username       string          `json:"username"`
		Password       string          `json:"password"`
		HTTPPath       string          `json:"httpPath"`
		Extra          json.RawMessage `json:"extra"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeErr(w, 400, "invalid body")
		return
	}
	dbType := strings.ToUpper(strings.TrimSpace(body.DbType))
	if dbType == "" {
		dbType = "MYSQL"
	}
	if _, ok := defaultPorts[dbType]; !ok {
		writeErr(w, 400, "unsupported dbType: "+dbType)
		return
	}
	if body.Port == 0 {
		body.Port = defaultPorts[dbType]
	}
	if body.DatabaseName == "" {
		body.DatabaseName = "default"
	}
	extra := string(body.Extra)
	if strings.TrimSpace(extra) == "" {
		extra = "{}"
	}
	sealed, err := h.box.Seal(body.Password)
	if err != nil {
		writeErr(w, 500, "could not protect connection secret")
		return
	}
	res, err := h.ctrl.Exec(`
		INSERT INTO data_connections (workspace_id, connection_name, db_type, host, port, database_name, username, password_enc, http_path, extra_config)
		VALUES (?,?,?,?,?,?,?,?,?,?)`,
		id, body.ConnectionName, dbType, body.Host, body.Port, body.DatabaseName, body.Username, sealed, body.HTTPPath, extra)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	cid, _ := res.LastInsertId()
	writeJSON(w, 201, map[string]interface{}{"connectionId": cid})
}

func (h *Handler) DeleteConnection(w http.ResponseWriter, r *http.Request) {
	ws, _ := wsID(r)
	connID, _ := strconv.ParseInt(chi.URLParam(r, "connId"), 10, 64)
	res, err := h.ctrl.Exec(`DELETE FROM data_connections WHERE connection_id=? AND workspace_id=?`, connID, ws)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	n, _ := res.RowsAffected()
	writeJSON(w, 200, map[string]interface{}{"deleted": n > 0, "connectionId": connID})
}

func (h *Handler) TestConnection(w http.ResponseWriter, r *http.Request) {
	ws, _ := wsID(r)
	h.ensureConnectionSchema()
	connID, _ := strconv.ParseInt(chi.URLParam(r, "connId"), 10, 64)
	var dbType, host, dbName, user, pass, httpPath, extra string
	var port int
	err := h.ctrl.QueryRow(`
		SELECT db_type, host, port, database_name, username, password_enc, COALESCE(http_path,''), COALESCE(extra_config,'{}')
		FROM data_connections WHERE connection_id=? AND workspace_id=?`, connID, ws).
		Scan(&dbType, &host, &port, &dbName, &user, &pass, &httpPath, &extra)
	if err != nil {
		writeErr(w, 404, "connection not found")
		return
	}
	pass, err = h.box.Open(pass)
	if err != nil {
		writeErr(w, 500, "could not read connection secret")
		return
	}

	switch strings.ToUpper(dbType) {
	case "DATABRICKS":
		ok, status, msg := h.testDatabricks(host, httpPath, user, pass, extra)
		writeJSON(w, 200, map[string]interface{}{"ok": ok, "status": status, "message": msg, "error": ifNot(ok, msg)})
		return
	case "SQLSERVER":
		writeJSON(w, 200, map[string]interface{}{
			"ok": false, "status": "placeholder",
			"message": "SQL Server connector is a placeholder — connectivity is not implemented yet.",
			"error":   "SQL Server connector is a placeholder — connectivity is not implemented yet.",
		})
		return
	default: // MYSQL
		dsn := fmt.Sprintf("%s:%s@tcp(%s:%d)/%s?timeout=5s", user, pass, host, port, dbName)
		test, err := sql.Open("mysql", dsn)
		if err != nil {
			writeJSON(w, 200, map[string]interface{}{"ok": false, "status": "failed", "error": err.Error()})
			return
		}
		defer test.Close()
		if err := test.Ping(); err != nil {
			writeJSON(w, 200, map[string]interface{}{"ok": false, "status": "failed", "error": err.Error()})
			return
		}
		var tables int
		_ = test.QueryRow(`SELECT COUNT(*) FROM information_schema.tables WHERE table_schema=?`, dbName).Scan(&tables)
		writeJSON(w, 200, map[string]interface{}{
			"ok": true, "status": "connected", "tableCount": tables,
			"message": fmt.Sprintf("Connected — %d tables in %s.", tables, dbName),
		})
	}
}

func ifNot(ok bool, msg string) string {
	if ok {
		return ""
	}
	return msg
}

// ---------------- Rules ----------------

func (h *Handler) ListRules(w http.ResponseWriter, r *http.Request) {
	id, _ := wsID(r)
	rows, err := h.ctrl.Query(`
		SELECT rule_id, rule_name, dimension, business_definition, technical_definition,
		       impacted_tables, impacted_columns, check_sql, status, proc_name, created_at
		FROM dq_rules WHERE workspace_id=? ORDER BY rule_id DESC`, id)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	defer rows.Close()
	list := []map[string]interface{}{}
	for rows.Next() {
		var rid int64
		var name, dim, status string
		var biz, tech, tables, cols, checkSQL, proc sql.NullString
		var created time.Time
		_ = rows.Scan(&rid, &name, &dim, &biz, &tech, &tables, &cols, &checkSQL, &status, &proc, &created)
		list = append(list, map[string]interface{}{
			"ruleId": rid, "ruleName": name, "dimension": dim,
			"businessDefinition": biz.String, "technicalDefinition": tech.String,
			"impactedTables": jsonRaw(tables), "impactedColumns": jsonRaw(cols),
			"checkSql": checkSQL.String, "status": status, "procName": proc.String, "createdAt": created,
		})
	}
	writeJSON(w, 200, list)
}

func jsonRaw(ns sql.NullString) interface{} {
	if !ns.Valid || ns.String == "" {
		return []interface{}{}
	}
	var v interface{}
	if err := json.Unmarshal([]byte(ns.String), &v); err != nil {
		return ns.String
	}
	return v
}

// ruleAuditPayload carries the traceability captured on the DQ Analyser screen
// when an analyst saves a recommended rule.
type ruleAuditPayload struct {
	Prompt       string          `json:"prompt"`
	Conversation json.RawMessage `json:"conversation"`
	ToolTraces   json.RawMessage `json:"toolTraces"`
	ModelUsed    string          `json:"modelUsed"`
	DataSource   string          `json:"dataSource"`
	Impact       *struct {
		ImpactedCount   int64   `json:"impactedCount"`
		TotalCount      int64   `json:"totalCount"`
		ImpactedPercent float64 `json:"impactedPercent"`
	} `json:"impact"`
}

func (h *Handler) CreateRule(w http.ResponseWriter, r *http.Request) {
	id, _ := wsID(r)
	uid := middleware.UserID(r.Context())
	var body struct {
		RuleName            string            `json:"ruleName"`
		Dimension           string            `json:"dimension"`
		BusinessDefinition  string            `json:"businessDefinition"`
		TechnicalDefinition string            `json:"technicalDefinition"`
		ImpactedTables      json.RawMessage   `json:"impactedTables"`
		ImpactedColumns     json.RawMessage   `json:"impactedColumns"`
		CheckSQL            string            `json:"checkSql"`
		Status              string            `json:"status"`
		ProcName            string            `json:"procName"`
		Audit               *ruleAuditPayload `json:"audit"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeErr(w, 400, "invalid body")
		return
	}
	if body.Status == "" {
		body.Status = "DRAFT"
	}
	if rejectBadCheckSQL(w, body.CheckSQL) {
		return
	}
	dim := strings.ToUpper(body.Dimension)

	existing, err := h.loadRules(id)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	if dup, reason, ok := findDuplicate(existing, body.RuleName, dim, body.CheckSQL); ok {
		why := "the same name"
		if reason == "check" {
			why = "the same dimension and check SQL"
		}
		writeJSON(w, 200, map[string]interface{}{
			"ruleId": dup.ID, "duplicate": true, "reason": reason, "status": dup.Status,
			"message": fmt.Sprintf("Rule already exists with ID %d (%s, %s)", dup.ID, dup.Status, why),
		})
		return
	}

	tables := string(body.ImpactedTables)
	cols := string(body.ImpactedColumns)
	if tables == "" {
		tables = "[]"
	}
	if cols == "" {
		cols = "[]"
	}
	res, err := h.ctrl.Exec(`
		INSERT INTO dq_rules (workspace_id, rule_name, dimension, business_definition, technical_definition,
		  impacted_tables, impacted_columns, check_sql, status, proc_name, created_by)
		VALUES (?,?,?,?,?,?,?,?,?,?,?)`,
		id, body.RuleName, dim, body.BusinessDefinition, body.TechnicalDefinition,
		tables, cols, body.CheckSQL, body.Status, nullIfEmpty(body.ProcName), uid)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	rid, _ := res.LastInsertId()

	// Persist the traceability record for every saved rule (non-fatal on failure).
	if err := h.saveRuleAudit(id, rid, uid, body.RuleName, dim, body.CheckSQL, body.Status, body.Audit); err != nil {
		log.Printf("rule audit save failed (rule %d): %v", rid, err)
	}

	writeJSON(w, 201, map[string]interface{}{"ruleId": rid, "duplicate": false})
}

// EditRule updates the editable content of a rule: business/technical definitions
// and the check SQL. Status and identity are not changed here.
func (h *Handler) EditRule(w http.ResponseWriter, r *http.Request) {
	ws, _ := wsID(r)
	ruleID, _ := strconv.ParseInt(chi.URLParam(r, "ruleId"), 10, 64)
	var body struct {
		BusinessDefinition  string `json:"businessDefinition"`
		TechnicalDefinition string `json:"technicalDefinition"`
		CheckSQL            string `json:"checkSql"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeErr(w, 400, "invalid body")
		return
	}
	if rejectBadCheckSQL(w, body.CheckSQL) {
		return
	}
	_, err := h.ctrl.Exec(
		`UPDATE dq_rules SET business_definition=?, technical_definition=?, check_sql=?
		 WHERE rule_id=? AND workspace_id=?`,
		body.BusinessDefinition, body.TechnicalDefinition, body.CheckSQL, ruleID, ws)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, map[string]interface{}{"ruleId": ruleID, "updated": true})
}

// DeleteRule hard-deletes a rule (used when a recommendation is rejected). The
// rule is never physicalised; its audit rows are removed alongside it.
func (h *Handler) DeleteRule(w http.ResponseWriter, r *http.Request) {
	ws, _ := wsID(r)
	ruleID, _ := strconv.ParseInt(chi.URLParam(r, "ruleId"), 10, 64)
	h.ensureAuditTable()
	_, _ = h.ctrl.Exec(`DELETE FROM dq_rule_audit WHERE rule_id=? AND workspace_id=?`, ruleID, ws)
	res, err := h.ctrl.Exec(`DELETE FROM dq_rules WHERE rule_id=? AND workspace_id=?`, ruleID, ws)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	n, _ := res.RowsAffected()
	writeJSON(w, 200, map[string]interface{}{"deleted": n > 0, "ruleId": ruleID})
}

var auditTableOnce sync.Once

func (h *Handler) ensureAuditTable() {
	auditTableOnce.Do(func() {
		_, err := h.ctrl.Exec(`
			CREATE TABLE IF NOT EXISTS dq_rule_audit (
			    audit_id         BIGINT UNSIGNED PRIMARY KEY AUTO_INCREMENT,
			    rule_id          BIGINT UNSIGNED NULL,
			    workspace_id     BIGINT UNSIGNED NOT NULL,
			    rule_name        VARCHAR(200) NOT NULL,
			    dimension        VARCHAR(40) NULL,
			    prompt           TEXT,
			    conversation     JSON,
			    tool_traces      JSON,
			    model_used       VARCHAR(100) NULL,
			    data_source      VARCHAR(20) NULL,
			    impacted_count   BIGINT NULL,
			    total_count      BIGINT NULL,
			    impacted_percent DECIMAL(7,2) NULL,
			    check_sql        TEXT,
			    status           VARCHAR(20) NULL,
			    created_by       BIGINT UNSIGNED NULL,
			    created_at       TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			    KEY idx_audit_ws (workspace_id),
			    KEY idx_audit_rule (rule_id)
			) ENGINE=InnoDB`)
		if err != nil {
			log.Printf("ensure dq_rule_audit: %v", err)
		}
	})
}

func (h *Handler) saveRuleAudit(ws, ruleID, uid int64, ruleName, dim, checkSQL, status string, a *ruleAuditPayload) error {
	h.ensureAuditTable()
	if a == nil {
		a = &ruleAuditPayload{}
	}
	conv := string(a.Conversation)
	if conv == "" {
		conv = "[]"
	}
	traces := string(a.ToolTraces)
	if traces == "" {
		traces = "[]"
	}
	var ic, tc, pct interface{}
	if a.Impact != nil {
		ic = a.Impact.ImpactedCount
		tc = a.Impact.TotalCount
		pct = a.Impact.ImpactedPercent
	}
	_, err := h.ctrl.Exec(`
		INSERT INTO dq_rule_audit (rule_id, workspace_id, rule_name, dimension, prompt, conversation,
		  tool_traces, model_used, data_source, impacted_count, total_count, impacted_percent,
		  check_sql, status, created_by)
		VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		ruleID, ws, ruleName, nullIfEmpty(dim), nullIfEmpty(a.Prompt), conv, traces,
		nullIfEmpty(a.ModelUsed), nullIfEmpty(a.DataSource), ic, tc, pct,
		nullIfEmpty(checkSQL), nullIfEmpty(status), uid)
	return err
}

// ListRuleAudit returns the traceability trail for saved rules (for the audit screen).
func (h *Handler) ListRuleAudit(w http.ResponseWriter, r *http.Request) {
	id, _ := wsID(r)
	h.ensureAuditTable()
	rows, err := h.ctrl.Query(`
		SELECT audit_id, rule_id, rule_name, dimension, prompt, conversation, tool_traces,
		       model_used, data_source, impacted_count, total_count, impacted_percent,
		       check_sql, status, created_by, created_at
		FROM dq_rule_audit WHERE workspace_id=? ORDER BY audit_id DESC`, id)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	defer rows.Close()
	list := []map[string]interface{}{}
	for rows.Next() {
		var auditID int64
		var ruleID sql.NullInt64
		var ic, tc sql.NullInt64
		var pct sql.NullFloat64
		var createdBy sql.NullInt64
		var name string
		var dim, prompt, conv, traces, model, dsrc, checkSQL, status sql.NullString
		var created time.Time
		_ = rows.Scan(&auditID, &ruleID, &name, &dim, &prompt, &conv, &traces,
			&model, &dsrc, &ic, &tc, &pct, &checkSQL, &status, &createdBy, &created)
		list = append(list, map[string]interface{}{
			"auditId": auditID, "ruleId": nullInt(ruleID), "ruleName": name,
			"dimension": dim.String, "prompt": prompt.String,
			"conversation": jsonRaw(conv), "toolTraces": jsonRaw(traces),
			"modelUsed": model.String, "dataSource": dsrc.String,
			"impactedCount": nullInt(ic), "totalCount": nullInt(tc),
			"impactedPercent": nullFloat(pct), "checkSql": checkSQL.String,
			"status": status.String, "createdBy": nullInt(createdBy), "createdAt": created,
		})
	}
	writeJSON(w, 200, list)
}

func nullInt(n sql.NullInt64) interface{} {
	if !n.Valid {
		return nil
	}
	return n.Int64
}

func nullFloat(n sql.NullFloat64) interface{} {
	if !n.Valid {
		return nil
	}
	return n.Float64
}

func nullIfEmpty(s string) interface{} {
	if s == "" {
		return nil
	}
	return s
}

func (h *Handler) UpdateRule(w http.ResponseWriter, r *http.Request) {
	ws, _ := wsID(r)
	ruleID, _ := strconv.ParseInt(chi.URLParam(r, "ruleId"), 10, 64)
	var body struct {
		Status string `json:"status"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeErr(w, 400, "invalid body")
		return
	}
	body.Status = strings.ToUpper(strings.TrimSpace(body.Status))
	switch body.Status {
	case "DRAFT", "APPROVED", "ACTIVE", "RETIRED":
	default:
		writeErr(w, 400, "status must be DRAFT, APPROVED, ACTIVE, or RETIRED")
		return
	}
	_, err := h.ctrl.Exec(`UPDATE dq_rules SET status=? WHERE rule_id=? AND workspace_id=?`, body.Status, ruleID, ws)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, map[string]string{"status": body.Status})
}

// ---------------- Pipelines / Runs ----------------

func (h *Handler) ListPipelines(w http.ResponseWriter, r *http.Request) {
	id, _ := wsID(r)
	rows, err := h.ctrl.Query(`
		SELECT pipeline_id, pipeline_name, proc_name, description, is_active, created_at
		FROM dq_pipelines WHERE workspace_id=? ORDER BY pipeline_id`, id)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	defer rows.Close()
	list := []map[string]interface{}{}
	for rows.Next() {
		var pid int64
		var name, proc, desc string
		var active bool
		var created time.Time
		_ = rows.Scan(&pid, &name, &proc, &desc, &active, &created)
		list = append(list, map[string]interface{}{
			"pipelineId": pid, "pipelineName": name, "procName": proc,
			"description": desc, "isActive": active, "createdAt": created,
		})
	}
	writeJSON(w, 200, list)
}

func (h *Handler) RunPipeline(w http.ResponseWriter, r *http.Request) {
	ws, _ := wsID(r)
	pid, _ := strconv.ParseInt(chi.URLParam(r, "pipelineId"), 10, 64)
	uid := middleware.UserID(r.Context())
	var procName string
	err := h.ctrl.QueryRow(`SELECT proc_name FROM dq_pipelines WHERE pipeline_id=? AND workspace_id=?`, pid, ws).Scan(&procName)
	if err != nil {
		writeErr(w, 404, "pipeline not found")
		return
	}
	if h.src == nil {
		writeErr(w, 503, "source database not connected")
		return
	}

	res, err := h.ctrl.Exec(`
		INSERT INTO dq_job_runs (pipeline_id, workspace_id, status, started_at, triggered_by)
		VALUES (?,?, 'RUNNING', NOW(), ?)`, pid, ws, uid)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	runID, _ := res.LastInsertId()

	rowsChecked, rowsFailed, findings, runErr := h.executeProc(procName)
	status := "PASSED"
	msg := "OK"
	if runErr != nil {
		status = "ERROR"
		msg = runErr.Error()
	} else if rowsFailed > 0 {
		status = "FAILED"
		msg = fmt.Sprintf("%d failing rows", rowsFailed)
	}
	_, _ = h.ctrl.Exec(`
		UPDATE dq_job_runs SET status=?, finished_at=NOW(), rows_checked=?, rows_failed=?, message=? WHERE run_id=?`,
		status, rowsChecked, rowsFailed, msg, runID)

	detail, _ := json.Marshal(findings)
	_, _ = h.ctrl.Exec(`INSERT INTO dq_job_results (run_id, dimension, detail_json) VALUES (?,?,?)`,
		runID, extractDim(procName), string(detail))

	writeJSON(w, 200, map[string]interface{}{
		"runId": runID, "status": status, "rowsChecked": rowsChecked,
		"rowsFailed": rowsFailed, "message": msg, "findings": findings,
	})
}

func extractDim(proc string) string {
	switch proc {
	case "sp_dq_cdr_unrated":
		return "COMPLETENESS"
	case "sp_dq_invoice_balance":
		return "CONSISTENCY"
	case "sp_dq_msisdn_validity":
		return "VALIDITY"
	default:
		return "UNKNOWN"
	}
}

func (h *Handler) executeProc(procName string) (checked, failed int64, findings []map[string]interface{}, err error) {
	allowed := map[string]bool{
		"sp_dq_cdr_unrated": true, "sp_dq_invoice_balance": true, "sp_dq_msisdn_validity": true,
	}
	if !allowed[procName] {
		return 0, 0, nil, fmt.Errorf("procedure not allowlisted: %s", procName)
	}
	rows, err := h.src.Query("CALL " + procName + "()")
	if err != nil {
		return 0, 0, nil, err
	}
	defer rows.Close()

	cols, _ := rows.Columns()
	findings = []map[string]interface{}{}
	for rows.Next() {
		vals := make([]interface{}, len(cols))
		ptrs := make([]interface{}, len(cols))
		for i := range vals {
			ptrs[i] = &vals[i]
		}
		_ = rows.Scan(ptrs...)
		row := map[string]interface{}{}
		for i, c := range cols {
			row[c] = stringify(vals[i])
		}
		findings = append(findings, row)
	}

	// next result sets: rows_checked, rows_failed
	if rows.NextResultSet() {
		if rows.Next() {
			_ = rows.Scan(&checked)
		}
	}
	if rows.NextResultSet() {
		if rows.Next() {
			_ = rows.Scan(&failed)
		}
	}
	if checked == 0 && len(findings) > 0 {
		// fallback if counts not available
		failed = int64(len(findings))
		checked = failed
	}
	return checked, failed, findings, nil
}

func stringify(v interface{}) interface{} {
	switch t := v.(type) {
	case []byte:
		return string(t)
	default:
		return t
	}
}

func (h *Handler) ListRuns(w http.ResponseWriter, r *http.Request) {
	id, _ := wsID(r)
	rows, err := h.ctrl.Query(`
		SELECT r.run_id, r.pipeline_id, p.pipeline_name, r.status, r.started_at, r.finished_at,
		       r.rows_checked, r.rows_failed, r.message
		FROM dq_job_runs r
		JOIN dq_pipelines p ON p.pipeline_id = r.pipeline_id
		WHERE r.workspace_id=?
		ORDER BY r.run_id DESC LIMIT 100`, id)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	defer rows.Close()
	list := []map[string]interface{}{}
	for rows.Next() {
		var runID, pid, checked, failed int64
		var pname, status, msg string
		var started, finished sql.NullTime
		_ = rows.Scan(&runID, &pid, &pname, &status, &started, &finished, &checked, &failed, &msg)
		item := map[string]interface{}{
			"runId": runID, "pipelineId": pid, "pipelineName": pname, "status": status,
			"rowsChecked": checked, "rowsFailed": failed, "message": msg,
		}
		if started.Valid {
			item["startedAt"] = started.Time
		}
		if finished.Valid {
			item["finishedAt"] = finished.Time
		}
		list = append(list, item)
	}
	writeJSON(w, 200, list)
}

func (h *Handler) Dashboard(w http.ResponseWriter, r *http.Request) {
	id, _ := wsID(r)
	var total, passed, failed, errors int64
	_ = h.ctrl.QueryRow(`SELECT COUNT(*) FROM dq_job_runs WHERE workspace_id=?`, id).Scan(&total)
	_ = h.ctrl.QueryRow(`SELECT COUNT(*) FROM dq_job_runs WHERE workspace_id=? AND status='PASSED'`, id).Scan(&passed)
	_ = h.ctrl.QueryRow(`SELECT COUNT(*) FROM dq_job_runs WHERE workspace_id=? AND status='FAILED'`, id).Scan(&failed)
	_ = h.ctrl.QueryRow(`SELECT COUNT(*) FROM dq_job_runs WHERE workspace_id=? AND status='ERROR'`, id).Scan(&errors)

	dimRows, err := h.ctrl.Query(`
		SELECT COALESCE(res.dimension,'UNKNOWN') AS dim, COUNT(*) AS cnt,
		       SUM(CASE WHEN r.status='FAILED' THEN 1 ELSE 0 END) AS fail_cnt
		FROM dq_job_runs r
		LEFT JOIN dq_job_results res ON res.run_id = r.run_id
		WHERE r.workspace_id=?
		GROUP BY COALESCE(res.dimension,'UNKNOWN')`, id)
	byDim := []map[string]interface{}{}
	if err == nil {
		defer dimRows.Close()
		for dimRows.Next() {
			var dim string
			var cnt, failCnt int64
			_ = dimRows.Scan(&dim, &cnt, &failCnt)
			byDim = append(byDim, map[string]interface{}{"dimension": dim, "runs": cnt, "failed": failCnt})
		}
	}

	var rulesDraft, rulesActive int64
	_ = h.ctrl.QueryRow(`SELECT COUNT(*) FROM dq_rules WHERE workspace_id=? AND status='DRAFT'`, id).Scan(&rulesDraft)
	_ = h.ctrl.QueryRow(`SELECT COUNT(*) FROM dq_rules WHERE workspace_id=? AND status IN ('APPROVED','ACTIVE')`, id).Scan(&rulesActive)

	writeJSON(w, 200, map[string]interface{}{
		"totalRuns": total, "passed": passed, "failed": failed, "errors": errors,
		"byDimension": byDim, "rulesDraft": rulesDraft, "rulesActive": rulesActive,
	})
}

// ---------------- ACL ----------------

func (h *Handler) GetACL(w http.ResponseWriter, r *http.Request) {
	rows, err := h.ctrl.Query(`
		SELECT a.screen_acl_id, r.role_code, a.screen_code, a.can_view, a.can_edit
		FROM screen_acl a JOIN roles r ON r.role_id = a.role_id
		ORDER BY r.role_code, a.screen_code`)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	defer rows.Close()
	list := []map[string]interface{}{}
	for rows.Next() {
		var aid int64
		var role, screen string
		var view, edit bool
		_ = rows.Scan(&aid, &role, &screen, &view, &edit)
		list = append(list, map[string]interface{}{
			"screenAclId": aid, "roleCode": role, "screenCode": screen, "canView": view, "canEdit": edit,
		})
	}
	writeJSON(w, 200, list)
}

func (h *Handler) UpdateACL(w http.ResponseWriter, r *http.Request) {
	var body struct {
		ScreenAclID int64 `json:"screenAclId"`
		CanView     bool  `json:"canView"`
		CanEdit     bool  `json:"canEdit"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeErr(w, 400, "invalid body")
		return
	}
	_, err := h.ctrl.Exec(`UPDATE screen_acl SET can_view=?, can_edit=? WHERE screen_acl_id=?`,
		body.CanView, body.CanEdit, body.ScreenAclID)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, map[string]string{"status": "updated"})
}

// ---------------- Analyser proxy ----------------

func (h *Handler) AnalyserChat(w http.ResponseWriter, r *http.Request) {
	ws, _ := wsID(r)
	body, err := io.ReadAll(r.Body)
	if err != nil {
		writeErr(w, 400, "bad body")
		return
	}
	var payload map[string]interface{}
	_ = json.Unmarshal(body, &payload)
	if payload == nil {
		payload = map[string]interface{}{}
	}
	payload["workspaceId"] = ws

	// Attach the active MySQL warehouse connection. Multi-driver workspaces may
	// also have Databricks/etc. marked active; those must not be handed to the
	// MySQL MCP analyser bridge (host:443 + PAT looks like a dead MCP channel).
	var host, dbName, user, pass string
	var port int
	err = h.ctrl.QueryRow(`
		SELECT host, port, database_name, username, password_enc
		FROM data_connections
		WHERE workspace_id=? AND is_active=1 AND UPPER(db_type)='MYSQL'
		ORDER BY connection_id DESC LIMIT 1`, ws).
		Scan(&host, &port, &dbName, &user, &pass)
	if err == nil {
		plain, oerr := h.box.Open(pass)
		if oerr != nil {
			log.Printf("analyser: connection secret: %v", oerr)
		} else {
			// Password stays off the chat body. The agent redeems this once.
			payload["connection"] = map[string]interface{}{
				"host": host, "port": port, "database": dbName, "username": user,
				"handoff": h.putHandoff(connectionHandoff{
					Host: host, Port: port, Database: dbName, Username: user, Password: plain,
				}),
			}
		}
	}
	if msg, _ := payload["message"].(string); strings.TrimSpace(msg) != "" {
		if rules, rerr := h.loadRules(ws); rerr == nil {
			payload["existingRules"] = catalogueForAgent(matchRules(rules, ruleQuery{Q: msg, Limit: 8}))
		}
	}

	out, _ := json.Marshal(payload)
	resp, err := http.Post(h.cfg.AgentURL+"/chat", "application/json", bytes.NewReader(out))
	if err != nil {
		writeErr(w, 502, "agent unreachable: "+err.Error())
		return
	}
	defer resp.Body.Close()
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(resp.StatusCode)
	_, _ = io.Copy(w, resp.Body)
}

// EtlAnalyze proxies ETL/SQL file analysis and follow-up questions to the agent.
func (h *Handler) EtlAnalyze(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(io.LimitReader(r.Body, 8<<20)) // 8 MB cap
	if err != nil {
		writeErr(w, 400, "bad body")
		return
	}
	resp, err := http.Post(h.cfg.AgentURL+"/etl/analyze", "application/json", bytes.NewReader(body))
	if err != nil {
		writeErr(w, 502, "agent unreachable: "+err.Error())
		return
	}
	defer resp.Body.Close()
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(resp.StatusCode)
	_, _ = io.Copy(w, resp.Body)
}

// EtlExplainNode proxies a single graph-node explanation request to the agent.
func (h *Handler) EtlExplainNode(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(io.LimitReader(r.Body, 8<<20))
	if err != nil {
		writeErr(w, 400, "bad body")
		return
	}
	resp, err := http.Post(h.cfg.AgentURL+"/etl/explain-node", "application/json", bytes.NewReader(body))
	if err != nil {
		writeErr(w, 502, "agent unreachable: "+err.Error())
		return
	}
	defer resp.Body.Close()
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(resp.StatusCode)
	_, _ = io.Copy(w, resp.Body)
}
