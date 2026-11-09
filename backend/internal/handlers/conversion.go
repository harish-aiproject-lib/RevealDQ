package handlers

import (
	"bytes"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/harish-aiproject-lib/AgenticDataGov/backend/internal/middleware"
)

// ===========================================================================
// Proxies to the conversion agent
// ===========================================================================
func (h *Handler) ConversionAnalyze(w http.ResponseWriter, r *http.Request) {
	h.proxyAgent(w, r, "/convert/analyze")
}

func (h *Handler) ConversionRun(w http.ResponseWriter, r *http.Request) {
	h.proxyAgent(w, r, "/convert/run")
}

func (h *Handler) proxyAgent(w http.ResponseWriter, r *http.Request, path string) {
	body, err := io.ReadAll(io.LimitReader(r.Body, 16<<20))
	if err != nil {
		writeErr(w, 400, "bad body")
		return
	}
	resp, err := http.Post(h.cfg.AgentURL+path, "application/json", bytes.NewReader(body))
	if err != nil {
		writeErr(w, 502, "agent unreachable: "+err.Error())
		return
	}
	defer resp.Body.Close()
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(resp.StatusCode)
	_, _ = io.Copy(w, resp.Body)
}

// ===========================================================================
// Conversion audit table
// ===========================================================================
var conversionAuditOnce sync.Once

func (h *Handler) ensureConversionAudit() {
	conversionAuditOnce.Do(func() {
		_, err := h.ctrl.Exec(`
			CREATE TABLE IF NOT EXISTS conversion_audit (
			    conversion_id  BIGINT UNSIGNED PRIMARY KEY AUTO_INCREMENT,
			    workspace_id   BIGINT UNSIGNED NOT NULL,
			    file_name      VARCHAR(255) NOT NULL,
			    source_type    VARCHAR(40) NULL,
			    source_label   VARCHAR(120) NULL,
			    target         VARCHAR(40) NULL,
			    model_used     VARCHAR(100) NULL,
			    engine         VARCHAR(20) NULL,
			    plan_md        MEDIUMTEXT,
			    source_code    MEDIUMTEXT,
			    converted_code MEDIUMTEXT,
			    language       VARCHAR(20) NULL,
			    validation     JSON,
			    status         VARCHAR(30) NULL,
			    git_path       VARCHAR(512) NULL,
			    notebook_path  VARCHAR(512) NULL,
			    job_id         VARCHAR(64) NULL,
			    run_id         VARCHAR(64) NULL,
			    created_by     BIGINT UNSIGNED NULL,
			    created_at     TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			    KEY idx_conv_ws (workspace_id)
			) ENGINE=InnoDB`)
		if err != nil {
			fmt.Printf("ensure conversion_audit: %v\n", err)
		}
	})
}

type conversionAuditBody struct {
	FileName      string          `json:"fileName"`
	SourceType    string          `json:"sourceType"`
	SourceLabel   string          `json:"sourceLabel"`
	Target        string          `json:"target"`
	ModelUsed     string          `json:"modelUsed"`
	Engine        string          `json:"engine"`
	PlanMd        string          `json:"planMarkdown"`
	SourceCode    string          `json:"sourceCode"`
	ConvertedCode string          `json:"convertedCode"`
	Language      string          `json:"language"`
	Validation    json.RawMessage `json:"validation"`
	Status        string          `json:"status"`
	GitPath       string          `json:"gitPath"`
	NotebookPath  string          `json:"notebookPath"`
	JobID         string          `json:"jobId"`
	RunID         string          `json:"runId"`
}

