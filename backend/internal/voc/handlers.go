package voc

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/harish-aiproject-lib/AgenticDataGov/backend/internal/middleware"
)

// Handler exposes the VoC module over HTTP. Mounted under
// /api/v1/workspaces/{id}/voc/* (JWT) plus the public OAuth callback.
type Handler struct {
	store  *Store
	engine *Engine
	cfg    Config
	http   *http.Client
}

func NewHandler(store *Store, engine *Engine, cfg Config) *Handler {
	return &Handler{store: store, engine: engine, cfg: cfg, http: &http.Client{Timeout: 30 * time.Second}}
}

// Routes mounts the authenticated VoC routes on a router already scoped to
// /workspaces/{id}/voc. (Mounting at /workspaces/{id} would shadow the
// platform's GET /workspaces/{id} — chi registers the mount path for all
// methods.)
func (h *Handler) Routes(r chi.Router) {
	r.Get("/overview", h.Overview)
	r.Get("/stats", h.Stats)

	r.Get("/companies", h.ListCompanies)
	r.Post("/companies", h.CreateCompany)
	r.Patch("/companies/{companyId}", h.UpdateCompany)
	r.Delete("/companies/{companyId}", h.DeleteCompany)

	r.Get("/sources", h.ListSources)
	r.Post("/sources", h.CreateSource)
	r.Delete("/sources/{sourceId}", h.DeleteSource)
	r.Post("/sources/{sourceId}/oauth/start", h.OAuthStart)
	r.Post("/sources/{sourceId}/import", h.Import)

	r.Post("/sync", h.Sync)
	r.Post("/reanalyse", h.Reanalyse)
	r.Get("/runs", h.Runs)
	r.Get("/schedule", h.GetSchedule)
	r.Put("/schedule", h.PutSchedule)

	r.Get("/posts", h.ListPosts)
	r.Get("/actions", h.ListActions)
	r.Post("/actions", h.CreateAction)
	r.Patch("/actions/{actionId}", h.UpdateAction)

	r.Post("/ask", h.Ask)
}

// ---- helpers ----

func writeJSON(w http.ResponseWriter, code int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

func writeErr(w http.ResponseWriter, code int, msg string) {
	writeJSON(w, code, map[string]string{"error": msg})
}

func wsID(r *http.Request) int64 {
	id, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	return id
}

func param64(r *http.Request, name string) int64 {
	id, _ := strconv.ParseInt(chi.URLParam(r, name), 10, 64)
	return id
}

func query64(r *http.Request, name string) int64 {
	id, _ := strconv.ParseInt(r.URL.Query().Get(name), 10, 64)
	return id
}

func decode(r *http.Request, v interface{}) error {
	defer r.Body.Close()
	return json.NewDecoder(io.LimitReader(r.Body, 32<<20)).Decode(v)
}

// ---- overview / stats ----

func (h *Handler) Stats(w http.ResponseWriter, r *http.Request) {
	st, err := h.store.Stats(wsID(r))
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, st)
}

// Overview bundles what the dashboard needs in one call.
func (h *Handler) Overview(w http.ResponseWriter, r *http.Request) {
	ws := wsID(r)
	st, err := h.store.Stats(ws)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	companies, _ := h.store.ListCompanies(ws)
	sources, _ := h.store.ListSources(ws, true)
	recent, _, _ := h.store.ListPosts(ws, PostFilter{Limit: 8})
	actions, _ := h.store.ListActions(ws, "OPEN")
	if len(actions) > 6 {
		actions = actions[:6]
	}
	writeJSON(w, 200, map[string]interface{}{
		"stats":        st,
		"companies":    companies,
		"sources":      sources,
		"recent":       recent,
		"openActions":  actions,
		"oauthReady":   h.cfg.oauthReady(),
		"accountEmail": h.cfg.LinkedInAccountEmail,
	})
}

// ---- companies ----

func (h *Handler) ListCompanies(w http.ResponseWriter, r *http.Request) {
	list, err := h.store.ListCompanies(wsID(r))
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, list)
}

