package handlers

import (
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/harish-aiproject-lib/AgenticDataGov/backend/internal/sqlguard"
)

type storedRule struct {
	ID        int64
	Name      string
	Dimension string
	Business  string
	Technical string
	Tables    string
	Columns   string
	CheckSQL  string
	Status    string
	ProcName  string
	CreatedAt time.Time
}

func (h *Handler) loadRules(ws int64) ([]storedRule, error) {
	rows, err := h.ctrl.Query(`
		SELECT rule_id, rule_name, dimension, business_definition, technical_definition,
		       impacted_tables, impacted_columns, check_sql, status, proc_name, created_at
		FROM dq_rules WHERE workspace_id=? ORDER BY rule_id DESC`, ws)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var list []storedRule
	for rows.Next() {
		var r storedRule
		var biz, tech, tables, cols, checkSQL, proc sql.NullString
		if err := rows.Scan(&r.ID, &r.Name, &r.Dimension, &biz, &tech, &tables, &cols, &checkSQL, &r.Status, &proc, &r.CreatedAt); err != nil {
			return nil, err
		}
		r.Business, r.Technical = biz.String, tech.String
		r.Tables, r.Columns, r.CheckSQL, r.ProcName = tables.String, cols.String, checkSQL.String, proc.String
		list = append(list, r)
	}
	return list, rows.Err()
}

func ruleJSON(r storedRule) map[string]interface{} {
	return map[string]interface{}{
		"ruleId": r.ID, "ruleName": r.Name, "dimension": r.Dimension,
		"businessDefinition": r.Business, "technicalDefinition": r.Technical,
		"impactedTables":  jsonRaw(sql.NullString{String: r.Tables, Valid: r.Tables != ""}),
		"impactedColumns": jsonRaw(sql.NullString{String: r.Columns, Valid: r.Columns != ""}),
		"checkSql":        r.CheckSQL, "status": r.Status, "procName": r.ProcName, "createdAt": r.CreatedAt,
	}
}

func normalizeSQL(s string) string {
	return strings.Join(strings.Fields(strings.ToLower(strings.TrimRight(strings.TrimSpace(s), ";"))), " ")
}

func sqlFingerprint(s string) string {
	sum := sha256.Sum256([]byte(normalizeSQL(s)))
	return hex.EncodeToString(sum[:])
}

func tableNames(raw string) []string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	var arr []string
	if err := json.Unmarshal([]byte(raw), &arr); err != nil {
		return nil
	}
	out := make([]string, 0, len(arr))
	for _, t := range arr {
		t = strings.ToLower(strings.TrimSpace(t))
		if t != "" {
			out = append(out, t)
		}
	}
	return out
}

// findDuplicate reports an existing rule with the same name, or the same
// dimension and normalised check SQL, inside this workspace.
func findDuplicate(rules []storedRule, name, dimension, checkSQL string) (storedRule, string, bool) {
	name = strings.ToLower(strings.TrimSpace(name))
	fp := sqlFingerprint(checkSQL)
	dim := strings.ToUpper(strings.TrimSpace(dimension))
	for _, r := range rules {
		if strings.ToLower(r.Name) == name {
			return r, "name", true
		}
	}
	for _, r := range rules {
		if strings.ToUpper(r.Dimension) == dim && sqlFingerprint(r.CheckSQL) == fp {
			return r, "check", true
		}
	}
	return storedRule{}, "", false
}

type ruleQuery struct {
	Q         string
	Table     string
	Dimension string
	Statuses  []string
	Limit     int
}