func (h *Handler) SaveConversionAudit(w http.ResponseWriter, r *http.Request) {
	ws, _ := wsID(r)
	uid := middleware.UserID(r.Context())
	h.ensureConversionAudit()
	var b conversionAuditBody
	if err := json.NewDecoder(r.Body).Decode(&b); err != nil {
		writeErr(w, 400, "invalid body")
		return
	}
	val := string(b.Validation)
	if strings.TrimSpace(val) == "" {
		val = "{}"
	}
	res, err := h.ctrl.Exec(`
		INSERT INTO conversion_audit (workspace_id, file_name, source_type, source_label, target,
		  model_used, engine, plan_md, source_code, converted_code, language, validation, status,
		  git_path, notebook_path, job_id, run_id, created_by)
		VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		ws, b.FileName, nullIfEmpty(b.SourceType), nullIfEmpty(b.SourceLabel), nullIfEmpty(b.Target),
		nullIfEmpty(b.ModelUsed), nullIfEmpty(b.Engine), nullIfEmpty(b.PlanMd), nullIfEmpty(b.SourceCode),
		nullIfEmpty(b.ConvertedCode), nullIfEmpty(b.Language), val, nullIfEmpty(b.Status),
		nullIfEmpty(b.GitPath), nullIfEmpty(b.NotebookPath), nullIfEmpty(b.JobID), nullIfEmpty(b.RunID), uid)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	id, _ := res.LastInsertId()
	writeJSON(w, 201, map[string]interface{}{"conversionId": id})
}

func (h *Handler) ListConversionAudit(w http.ResponseWriter, r *http.Request) {
	ws, _ := wsID(r)
	h.ensureConversionAudit()
	rows, err := h.ctrl.Query(`
		SELECT conversion_id, file_name, source_type, source_label, target, model_used, engine,
		       plan_md, converted_code, language, validation, status,
		       COALESCE(git_path,''), COALESCE(notebook_path,''), COALESCE(job_id,''), COALESCE(run_id,''), created_at
		FROM conversion_audit WHERE workspace_id=? ORDER BY conversion_id DESC`, ws)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	defer rows.Close()
	list := []map[string]interface{}{}
	for rows.Next() {
		var id int64
		var fname string
		var stype, slabel, target, model, engine, planMd, code, lang, val, status, git, nb, job, run sql.NullString
		var created time.Time
		_ = rows.Scan(&id, &fname, &stype, &slabel, &target, &model, &engine, &planMd, &code, &lang,
			&val, &status, &git, &nb, &job, &run, &created)
		list = append(list, map[string]interface{}{
			"conversionId": id, "fileName": fname, "sourceType": stype.String, "sourceLabel": slabel.String,
			"target": target.String, "modelUsed": model.String, "engine": engine.String,
			"planMarkdown": planMd.String, "convertedCode": code.String, "language": lang.String,
			"validation": jsonRaw(val), "status": status.String, "gitPath": git.String,
			"notebookPath": nb.String, "jobId": job.String, "runId": run.String, "createdAt": created,
		})
	}
	writeJSON(w, 200, list)
}

// ===========================================================================
// Git check-in of a converted artifact (user-triggered only)
// ===========================================================================
func repoRoot() (string, error) {
	out, err := exec.Command("git", "rev-parse", "--show-toplevel").Output()
	if err != nil {
		wd, e := os.Getwd()
		if e != nil {
			return "", err
		}
		return filepath.Dir(wd), nil // fall back to parent of backend/
	}
	return strings.TrimSpace(string(out)), nil
}

func safeBase(name string) string {
	name = filepath.Base(strings.TrimSpace(name))
	name = strings.NewReplacer(" ", "_", "/", "_", "\\", "_").Replace(name)
	if name == "" || name == "." {
		name = "artifact"
	}
	return name
}

func (h *Handler) ConversionCheckin(w http.ResponseWriter, r *http.Request) {
	ws, _ := wsID(r)
	var b struct {
		FileName string `json:"fileName"`
		Code     string `json:"code"`
		Language string `json:"language"`
	}
	if err := json.NewDecoder(r.Body).Decode(&b); err != nil {
		writeErr(w, 400, "invalid body")
		return
	}
	if strings.TrimSpace(b.Code) == "" {
		writeErr(w, 400, "no code to commit")
		return
	}
	root, err := repoRoot()
	if err != nil {
		writeErr(w, 500, "repo root: "+err.Error())
		return
	}
	ext := ".sql"
	if b.Language == "python" {
		ext = ".py"
	}
	base := safeBase(b.FileName)
	base = strings.TrimSuffix(base, filepath.Ext(base))
	relDir := filepath.Join("conversions", fmt.Sprintf("ws%d", ws))
	relPath := filepath.Join(relDir, base+ext)
	absDir := filepath.Join(root, relDir)
	absPath := filepath.Join(root, relPath)
	if err := os.MkdirAll(absDir, 0o755); err != nil {
		writeErr(w, 500, "mkdir: "+err.Error())
		return
	}
	if err := os.WriteFile(absPath, []byte(b.Code), 0o644); err != nil {
		writeErr(w, 500, "write: "+err.Error())
		return
	}
	add := exec.Command("git", "-C", root, "add", "--", relPath)
	if out, err := add.CombinedOutput(); err != nil {
		writeJSON(w, 200, map[string]interface{}{"committed": false, "path": relPath, "output": string(out) + err.Error()})
		return
	}
	msg := fmt.Sprintf("Add converted artifact %s (workspace %d)", base+ext, ws)
	commit := exec.Command("git", "-C", root, "commit", "-m", msg, "--", relPath)
	out, err := commit.CombinedOutput()
	if err != nil {
		// Non-fatal: file is written+staged, but commit may fail (e.g. nothing changed).
		writeJSON(w, 200, map[string]interface{}{"committed": false, "path": relPath, "output": strings.TrimSpace(string(out))})
		return
	}
	writeJSON(w, 200, map[string]interface{}{"committed": true, "path": relPath, "output": strings.TrimSpace(string(out))})
}

// ===========================================================================
// Databricks deploy (notebook) + job (user-triggered only)
// ===========================================================================
func (h *Handler) dbxConnAuth(ws int64) (base, token, warehouse string, err error) {
	var host, user, pass, httpPath, extra string
	e := h.ctrl.QueryRow(`
		SELECT host, username, password_enc, COALESCE(http_path,''), COALESCE(extra_config,'{}')
		FROM data_connections WHERE workspace_id=? AND db_type='DATABRICKS' ORDER BY connection_id DESC LIMIT 1`, ws).
		Scan(&host, &user, &pass, &httpPath, &extra)
	if e != nil {
		return "", "", "", fmt.Errorf("no Databricks connection configured for this workspace")
	}
	pass, e = h.box.Open(pass)
	if e != nil {
		return "", "", "", fmt.Errorf("could not read connection secret")
	}
	var ex dbxExtra
	_ = json.Unmarshal([]byte(extra), &ex)
	base = "https://" + normalizeHost(host)
	warehouse = strings.TrimSpace(ex.WarehouseID)
	if warehouse == "" {
		warehouse = warehouseFromPath(httpPath)
	}
	client := &http.Client{Timeout: 25 * time.Second}
	if strings.ToLower(ex.AuthType) == "oauth" {
		cid := ex.ClientID
		if cid == "" {
			cid = user
		}
		token, err = databricksOAuthToken(client, base, cid, pass)
		return base, token, warehouse, err
	}
	return base, strings.TrimSpace(pass), warehouse, nil
}

func (h *Handler) ConversionDeploy(w http.ResponseWriter, r *http.Request) {
	ws, _ := wsID(r)
	var b struct {
		FileName string `json:"fileName"`
		Code     string `json:"code"`
		Language string `json:"language"`
	}
	if err := json.NewDecoder(r.Body).Decode(&b); err != nil {
		writeErr(w, 400, "invalid body")
		return
	}
	base, token, _, err := h.dbxConnAuth(ws)
	if err != nil {
		writeJSON(w, 200, map[string]interface{}{"deployed": false, "error": err.Error()})
		return
	}
	lang := "SQL"
	if b.Language == "python" {
		lang = "PYTHON"
	}
	name := strings.TrimSuffix(safeBase(b.FileName), filepath.Ext(safeBase(b.FileName)))
	dir := "/Shared/reveal_migrations/migration_demo"
	nbPath := dir + "/" + name
	client := &http.Client{Timeout: 30 * time.Second}

	// Ensure the parent folder exists.
	_ = dbxPost(client, base, token, "/api/2.0/workspace/mkdirs", map[string]interface{}{"path": dir}, nil)

	importBody := map[string]interface{}{
		"path":      nbPath,
		"format":    "SOURCE",
		"language":  lang,
		"overwrite": true,
		"content":   base64.StdEncoding.EncodeToString([]byte(b.Code)),
	}
	var importResp map[string]interface{}
	if err := dbxPost(client, base, token, "/api/2.0/workspace/import", importBody, &importResp); err != nil {
		writeJSON(w, 200, map[string]interface{}{"deployed": false, "error": err.Error(), "notebookPath": nbPath})
		return
	}
	writeJSON(w, 200, map[string]interface{}{"deployed": true, "notebookPath": nbPath})
}

func (h *Handler) ConversionJob(w http.ResponseWriter, r *http.Request) {
	ws, _ := wsID(r)
	var b struct {
		Name         string `json:"name"`
		NotebookPath string `json:"notebookPath"`
		Run          bool   `json:"run"`
	}
	if err := json.NewDecoder(r.Body).Decode(&b); err != nil {
		writeErr(w, 400, "invalid body")
		return
	}
	if strings.TrimSpace(b.NotebookPath) == "" {
		writeErr(w, 400, "notebookPath required (deploy first)")
		return
	}
	base, token, _, err := h.dbxConnAuth(ws)
	if err != nil {
		writeJSON(w, 200, map[string]interface{}{"created": false, "error": err.Error()})
		return
	}
	client := &http.Client{Timeout: 30 * time.Second}
	jobName := "RevealDQ · " + b.Name

	// Serverless notebook task (no cluster spec) — relies on serverless jobs compute
	// being enabled in the workspace. If not, Databricks returns a clear error.
	task := map[string]interface{}{
		"task_key":      "convert_run",
		"notebook_task": map[string]interface{}{"notebook_path": b.NotebookPath},
	}
	createBody := map[string]interface{}{
		"name":  jobName,
		"tasks": []interface{}{task},
	}
	var createResp struct {
		JobID int64 `json:"job_id"`
	}
	if err := dbxPost(client, base, token, "/api/2.1/jobs/create", createBody, &createResp); err != nil {
		writeJSON(w, 200, map[string]interface{}{"created": false, "error": err.Error()})
		return
	}
	out := map[string]interface{}{"created": true, "jobId": fmt.Sprintf("%d", createResp.JobID)}

	if b.Run {
		var runResp struct {
			RunID int64 `json:"run_id"`
		}
		if err := dbxPost(client, base, token, "/api/2.1/jobs/run-now",
			map[string]interface{}{"job_id": createResp.JobID}, &runResp); err != nil {
			out["run"] = false
			out["runError"] = err.Error()
		} else {
			out["run"] = true
			out["runId"] = fmt.Sprintf("%d", runResp.RunID)
		}
	}
	writeJSON(w, 200, out)
}

func dbxPost(client *http.Client, base, token, path string, body interface{}, out interface{}) error {
	buf, _ := json.Marshal(body)
	req, _ := http.NewRequest(http.MethodPost, base+path, bytes.NewReader(buf))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	rb, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode >= 400 {
		return fmt.Errorf("HTTP %d: %s", resp.StatusCode, snippet(rb))
	}
	if out != nil {
		_ = json.Unmarshal(rb, out)
	}
	return nil
}
