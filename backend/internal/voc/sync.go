package voc

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"
)

// Engine runs the extraction → store → analyse → act pipeline.
//
//	extraction  : Provider (automation, this package)
//	analysis    : Python agent  POST /voc/analyze   (agentic)
//	actions     : Python agent  POST /voc/actions   (agentic)
type Engine struct {
	store *Store
	cfg   Config
	http  *http.Client
	mu    sync.Mutex
	busy  map[int64]bool // workspace → sync in progress
}

func NewEngine(store *Store, cfg Config) *Engine {
	return &Engine{
		store: store,
		cfg:   cfg,
		http:  &http.Client{Timeout: 90 * time.Second},
		busy:  map[int64]bool{},
	}
}

// DefaultWindow is how far back a sync looks when no explicit range is given
// and the company has never been synced: the last 5 days.
const DefaultWindow = 5 * 24 * time.Hour

// SyncOptions scopes a run.
type SyncOptions struct {
	CompanyID int64
	From, To  time.Time // extraction window; zero From → LastSync-48h or DefaultWindow, zero To → now
	Trigger   string    // MANUAL | SCHEDULE
	UserID    int64
	Provider  string // optional: force a source type (e.g. SAMPLE) instead of the preferred one
}

// SyncResult summarises a run for the API response.
type SyncResult struct {
	RunIDs         []int64  `json:"runIds"`
	Companies      int      `json:"companies"`
	ItemsFetched   int      `json:"itemsFetched"`
	ItemsNew       int      `json:"itemsNew"`
	ItemsAnalysed  int      `json:"itemsAnalysed"`
	ActionsCreated int      `json:"actionsCreated"`
	Provider       string   `json:"provider"`
	Engine         string   `json:"engine"`
	Errors         []string `json:"errors"`
	Notes          []string `json:"notes"`
	Status         string   `json:"status"`
	WindowFrom     string   `json:"windowFrom"`
	WindowTo       string   `json:"windowTo"`
}