func (h *Handler) SearchRules(w http.ResponseWriter, r *http.Request) {
	ws, _ := wsID(r)
	q := ruleQuery{
		Q:         strings.TrimSpace(r.URL.Query().Get("q")),
		Table:     strings.ToLower(strings.TrimSpace(r.URL.Query().Get("table"))),
		Dimension: strings.ToUpper(strings.TrimSpace(r.URL.Query().Get("dimension"))),
		Limit:     200,
	}
	if s := strings.TrimSpace(r.URL.Query().Get("status")); s != "" {
		for _, part := range strings.Split(s, ",") {
			part = strings.ToUpper(strings.TrimSpace(part))
			if part != "" {
				q.Statuses = append(q.Statuses, part)
			}
		}
	}
	if n, err := strconv.Atoi(r.URL.Query().Get("limit")); err == nil && n > 0 && n <= 200 {
		q.Limit = n
	}
	rules, err := h.loadRules(ws)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	matched := matchRules(rules, q)
	out := make([]map[string]interface{}, 0, len(matched))
	for _, rule := range matched {
		out = append(out, ruleJSON(rule))
	}
	writeJSON(w, 200, out)
}

func matchRules(rules []storedRule, q ruleQuery) []storedRule {
	type scored struct {
		r storedRule
		n int
	}
	var hits []scored
	var tokens []string
	for _, tok := range strings.Fields(strings.ToLower(q.Q)) {
		if len(tok) >= 3 {
			tokens = append(tokens, tok)
		}
	}
	for _, r := range rules {
		if q.Dimension != "" && !strings.EqualFold(r.Dimension, q.Dimension) {
			continue
		}
		if len(q.Statuses) > 0 && !containsFold(q.Statuses, r.Status) {
			continue
		}
		tables := tableNames(r.Tables)
		if q.Table != "" && !containsFold(tables, q.Table) {
			continue
		}
		score := 1
		if len(tokens) > 0 {
			score = 0
			hayName := strings.ToLower(r.Name)
			hayBiz := strings.ToLower(r.Business)
			hayTech := strings.ToLower(r.Technical)
			haySQL := strings.ToLower(r.CheckSQL)
			hayDim := strings.ToLower(r.Dimension)
			for _, tok := range tokens {
				switch {
				case strings.Contains(hayName, tok):
					score += 4
				case containsFold(tables, tok):
					score += 3
				case strings.Contains(hayDim, tok) || strings.Contains(hayBiz, tok):
					score += 2
				case strings.Contains(hayTech, tok) || strings.Contains(haySQL, tok):
					score += 1
				}
			}
			if score == 0 {
				continue
			}
		}
		hits = append(hits, scored{r, score})
	}
	// Stable: higher score first; loadRules is already newest-first, so equal
	// scores keep that order (simple insertion sort, catalogues are small).
	for i := 1; i < len(hits); i++ {
		j := i
		for j > 0 && hits[j].n > hits[j-1].n {
			hits[j], hits[j-1] = hits[j-1], hits[j]
			j--
		}
	}
	if q.Limit > 0 && len(hits) > q.Limit {
		hits = hits[:q.Limit]
	}
	out := make([]storedRule, len(hits))
	for i, h := range hits {
		out[i] = h.r
	}
	return out
}

func containsFold(list []string, want string) bool {
	for _, s := range list {
		if strings.EqualFold(s, want) {
			return true
		}
	}
	return false
}

func catalogueForAgent(rules []storedRule) []map[string]interface{} {
	out := make([]map[string]interface{}, 0, len(rules))
	for _, r := range rules {
		check := r.CheckSQL
		if len(check) > 300 {
			check = check[:300]
		}
		biz := r.Business
		if len(biz) > 200 {
			biz = biz[:200]
		}
		out = append(out, map[string]interface{}{
			"ruleId": r.ID, "ruleName": r.Name, "dimension": r.Dimension, "status": r.Status,
			"businessDefinition": biz, "impactedTables": tableNames(r.Tables), "checkSql": check,
		})
	}
	return out
}

func rejectBadCheckSQL(w http.ResponseWriter, sql string) bool {
	if err := sqlguard.ValidateSelect(sql); err != nil {
		writeErr(w, 400, "checkSql: "+err.Error())
		return true
	}
	return false
}