func (h *Handler) CreateCompany(w http.ResponseWriter, r *http.Request) {
	var body struct {
		CompanyName    string   `json:"companyName"`
		LinkedinHandle string   `json:"linkedinHandle"`
		Hashtags       []string `json:"hashtags"`
		SyncNow        bool     `json:"syncNow"`
	}
	if err := decode(r, &body); err != nil {
		writeErr(w, 400, "invalid body")
		return
	}
	ws := wsID(r)
	uid := middleware.UserID(r.Context())
	id, err := h.store.CreateCompany(ws, uid, strings.TrimSpace(body.CompanyName), body.LinkedinHandle, body.Hashtags)
	if err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	c, _ := h.store.GetCompany(ws, id)
	resp := map[string]interface{}{"company": c}
	if body.SyncNow {
		res, serr := h.engine.Sync(r.Context(), ws, SyncOptions{CompanyID: id, Trigger: "MANUAL", UserID: uid})
		resp["sync"] = res
		if serr != nil {
			resp["syncError"] = serr.Error()
		}
	}
	writeJSON(w, 201, resp)
}

func (h *Handler) UpdateCompany(w http.ResponseWriter, r *http.Request) {
	var body struct {
		IsActive *bool `json:"isActive"`
	}
	if err := decode(r, &body); err != nil {
		writeErr(w, 400, "invalid body")
		return
	}
	if body.IsActive != nil {
		if err := h.store.UpdateCompanyActive(wsID(r), param64(r, "companyId"), *body.IsActive); err != nil {
			writeErr(w, 500, err.Error())
			return
		}
	}
	writeJSON(w, 200, map[string]string{"status": "ok"})
}

func (h *Handler) DeleteCompany(w http.ResponseWriter, r *http.Request) {
	if err := h.store.DeleteCompany(wsID(r), param64(r, "companyId")); err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, map[string]string{"status": "deleted"})
}

// ---- sources ----

func (h *Handler) ListSources(w http.ResponseWriter, r *http.Request) {
	list, err := h.store.ListSources(wsID(r), true)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, map[string]interface{}{
		"sources":      list,
		"oauthReady":   h.cfg.oauthReady(),
		"accountEmail": h.cfg.LinkedInAccountEmail,
		"redirectUri":  h.cfg.LinkedInRedirectURI,
	})
}

func (h *Handler) CreateSource(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Provider     string                 `json:"provider"`
		Label        string                 `json:"label"`
		AccountEmail string                 `json:"accountEmail"`
		Config       map[string]interface{} `json:"config"`
	}
	if err := decode(r, &body); err != nil {
		writeErr(w, 400, "invalid body")
		return
	}
	if body.Config == nil {
		body.Config = map[string]interface{}{}
	}
	if strings.EqualFold(body.Provider, "OFFICIAL_API") && body.AccountEmail == "" {
		body.AccountEmail = h.cfg.LinkedInAccountEmail
	}
	id, err := h.store.CreateSource(wsID(r), body.Provider, strings.TrimSpace(body.Label), body.AccountEmail, body.Config)
	if err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	src, _ := h.store.GetSource(id)
	src.Config = redactConfig(src.Config)
	writeJSON(w, 201, src)
}

func (h *Handler) DeleteSource(w http.ResponseWriter, r *http.Request) {
	if err := h.store.DeleteSource(wsID(r), param64(r, "sourceId")); err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, map[string]string{"status": "deleted"})
}

// OAuthStart returns the LinkedIn consent URL for the browser to open.
func (h *Handler) OAuthStart(w http.ResponseWriter, r *http.Request) {
	ws := wsID(r)
	sid := param64(r, "sourceId")
	src, err := h.store.GetSource(sid)
	if err != nil || src.WorkspaceID != ws {
		writeErr(w, 404, "source not found")
		return
	}
	if src.Provider != "OFFICIAL_API" {
		writeErr(w, 400, "only OFFICIAL_API sources use OAuth")
		return
	}
	u, err := h.cfg.AuthorizeURL(ws, sid, middleware.UserID(r.Context()))
	if err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	writeJSON(w, 200, map[string]string{"url": u})
}

// OAuthCallback is the public redirect target registered with LinkedIn.
func (h *Handler) OAuthCallback(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	redirectUI := func(ws int64, status, msg string) {
		target := strings.TrimRight(h.cfg.UIURL, "/") + fmt.Sprintf("/workspaces/%d/voc/sources?linkedin=%s", ws, status)
		if msg != "" {
			target += "&message=" + url.QueryEscape(msg)
		}
		http.Redirect(w, r, target, http.StatusFound)
	}
	st, err := h.cfg.verifyState(q.Get("state"))
	if err != nil {
		writeErr(w, 400, "invalid OAuth state: "+err.Error())
		return
	}
	if e := q.Get("error"); e != "" {
		redirectUI(st.WorkspaceID, "error", e+": "+q.Get("error_description"))
		return
	}
	cfg, email, err := h.cfg.exchangeCode(h.http, q.Get("code"))
	if err != nil {
		redirectUI(st.WorkspaceID, "error", err.Error())
		return
	}
	src, err := h.store.GetSource(st.SourceID)
	if err != nil {
		redirectUI(st.WorkspaceID, "error", "source no longer exists")
		return
	}
	for k, v := range cfg {
		src.Config[k] = v
	}
	if err := h.store.UpdateSourceConfig(src.ID, src.Config, "CONNECTED"); err != nil {
		redirectUI(st.WorkspaceID, "error", err.Error())
		return
	}
	if email != "" {
		_, _ = h.store.db.Exec(`UPDATE voc_sources SET account_email=? WHERE source_id=?`, email, src.ID)
	}
	redirectUI(st.WorkspaceID, "connected", "")
}