// Sync extracts and analyses for one company (opts.CompanyID>0) or all
// active companies in the workspace, inside the requested window.
func (e *Engine) Sync(ctx context.Context, ws int64, opts SyncOptions) (SyncResult, error) {
	companyID, uid, trigger := opts.CompanyID, opts.UserID, opts.Trigger
	if trigger == "" {
		trigger = "MANUAL"
	}
	e.mu.Lock()
	if e.busy[ws] {
		e.mu.Unlock()
		return SyncResult{}, fmt.Errorf("a sync is already running for this project")
	}
	e.busy[ws] = true
	e.mu.Unlock()
	defer func() {
		e.mu.Lock()
		delete(e.busy, ws)
		e.mu.Unlock()
	}()

	res := SyncResult{RunIDs: []int64{}, Errors: []string{}, Notes: []string{}}

	companies, err := e.store.ListCompanies(ws)
	if err != nil {
		return res, err
	}
	targets := []Company{}
	for _, c := range companies {
		if companyID > 0 && c.ID != companyID {
			continue
		}
		if c.IsActive || companyID > 0 {
			targets = append(targets, c)
		}
	}
	if len(targets) == 0 {
		return res, fmt.Errorf("no companies to sync — add a company first")
	}

	sources, err := e.ensurePlaywrightSource(ws)
	if err != nil {
		return res, err
	}
	if opts.Provider != "" {
		filtered := []Source{}
		for _, s := range sources {
			if strings.EqualFold(s.Provider, opts.Provider) {
				filtered = append(filtered, s)
			}
		}
		if len(filtered) == 0 {
			return res, fmt.Errorf("no active %s source in this project", strings.ToUpper(opts.Provider))
		}
		sources = filtered
	}
	src := pickSource(sources)
	provider, err := NewProvider(src, e.cfg, e.http)
	if err != nil {
		return res, err
	}
	res.Provider = provider.Name()
	res.Companies = len(targets)

	to := opts.To
	if to.IsZero() {
		to = time.Now().UTC()
	}
	res.WindowTo = to.UTC().Format(time.RFC3339)

	for _, c := range targets {
		cid := c.ID
		sid := src.ID
		runID := e.store.StartRun(ws, &cid, &sid, provider.Name(), trigger, uid)
		res.RunIDs = append(res.RunIDs, runID)

		from := opts.From
		if from.IsZero() {
			// Incremental: overlap the previous sync by 48h so late engagement is
			// refreshed. First sync: the default 5-day window.
			from = to.Add(-DefaultWindow)
			if c.LastSyncAt != nil && c.LastSyncAt.Add(-48*time.Hour).After(from) {
				from = c.LastSyncAt.Add(-48 * time.Hour)
			}
		}
		if res.WindowFrom == "" {
			res.WindowFrom = from.UTC().Format(time.RFC3339)
		}
		window := fmt.Sprintf("window %s → %s", from.UTC().Format("2006-01-02"), to.UTC().Format("2006-01-02"))

		items, ferr := provider.Fetch(ctx, c, from, to)
		if n, ok := provider.(Noter); ok {
			for _, note := range n.Notes() {
				res.Notes = append(res.Notes, c.Name+": "+note)
			}
		}
		if ferr != nil {
			msg := fmt.Sprintf("%s: %v", c.Name, ferr)
			res.Errors = append(res.Errors, msg)
			e.store.FinishRun(runID, "FAILED", 0, 0, 0, 0, window+" · "+ferr.Error())
			continue
		}
		newN, _, uerr := e.store.UpsertItems(ws, c.ID, src.ID, runID, items)
		if uerr != nil {
			res.Errors = append(res.Errors, fmt.Sprintf("%s: store: %v", c.Name, uerr))
			e.store.FinishRun(runID, "FAILED", len(items), newN, 0, 0, uerr.Error())
			continue
		}
		res.ItemsFetched += len(items)
		res.ItemsNew += newN
		e.store.TouchCompanySync(c.ID)

		analysed, actions, engine, aerr := e.AnalyseCompany(ctx, ws, c)
		res.ItemsAnalysed += analysed
		res.ActionsCreated += actions
		if engine != "" {
			res.Engine = engine
		}
		status := "SUCCESS"
		msg := fmt.Sprintf("%s · fetched %d, new %d, analysed %d, actions %d", window, len(items), newN, analysed, actions)
		if n, ok := provider.(Noter); ok && len(n.Notes()) > 0 {
			msg += " · " + strings.Join(n.Notes(), " | ")
		}
		if aerr != nil {
			status = "PARTIAL"
			msg += " · analysis: " + aerr.Error()
			res.Errors = append(res.Errors, fmt.Sprintf("%s: analysis: %v", c.Name, aerr))
		}
		e.store.FinishRun(runID, status, len(items), newN, analysed, actions, msg)
	}
	_, _ = e.store.db.Exec(`UPDATE voc_sources SET last_sync_at=NOW() WHERE source_id=?`, src.ID)

	switch {
	case len(res.Errors) == 0:
		res.Status = "SUCCESS"
	case res.ItemsFetched > 0:
		res.Status = "PARTIAL"
	default:
		res.Status = "FAILED"
	}
	return res, nil
}

// ensurePlaywrightSource returns the workspace's active sync sources, creating
// the zero-config default — LinkedIn public pages via Playwright — when the
// project does not have one yet. Extraction therefore works the moment a
// company is added, with no credentials or setup.
func (e *Engine) ensurePlaywrightSource(ws int64) ([]Source, error) {
	all, err := e.store.ListSources(ws, false)
	if err != nil {
		return nil, err
	}
	hasPW := false
	for _, s := range all {
		if s.Provider == "PLAYWRIGHT" {
			hasPW = true
			break
		}
	}
	if !hasPW {
		if _, cerr := e.store.CreateSource(ws, "PLAYWRIGHT", "LinkedIn public pages (Playwright)", "", map[string]interface{}{}); cerr != nil {
			return nil, fmt.Errorf("default scraper source could not be created: %w", cerr)
		}
	}
	sources, err := e.store.ActiveSyncSources(ws)
	if err != nil {
		return nil, err
	}
	if len(sources) == 0 {
		return nil, fmt.Errorf("no active extraction source — re-add the Playwright source under Sources & sync")
	}
	return sources, nil
}

// pickSource prefers the Playwright scraper, then a connected official API,
// then a data provider, then sample data.
func pickSource(list []Source) Source {
	rank := func(s Source) int {
		switch s.Provider {
		case "PLAYWRIGHT":
			return 0
		case "OFFICIAL_API":
			if s.Status == "CONNECTED" {
				return 1
			}
			return 4
		case "GENERIC_JSON":
			return 2
		case "SAMPLE":
			return 3
		}
		return 5
	}
	best := list[0]
	for _, s := range list[1:] {
		if rank(s) < rank(best) {
			best = s
		}
	}
	return best
}

// AnalyseCompany sends every un-analysed stored post to the agent in batches,
// persists results, then asks the agent for actions. Returns counts.
func (e *Engine) AnalyseCompany(ctx context.Context, ws int64, c Company) (analysed, actions int, engine string, err error) {
	for {
		pending, perr := e.store.PendingAnalysis(ws, c.ID, 25)
		if perr != nil {
			return analysed, actions, engine, perr
		}
		if len(pending) == 0 {
			break
		}
		results, eng, model, aerr := e.callAnalyze(ctx, c, pending)
		if aerr != nil {
			return analysed, actions, engine, aerr
		}
		engine = eng
		byExt := map[string]int64{}
		for _, p := range pending {
			byExt[p.ExternalID] = p.PostID
		}
		saved := 0
		for _, r := range results {
			pid, ok := byExt[r.ExternalID]
			if !ok {
				continue
			}
			if serr := e.store.SaveAnalysis(ws, pid, r, eng, model); serr == nil {
				saved++
			}
		}
		// Anything the agent skipped gets a neutral placeholder so we don't loop forever.
		for _, p := range pending {
			found := false
			for _, r := range results {
				if r.ExternalID == p.ExternalID {
					found = true
					break
				}
			}
			if !found {
				_ = e.store.SaveAnalysis(ws, p.PostID, Analysis{Sentiment: "NEUTRAL", Category: "OTHER", Summary: "Not classified by agent", Urgency: "LOW"}, "fallback", "")
			}
		}
		analysed += saved
		if len(pending) < 25 {
			break
		}
	}

	// Actions from the current picture of this company's negative / urgent signals.
	created, aerr := e.createActions(ctx, ws, c)
	actions += created
	return analysed, actions, engine, aerr
}

// ---- agent calls -------------------------------------------------------

type agentItem struct {
	ExternalID   string   `json:"externalId"`
	Kind         string   `json:"kind"`
	AuthorType   string   `json:"authorType"`
	AuthorName   string   `json:"authorName"`
	AuthorTitle  string   `json:"authorHeadline"`
	Content      string   `json:"content"`
	Hashtags     []string `json:"hashtags"`
	LikeCount    int      `json:"likeCount"`
	CommentCount int      `json:"commentCount"`
	PublishedAt  string   `json:"publishedAt"`
}

func (e *Engine) callAnalyze(ctx context.Context, c Company, posts []PostRow) ([]Analysis, string, string, error) {
	items := make([]agentItem, 0, len(posts))
	for _, p := range posts {
		items = append(items, agentItem{
			ExternalID: p.ExternalID, Kind: p.Kind, AuthorType: p.AuthorType, AuthorName: p.AuthorName, AuthorTitle: p.AuthorHeadline,
			Content: p.Content, Hashtags: p.Hashtags, LikeCount: p.LikeCount, CommentCount: p.CommentCount,
			PublishedAt: p.PublishedAt.UTC().Format(time.RFC3339),
		})
	}
	payload := map[string]interface{}{
		"company":  map[string]interface{}{"name": c.Name, "handle": c.Handle, "hashtags": c.Hashtags},
		"items":    items,
	}
	var out struct {
		Results   []Analysis `json:"results"`
		Engine    string     `json:"engine"`
		ModelUsed string     `json:"modelUsed"`
		Error     string     `json:"error"`
	}
	if err := e.postAgent(ctx, "/voc/analyze", payload, &out); err != nil {
		return nil, "", "", err
	}
	if out.Error != "" {
		return nil, "", "", fmt.Errorf("agent: %s", out.Error)
	}
	return out.Results, out.Engine, out.ModelUsed, nil
}