// Import ingests normalised items (JSON array or {"items":[...]}) for a company.
func (h *Handler) Import(w http.ResponseWriter, r *http.Request) {
	ws := wsID(r)
	sid := param64(r, "sourceId")
	companyID := query64(r, "companyId")
	if companyID == 0 {
		writeErr(w, 400, "companyId query parameter is required")
		return
	}
	c, err := h.store.GetCompany(ws, companyID)
	if err != nil {
		writeErr(w, 404, "company not found")
		return
	}
	defer r.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(r.Body, 32<<20))
	items, err := ParseItems(raw)
	if err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	uid := middleware.UserID(r.Context())
	cid := c.ID
	runID := h.store.StartRun(ws, &cid, &sid, "IMPORT", "MANUAL", uid)
	newN, _, err := h.store.UpsertItems(ws, c.ID, sid, runID, items)
	if err != nil {
		h.store.FinishRun(runID, "FAILED", len(items), newN, 0, 0, err.Error())
		writeErr(w, 500, err.Error())
		return
	}
	h.store.TouchCompanySync(c.ID)
	analysed, actions, _, aerr := h.engine.AnalyseCompany(r.Context(), ws, c)
	status, msg := "SUCCESS", fmt.Sprintf("imported %d, new %d, analysed %d, actions %d", len(items), newN, analysed, actions)
	if aerr != nil {
		status = "PARTIAL"
		msg += " · analysis: " + aerr.Error()
	}
	h.store.FinishRun(runID, status, len(items), newN, analysed, actions, msg)
	writeJSON(w, 200, map[string]interface{}{
		"itemsFetched": len(items), "itemsNew": newN, "itemsAnalysed": analysed, "actionsCreated": actions,
		"status": status, "message": msg,
	})
}

// ---- sync / analysis ----

// Sync triggers an extraction + analysis run.
//
//	{ "companyId": 0, "from": "2026-09-12", "to": "2026-09-17", "provider": "" }
//
// Dates accept YYYY-MM-DD (inclusive, whole days, UTC) or RFC 3339. Omitted
// `from` → incremental since the last sync (5-day window on first run);
// omitted `to` → now. `provider` optionally forces a source type (e.g.
// "SAMPLE" for demos); by default the Playwright scraper is used.
func (h *Handler) Sync(w http.ResponseWriter, r *http.Request) {
	var body struct {
		CompanyID int64  `json:"companyId"`
		From      string `json:"from"`
		To        string `json:"to"`
		Provider  string `json:"provider"` // optional override, e.g. "SAMPLE"
	}
	_ = decode(r, &body)
	from, err := parseDateBound(body.From, false)
	if err != nil {
		writeErr(w, 400, "from: "+err.Error())
		return
	}
	to, err := parseDateBound(body.To, true)
	if err != nil {
		writeErr(w, 400, "to: "+err.Error())
		return
	}
	if !from.IsZero() && !to.IsZero() && from.After(to) {
		writeErr(w, 400, "from must be before to")
		return
	}
	if !from.IsZero() && time.Since(from) > 366*24*time.Hour {
		writeErr(w, 400, "from cannot be more than a year back")
		return
	}
	res, err := h.engine.Sync(r.Context(), wsID(r), SyncOptions{
		CompanyID: body.CompanyID, From: from, To: to, Trigger: "MANUAL", UserID: middleware.UserID(r.Context()),
		Provider: strings.TrimSpace(body.Provider),
	})
	if err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	writeJSON(w, 200, res)
}