func (e *Engine) createActions(ctx context.Context, ws int64, c Company) (int, error) {
	// Feed the agent negative + high-urgency signals (bounded) plus counts.
	neg, _, err := e.store.ListPosts(ws, PostFilter{CompanyID: c.ID, Sentiment: "NEGATIVE", Limit: 60})
	if err != nil {
		return 0, err
	}
	pos, posTotal, _ := e.store.ListPosts(ws, PostFilter{CompanyID: c.ID, Sentiment: "POSITIVE", Limit: 15})
	if len(neg) == 0 && len(pos) == 0 {
		return 0, nil
	}
	type sig struct {
		ExternalID string   `json:"externalId"`
		Sentiment  string   `json:"sentiment"`
		Category   string   `json:"category"`
		Urgency    string   `json:"urgency"`
		Summary    string   `json:"summary"`
		Themes     []string `json:"themes"`
		Content    string   `json:"content"`
		Likes      int      `json:"likeCount"`
	}
	signals := []sig{}
	for _, p := range append(neg, pos...) {
		signals = append(signals, sig{p.ExternalID, p.Sentiment, p.Category, p.Urgency, p.Summary, p.Themes, truncate(p.Content, 400), p.LikeCount})
	}
	payload := map[string]interface{}{
		"company":       map[string]interface{}{"name": c.Name, "handle": c.Handle},
		"signals":       signals,
		"positiveTotal": posTotal,
		"negativeTotal": len(neg),
	}
	var out struct {
		Actions []ActionProposal `json:"actions"`
		Error   string           `json:"error"`
	}
	if err := e.postAgent(ctx, "/voc/actions", payload, &out); err != nil {
		return 0, err
	}
	if out.Error != "" {
		return 0, fmt.Errorf("agent: %s", out.Error)
	}
	created := 0
	for _, a := range out.Actions {
		if ok, err := e.store.UpsertAgentAction(ws, c.ID, a); err == nil && ok {
			created++
		}
	}
	return created, nil
}

// Ask forwards a natural-language question to the VoC analyser agent with a
// compact view of the project's signals.
func (e *Engine) Ask(ctx context.Context, ws int64, question string, companyID int64, model string) (map[string]interface{}, error) {
	stats, err := e.store.Stats(ws)
	if err != nil {
		return nil, err
	}
	sample, _, err := e.store.ListPosts(ws, PostFilter{CompanyID: companyID, Limit: 80})
	if err != nil {
		return nil, err
	}
	type sig struct {
		Company   string `json:"company"`
		Kind      string `json:"kind"`
		Sentiment string `json:"sentiment"`
		Category  string `json:"category"`
		Summary   string `json:"summary"`
		Content   string `json:"content"`
		Published string `json:"publishedAt"`
		Likes     int    `json:"likeCount"`
	}
	sigs := make([]sig, 0, len(sample))
	for _, p := range sample {
		sigs = append(sigs, sig{p.CompanyName, p.Kind, p.Sentiment, p.Category, p.Summary, truncate(p.Content, 300), p.PublishedAt.Format("2006-01-02"), p.LikeCount})
	}
	actions, _ := e.store.ListActions(ws, "")
	payload := map[string]interface{}{
		"question": question,
		"stats":    stats,
		"signals":  sigs,
		"actions":  actions,
		"model":    model,
	}
	var out map[string]interface{}
	if err := e.postAgent(ctx, "/voc/ask", payload, &out); err != nil {
		return nil, err
	}
	return out, nil
}

func (e *Engine) postAgent(ctx context.Context, path string, payload interface{}, out interface{}) error {
	body, _ := json.Marshal(payload)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(e.cfg.AgentURL, "/")+path, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := e.http.Do(req)
	if err != nil {
		return fmt.Errorf("agent unreachable at %s: %w", e.cfg.AgentURL, err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if resp.StatusCode >= 300 {
		return fmt.Errorf("agent %s → %d: %s", path, resp.StatusCode, truncate(string(raw), 300))
	}
	return json.Unmarshal(raw, out)
}

// ---- scheduler ---------------------------------------------------------

// StartScheduler runs due workspace syncs every minute until ctx is done.
func (e *Engine) StartScheduler(ctx context.Context) {
	go func() {
		t := time.NewTicker(60 * time.Second)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				due, err := e.store.DueSchedules()
				if err != nil {
					log.Printf("voc scheduler: %v", err)
					continue
				}
				for _, ws := range due {
					// Mark first so a slow run cannot be double-triggered.
					e.store.MarkScheduleRun(ws, "RUNNING")
					res, err := e.Sync(ctx, ws, SyncOptions{Trigger: "SCHEDULE"})
					status := res.Status
					if err != nil {
						status = "FAILED"
						log.Printf("voc scheduler ws=%d: %v", ws, err)
					}
					e.store.MarkScheduleRun(ws, status)
					log.Printf("voc scheduler ws=%d status=%s fetched=%d new=%d analysed=%d actions=%d",
						ws, status, res.ItemsFetched, res.ItemsNew, res.ItemsAnalysed, res.ActionsCreated)
				}
			}
		}
	}()
}