// parseDateBound reads "" (zero), YYYY-MM-DD or RFC 3339. A date-only upper
// bound is pushed to the end of that day so the range is inclusive.
func parseDateBound(s string, upper bool) (time.Time, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return time.Time{}, nil
	}
	if t, err := time.Parse(time.RFC3339, s); err == nil {
		return t.UTC(), nil
	}
	t, err := time.Parse("2006-01-02", s)
	if err != nil {
		return time.Time{}, fmt.Errorf("expected YYYY-MM-DD or RFC 3339, got %q", s)
	}
	if upper {
		t = t.Add(24*time.Hour - time.Second)
	}
	return t.UTC(), nil
}

// Reanalyse drops existing analysis (optionally for one company) and re-runs
// the agent — useful after switching models or prompts.
func (h *Handler) Reanalyse(w http.ResponseWriter, r *http.Request) {
	var body struct {
		CompanyID int64 `json:"companyId"`
	}
	_ = decode(r, &body)
	ws := wsID(r)
	if err := h.store.DeleteAnalysis(ws, body.CompanyID); err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	companies, _ := h.store.ListCompanies(ws)
	total, actions := 0, 0
	errs := []string{}
	for _, c := range companies {
		if body.CompanyID > 0 && c.ID != body.CompanyID {
			continue
		}
		n, a, _, err := h.engine.AnalyseCompany(r.Context(), ws, c)
		total += n
		actions += a
		if err != nil {
			errs = append(errs, c.Name+": "+err.Error())
		}
	}
	writeJSON(w, 200, map[string]interface{}{"itemsAnalysed": total, "actionsCreated": actions, "errors": errs})
}

func (h *Handler) Runs(w http.ResponseWriter, r *http.Request) {
	list, err := h.store.ListRuns(wsID(r), int(query64(r, "limit")))
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, list)
}

func (h *Handler) GetSchedule(w http.ResponseWriter, r *http.Request) {
	sc, err := h.store.GetSchedule(wsID(r))
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, sc)
}

func (h *Handler) PutSchedule(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Enabled        bool `json:"enabled"`
		CadenceMinutes int  `json:"cadenceMinutes"`
	}
	if err := decode(r, &body); err != nil {
		writeErr(w, 400, "invalid body")
		return
	}
	sc, err := h.store.PutSchedule(wsID(r), body.Enabled, body.CadenceMinutes)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, sc)
}

// ---- posts ----

func (h *Handler) ListPosts(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	f := PostFilter{
		CompanyID: query64(r, "companyId"),
		Sentiment: q.Get("sentiment"),
		Category:  q.Get("category"),
		Kind:      q.Get("kind"),
		Search:    strings.TrimSpace(q.Get("q")),
		Limit:     int(query64(r, "limit")),
		Offset:    int(query64(r, "offset")),
	}
	list, total, err := h.store.ListPosts(wsID(r), f)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, map[string]interface{}{"items": list, "total": total})
}

// ---- actions ----

func (h *Handler) ListActions(w http.ResponseWriter, r *http.Request) {
	list, err := h.store.ListActions(wsID(r), r.URL.Query().Get("status"))
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, list)
}

func (h *Handler) CreateAction(w http.ResponseWriter, r *http.Request) {
	var body struct {
		CompanyID   *int64 `json:"companyId"`
		Title       string `json:"title"`
		Description string `json:"description"`
		Category    string `json:"category"`
		Priority    string `json:"priority"`
	}
	if err := decode(r, &body); err != nil || strings.TrimSpace(body.Title) == "" {
		writeErr(w, 400, "title is required")
		return
	}
	id, err := h.store.CreateManualAction(wsID(r), body.CompanyID, body.Title, body.Description, body.Category, body.Priority)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	writeJSON(w, 201, map[string]interface{}{"actionId": id})
}

func (h *Handler) UpdateAction(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Status   string `json:"status"`
		Owner    string `json:"owner"`
		Priority string `json:"priority"`
	}
	if err := decode(r, &body); err != nil {
		writeErr(w, 400, "invalid body")
		return
	}
	if err := h.store.UpdateAction(wsID(r), param64(r, "actionId"), body.Status, body.Owner, body.Priority); err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, map[string]string{"status": "ok"})
}

// ---- agent Q&A ----

func (h *Handler) Ask(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Question  string `json:"question"`
		CompanyID int64  `json:"companyId"`
		Model     string `json:"model"`
	}
	if err := decode(r, &body); err != nil || strings.TrimSpace(body.Question) == "" {
		writeErr(w, 400, "question is required")
		return
	}
	out, err := h.engine.Ask(r.Context(), wsID(r), body.Question, body.CompanyID, body.Model)
	if err != nil {
		writeErr(w, 502, err.Error())
		return
	}
	writeJSON(w, 200, out)
}

var _ = sql.ErrNoRows
