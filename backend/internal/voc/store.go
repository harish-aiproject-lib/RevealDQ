package voc

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// Config is the subset of platform configuration the VoC module needs.
type Config struct {
	AgentURL             string
	JWTSecret            string
	UIURL                string
	LinkedInClientID     string
	LinkedInClientSecret string
	LinkedInRedirectURI  string
	LinkedInAccountEmail string
}

// Store wraps the VOC_DB connection.
type Store struct {
	db *sql.DB
}

func NewStore(db *sql.DB) *Store { return &Store{db: db} }

func jsonOrNull(v interface{}) interface{} {
	if v == nil {
		return nil
	}
	b, err := json.Marshal(v)
	if err != nil {
		return nil
	}
	return string(b)
}

func unmarshalStrings(raw []byte) []string {
	out := []string{}
	if len(raw) > 0 {
		_ = json.Unmarshal(raw, &out)
	}
	return out
}

func unmarshalMap(raw []byte) map[string]interface{} {
	out := map[string]interface{}{}
	if len(raw) > 0 {
		_ = json.Unmarshal(raw, &out)
	}
	return out
}

// ---------------- companies ----------------

func (s *Store) ListCompanies(ws int64) ([]Company, error) {
	rows, err := s.db.Query(`
		SELECT c.company_id, c.workspace_id, c.company_name, c.linkedin_handle, COALESCE(c.linkedin_urn,''),
		       c.hashtags, c.is_active, c.last_sync_at, c.created_at,
		       (SELECT COUNT(*) FROM voc_posts p WHERE p.company_id = c.company_id) AS post_count
		FROM voc_companies c WHERE c.workspace_id=? ORDER BY c.company_id`, ws)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	list := []Company{}
	for rows.Next() {
		var c Company
		var tags []byte
		var last sql.NullTime
		if err := rows.Scan(&c.ID, &c.WorkspaceID, &c.Name, &c.Handle, &c.URN, &tags, &c.IsActive, &last, &c.CreatedAt, &c.PostCount); err != nil {
			return nil, err
		}
		c.Hashtags = unmarshalStrings(tags)
		if last.Valid {
			t := last.Time
			c.LastSyncAt = &t
		}
		list = append(list, c)
	}
	return list, nil
}

func (s *Store) GetCompany(ws, id int64) (Company, error) {
	list, err := s.ListCompanies(ws)
	if err != nil {
		return Company{}, err
	}
	for _, c := range list {
		if c.ID == id {
			return c, nil
		}
	}
	return Company{}, sql.ErrNoRows
}

func (s *Store) CreateCompany(ws, uid int64, name, handle string, hashtags []string) (int64, error) {
	handle = normaliseHandle(handle)
	if name == "" || handle == "" {
		return 0, fmt.Errorf("companyName and linkedinHandle are required")
	}
	clean := []string{}
	for _, h := range hashtags {
		h = strings.ToLower(strings.TrimSpace(strings.TrimPrefix(h, "#")))
		if h != "" {
			clean = append(clean, h)
		}
	}
	if len(clean) == 0 {
		clean = []string{strings.ToLower(strings.ReplaceAll(name, " ", ""))}
	}
	res, err := s.db.Exec(`
		INSERT INTO voc_companies (workspace_id, company_name, linkedin_handle, hashtags, created_by)
		VALUES (?,?,?,?,?)`, ws, name, handle, jsonOrNull(clean), uid)
	if err != nil {
		if strings.Contains(err.Error(), "Duplicate") {
			return 0, fmt.Errorf("company with LinkedIn handle %q already exists in this project", handle)
		}
		return 0, err
	}
	id, _ := res.LastInsertId()
	return id, nil
}

func (s *Store) UpdateCompanyActive(ws, id int64, active bool) error {
	_, err := s.db.Exec(`UPDATE voc_companies SET is_active=? WHERE workspace_id=? AND company_id=?`, active, ws, id)
	return err
}

func (s *Store) DeleteCompany(ws, id int64) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	_, _ = tx.Exec(`DELETE a FROM voc_post_analysis a JOIN voc_posts p ON p.post_id=a.post_id WHERE p.workspace_id=? AND p.company_id=?`, ws, id)
	_, _ = tx.Exec(`DELETE FROM voc_posts WHERE workspace_id=? AND company_id=?`, ws, id)
	_, _ = tx.Exec(`DELETE FROM voc_actions WHERE workspace_id=? AND company_id=?`, ws, id)
	_, _ = tx.Exec(`DELETE FROM voc_sync_runs WHERE workspace_id=? AND company_id=?`, ws, id)
	if _, err := tx.Exec(`DELETE FROM voc_companies WHERE workspace_id=? AND company_id=?`, ws, id); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) SetCompanyURN(id int64, urn string) {
	_, _ = s.db.Exec(`UPDATE voc_companies SET linkedin_urn=? WHERE company_id=?`, urn, id)
}

func (s *Store) TouchCompanySync(id int64) {
	_, _ = s.db.Exec(`UPDATE voc_companies SET last_sync_at=NOW() WHERE company_id=?`, id)
}

// normaliseHandle accepts "acme", "@acme", or a full LinkedIn company URL.
func normaliseHandle(h string) string {
	h = strings.TrimSpace(h)
	h = strings.TrimPrefix(h, "@")
	lower := strings.ToLower(h)
	for _, marker := range []string{"linkedin.com/company/", "linkedin.com/school/", "linkedin.com/showcase/"} {
		if i := strings.Index(lower, marker); i >= 0 {
			h = h[i+len(marker):]
			break
		}
	}
	h = strings.Trim(h, "/")
	if i := strings.IndexAny(h, "/?#"); i >= 0 {
		h = h[:i]
	}
	return strings.ToLower(h)
}

// ---------------- sources ----------------

func (s *Store) ListSources(ws int64, redact bool) ([]Source, error) {
	rows, err := s.db.Query(`
		SELECT source_id, workspace_id, channel, provider, label, COALESCE(account_email,''), config, status, is_active, last_sync_at, created_at
		FROM voc_sources WHERE workspace_id=? ORDER BY source_id`, ws)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	list := []Source{}
	for rows.Next() {
		var src Source
		var cfg []byte
		var last sql.NullTime
		if err := rows.Scan(&src.ID, &src.WorkspaceID, &src.Channel, &src.Provider, &src.Label, &src.AccountEmail, &cfg, &src.Status, &src.IsActive, &last, &src.CreatedAt); err != nil {
			return nil, err
		}
		src.Config = unmarshalMap(cfg)
		if last.Valid {
			t := last.Time
			src.LastSyncAt = &t
		}
		if redact {
			src.Config = redactConfig(src.Config)
		}
		list = append(list, src)
	}
	return list, nil
}

func redactConfig(cfg map[string]interface{}) map[string]interface{} {
	out := map[string]interface{}{}
	for k, v := range cfg {
		switch k {
		case "accessToken", "refreshToken", "headerValue":
			if str, ok := v.(string); ok && str != "" {
				out[k] = "••••" + str[max(0, len(str)-4):]
			}
		default:
			out[k] = v
		}
	}
	return out
}

func (s *Store) CreateSource(ws int64, provider, label, email string, cfg map[string]interface{}) (int64, error) {
	provider = strings.ToUpper(strings.TrimSpace(provider))
	switch provider {
	case "PLAYWRIGHT", "OFFICIAL_API", "GENERIC_JSON", "IMPORT", "SAMPLE":
	default:
		return 0, fmt.Errorf("unknown provider %q", provider)
	}
	if label == "" {
		label = map[string]string{
			"OFFICIAL_API": "LinkedIn (official API)",
			"GENERIC_JSON": "LinkedIn data provider",
			"IMPORT":       "Manual import",
			"SAMPLE":       "Sample generator",
		}[provider]
	}
	status := "CONFIGURED"
	if provider == "SAMPLE" || provider == "IMPORT" {
		status = "CONNECTED"
	}
	res, err := s.db.Exec(`
		INSERT INTO voc_sources (workspace_id, channel, provider, label, account_email, config, status)
		VALUES (?,'LINKEDIN',?,?,?,?,?)`, ws, provider, label, nullIfEmpty(email), jsonOrNull(cfg), status)
	if err != nil {
		return 0, err
	}
	id, _ := res.LastInsertId()
	return id, nil
}

func (s *Store) UpdateSourceConfig(id int64, cfg map[string]interface{}, status string) error {
	_, err := s.db.Exec(`UPDATE voc_sources SET config=?, status=? WHERE source_id=?`, jsonOrNull(cfg), status, id)
	return err
}

func (s *Store) DeleteSource(ws, id int64) error {
	_, err := s.db.Exec(`DELETE FROM voc_sources WHERE workspace_id=? AND source_id=?`, ws, id)
	return err
}

func (s *Store) GetSource(id int64) (Source, error) {
	var src Source
	var cfg []byte
	var last sql.NullTime
	err := s.db.QueryRow(`
		SELECT source_id, workspace_id, channel, provider, label, COALESCE(account_email,''), config, status, is_active, last_sync_at, created_at
		FROM voc_sources WHERE source_id=?`, id).
		Scan(&src.ID, &src.WorkspaceID, &src.Channel, &src.Provider, &src.Label, &src.AccountEmail, &cfg, &src.Status, &src.IsActive, &last, &src.CreatedAt)
	if err != nil {
		return src, err
	}
	src.Config = unmarshalMap(cfg)
	if last.Valid {
		t := last.Time
		src.LastSyncAt = &t
	}
	return src, nil
}

// ActiveSyncSources returns sources that can be synced (not IMPORT), preferring
// a connected official API, then a data provider, then the sample generator.
func (s *Store) ActiveSyncSources(ws int64) ([]Source, error) {
	all, err := s.ListSources(ws, false)
	if err != nil {
		return nil, err
	}
	out := []Source{}
	for _, src := range all {
		if src.IsActive && src.Provider != "IMPORT" {
			out = append(out, src)
		}
	}
	return out, nil
}

func nullIfEmpty(s string) interface{} {
	if strings.TrimSpace(s) == "" {
		return nil
	}
	return s
}

// ---------------- posts ----------------

// UpsertItems stores raw items; returns the count of newly inserted rows and
// the internal post ids of new rows keyed by external id.
func (s *Store) UpsertItems(ws, companyID, sourceID, runID int64, items []Item) (int, map[string]int64, error) {
	newIDs := map[string]int64{}
	inserted := 0
	for _, it := range items {
		if strings.TrimSpace(it.ExternalID) == "" {
			continue
		}
		res, err := s.db.Exec(`
			INSERT INTO voc_posts (workspace_id, company_id, source_id, sync_run_id, channel, external_id, kind, parent_external_id,
			                       author_name, author_handle, author_headline, author_type, content, url, hashtags, mentions,
			                       like_count, comment_count, share_count, published_at, raw_json)
			VALUES (?,?,?,?,'LINKEDIN',?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)
			ON DUPLICATE KEY UPDATE
			  like_count=VALUES(like_count), comment_count=VALUES(comment_count), share_count=VALUES(share_count),
			  content=VALUES(content), raw_json=VALUES(raw_json)`,
			ws, companyID, sourceID, runID, it.ExternalID, it.Kind, nullIfEmpty(it.ParentExternalID),
			nullIfEmpty(it.AuthorName), nullIfEmpty(it.AuthorHandle), nullIfEmpty(truncate(it.AuthorHeadline, 300)), nullIfEmpty(it.AuthorType),
			it.Content, nullIfEmpty(truncate(it.URL, 600)), jsonOrNull(it.Hashtags), jsonOrNull(it.Mentions),
			it.LikeCount, it.CommentCount, it.ShareCount, it.PublishedAt.UTC(), jsonOrNull(it.Raw))
		if err != nil {
			return inserted, newIDs, err
		}
		// MySQL: affected rows = 1 for insert, 2 for update, 0 for no-op.
		if n, _ := res.RowsAffected(); n == 1 {
			id, _ := res.LastInsertId()
			newIDs[it.ExternalID] = id
			inserted++
		}
	}
	return inserted, newIDs, nil
}

// PostRow is the API/UI shape for a stored signal plus its analysis.
type PostRow struct {
	PostID          int64     `json:"postId"`
	CompanyID       int64     `json:"companyId"`
	CompanyName     string    `json:"companyName"`
	ExternalID      string    `json:"externalId"`
	Kind            string    `json:"kind"`
	ParentExternal  string    `json:"parentExternalId"`
	AuthorName      string    `json:"authorName"`
	AuthorHandle    string    `json:"authorHandle"`
	AuthorHeadline  string    `json:"authorHeadline"`
	AuthorType      string    `json:"authorType"`
	Content         string    `json:"content"`
	URL             string    `json:"url"`
	Hashtags        []string  `json:"hashtags"`
	Mentions        []string  `json:"mentions"`
	LikeCount       int       `json:"likeCount"`
	CommentCount    int       `json:"commentCount"`
	ShareCount      int       `json:"shareCount"`
	PublishedAt     time.Time `json:"publishedAt"`
	CollectedAt     time.Time `json:"collectedAt"`
	Sentiment       string    `json:"sentiment"`
	SentimentScore  *float64  `json:"sentimentScore"`
	Category        string    `json:"category"`
	Themes          []string  `json:"themes"`
	Summary         string    `json:"summary"`
	Urgency         string    `json:"urgency"`
	SuggestedAction string    `json:"suggestedAction"`
	Engine          string    `json:"engine"`
	Analysed        bool      `json:"analysed"`
}

type PostFilter struct {
	CompanyID int64
	Sentiment string
	Category  string
	Kind      string
	Search    string
	Limit     int
	Offset    int
	Unanalysed bool
}

func (s *Store) ListPosts(ws int64, f PostFilter) ([]PostRow, int, error) {
	where := []string{"p.workspace_id=?"}
	args := []interface{}{ws}
	if f.CompanyID > 0 {
		where = append(where, "p.company_id=?")
		args = append(args, f.CompanyID)
	}
	if f.Sentiment != "" {
		where = append(where, "a.sentiment=?")
		args = append(args, strings.ToUpper(f.Sentiment))
	}
	if f.Category != "" {
		where = append(where, "a.category=?")
		args = append(args, strings.ToUpper(f.Category))
	}
	if f.Kind != "" {
		where = append(where, "p.kind=?")
		args = append(args, strings.ToUpper(f.Kind))
	}
	if f.Search != "" {
		where = append(where, "(p.content LIKE ? OR p.author_name LIKE ?)")
		like := "%" + f.Search + "%"
		args = append(args, like, like)
	}
	if f.Unanalysed {
		where = append(where, "a.post_id IS NULL")
	}
	w := strings.Join(where, " AND ")

	var total int
	if err := s.db.QueryRow(`
		SELECT COUNT(*) FROM voc_posts p LEFT JOIN voc_post_analysis a ON a.post_id=p.post_id WHERE `+w, args...).Scan(&total); err != nil {
		return nil, 0, err
	}
	limit := f.Limit
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	q := `
		SELECT p.post_id, p.company_id, c.company_name, p.external_id, p.kind, COALESCE(p.parent_external_id,''),
		       COALESCE(p.author_name,''), COALESCE(p.author_handle,''), COALESCE(p.author_headline,''), COALESCE(p.author_type,''),
		       COALESCE(p.content,''), COALESCE(p.url,''), p.hashtags, p.mentions, p.like_count, p.comment_count, p.share_count,
		       COALESCE(p.published_at, p.collected_at), p.collected_at,
		       COALESCE(a.sentiment,''), a.sentiment_score, COALESCE(a.category,''), a.themes, COALESCE(a.summary,''),
		       COALESCE(a.urgency,''), COALESCE(a.suggested_action,''), COALESCE(a.engine,''), a.post_id IS NOT NULL
		FROM voc_posts p
		JOIN voc_companies c ON c.company_id=p.company_id
		LEFT JOIN voc_post_analysis a ON a.post_id=p.post_id
		WHERE ` + w + ` ORDER BY p.published_at DESC, p.post_id DESC LIMIT ? OFFSET ?`
	args = append(args, limit, f.Offset)
	rows, err := s.db.Query(q, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	list := []PostRow{}
	for rows.Next() {
		var r PostRow
		var tags, mentions, themes []byte
		var score sql.NullFloat64
		if err := rows.Scan(&r.PostID, &r.CompanyID, &r.CompanyName, &r.ExternalID, &r.Kind, &r.ParentExternal,
			&r.AuthorName, &r.AuthorHandle, &r.AuthorHeadline, &r.AuthorType, &r.Content, &r.URL, &tags, &mentions,
			&r.LikeCount, &r.CommentCount, &r.ShareCount, &r.PublishedAt, &r.CollectedAt,
			&r.Sentiment, &score, &r.Category, &themes, &r.Summary, &r.Urgency, &r.SuggestedAction, &r.Engine, &r.Analysed); err != nil {
			return nil, 0, err
		}
		r.Hashtags = unmarshalStrings(tags)
		r.Mentions = unmarshalStrings(mentions)
		r.Themes = unmarshalStrings(themes)
		if score.Valid {
			v := score.Float64
			r.SentimentScore = &v
		}
		list = append(list, r)
	}
	return list, total, nil
}

// PendingAnalysis returns stored posts with no analysis row yet.
func (s *Store) PendingAnalysis(ws, companyID int64, limit int) ([]PostRow, error) {
	list, _, err := s.ListPosts(ws, PostFilter{CompanyID: companyID, Unanalysed: true, Limit: limit})
	return list, err
}

func (s *Store) SaveAnalysis(ws, postID int64, a Analysis, engine, model string) error {
	sent := strings.ToUpper(a.Sentiment)
	switch sent {
	case "POSITIVE", "NEGATIVE", "NEUTRAL", "BROADCAST":
	default:
		sent = "NEUTRAL"
	}
	urg := strings.ToUpper(a.Urgency)
	switch urg {
	case "LOW", "MEDIUM", "HIGH":
	default:
		urg = "LOW"
	}
	_, err := s.db.Exec(`
		INSERT INTO voc_post_analysis (post_id, workspace_id, sentiment, sentiment_score, category, themes, summary, urgency, suggested_action, engine, model_used)
		VALUES (?,?,?,?,?,?,?,?,?,?,?)
		ON DUPLICATE KEY UPDATE sentiment=VALUES(sentiment), sentiment_score=VALUES(sentiment_score), category=VALUES(category),
		  themes=VALUES(themes), summary=VALUES(summary), urgency=VALUES(urgency), suggested_action=VALUES(suggested_action),
		  engine=VALUES(engine), model_used=VALUES(model_used)`,
		postID, ws, sent, a.Score, strings.ToUpper(a.Category), jsonOrNull(a.Themes), truncate(a.Summary, 600), urg,
		truncate(a.SuggestedAction, 400), engine, model)
	return err
}

func (s *Store) DeleteAnalysis(ws, companyID int64) error {
	if companyID > 0 {
		_, err := s.db.Exec(`DELETE a FROM voc_post_analysis a JOIN voc_posts p ON p.post_id=a.post_id WHERE a.workspace_id=? AND p.company_id=?`, ws, companyID)
		return err
	}
	_, err := s.db.Exec(`DELETE FROM voc_post_analysis WHERE workspace_id=?`, ws)
	return err
}

// ---------------- actions ----------------

type ActionRow struct {
	ActionID    int64     `json:"actionId"`
	CompanyID   *int64    `json:"companyId"`
	CompanyName string    `json:"companyName"`
	Title       string    `json:"title"`
	Description string    `json:"description"`
	Category    string    `json:"category"`
	Priority    string    `json:"priority"`
	Status      string    `json:"status"`
	Source      string    `json:"source"`
	Evidence    []string  `json:"evidence"`
	SignalCount int       `json:"signalCount"`
	Owner       string    `json:"owner"`
	CreatedAt   time.Time `json:"createdAt"`
	UpdatedAt   time.Time `json:"updatedAt"`
}

func (s *Store) ListActions(ws int64, status string) ([]ActionRow, error) {
	q := `
		SELECT a.action_id, a.company_id, COALESCE(c.company_name,''), a.title, COALESCE(a.description,''), COALESCE(a.category,''),
		       a.priority, a.status, a.source, a.evidence, a.signal_count, COALESCE(a.owner,''), a.created_at, a.updated_at
		FROM voc_actions a LEFT JOIN voc_companies c ON c.company_id=a.company_id
		WHERE a.workspace_id=?`
	args := []interface{}{ws}
	if status != "" {
		q += " AND a.status=?"
		args = append(args, strings.ToUpper(status))
	}
	q += " ORDER BY FIELD(a.status,'OPEN','IN_PROGRESS','DONE','DISMISSED'), FIELD(a.priority,'HIGH','MEDIUM','LOW'), a.action_id DESC"
	rows, err := s.db.Query(q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	list := []ActionRow{}
	for rows.Next() {
		var r ActionRow
		var cid sql.NullInt64
		var ev []byte
		if err := rows.Scan(&r.ActionID, &cid, &r.CompanyName, &r.Title, &r.Description, &r.Category, &r.Priority, &r.Status,
			&r.Source, &ev, &r.SignalCount, &r.Owner, &r.CreatedAt, &r.UpdatedAt); err != nil {
			return nil, err
		}
		if cid.Valid {
			v := cid.Int64
			r.CompanyID = &v
		}
		r.Evidence = unmarshalStrings(ev)
		list = append(list, r)
	}
	return list, nil
}

// UpsertAgentAction creates an action unless an OPEN/IN_PROGRESS one with the
// same dedupe key already exists — in which case the evidence is merged.
func (s *Store) UpsertAgentAction(ws, companyID int64, p ActionProposal) (bool, error) {
	if strings.TrimSpace(p.Title) == "" {
		return false, nil
	}
	prio := strings.ToUpper(p.Priority)
	switch prio {
	case "LOW", "MEDIUM", "HIGH":
	default:
		prio = "MEDIUM"
	}
	// Dedupe is always scoped to the company: the same root cause at two
	// companies must produce two actions.
	key := p.DedupeKey
	if key == "" {
		key = fmt.Sprintf("%s:%s", strings.ToUpper(p.Category), strings.ToLower(strings.TrimSpace(p.Title)))
	}
	key = truncate(fmt.Sprintf("%d:%s", companyID, key), 200)

	var existing int64
	var ev []byte
	err := s.db.QueryRow(`SELECT action_id, evidence FROM voc_actions WHERE workspace_id=? AND dedupe_key=? AND status IN ('OPEN','IN_PROGRESS') LIMIT 1`, ws, key).Scan(&existing, &ev)
	if err == nil {
		merged := unmarshalStrings(ev)
		seen := map[string]bool{}
		for _, e := range merged {
			seen[e] = true
		}
		for _, e := range p.Evidence {
			if !seen[e] {
				merged = append(merged, e)
			}
		}
		_, err = s.db.Exec(`UPDATE voc_actions SET evidence=?, signal_count=?, priority=IF(FIELD(?, 'LOW','MEDIUM','HIGH') > FIELD(priority,'LOW','MEDIUM','HIGH'), ?, priority) WHERE action_id=?`,
			jsonOrNull(merged), len(merged), prio, prio, existing)
		return false, err
	}
	_, err = s.db.Exec(`
		INSERT INTO voc_actions (workspace_id, company_id, title, description, category, priority, status, source, evidence, signal_count, dedupe_key)
		VALUES (?,?,?,?,?,?,'OPEN','AGENT',?,?,?)`,
		ws, companyID, truncate(p.Title, 300), p.Description, strings.ToUpper(p.Category), prio, jsonOrNull(p.Evidence), p.SignalCount, key)
	return err == nil, err
}

func (s *Store) CreateManualAction(ws int64, companyID *int64, title, desc, category, prio string) (int64, error) {
	prio = strings.ToUpper(prio)
	switch prio {
	case "LOW", "MEDIUM", "HIGH":
	default:
		prio = "MEDIUM"
	}
	res, err := s.db.Exec(`
		INSERT INTO voc_actions (workspace_id, company_id, title, description, category, priority, status, source)
		VALUES (?,?,?,?,?,?,'OPEN','MANUAL')`, ws, companyID, truncate(title, 300), desc, strings.ToUpper(category), prio)
	if err != nil {
		return 0, err
	}
	id, _ := res.LastInsertId()
	return id, nil
}

func (s *Store) UpdateAction(ws, id int64, status, owner, priority string) error {
	sets := []string{}
	args := []interface{}{}
	if status != "" {
		sets = append(sets, "status=?")
		args = append(args, strings.ToUpper(status))
	}
	if owner != "" {
		sets = append(sets, "owner=?")
		args = append(args, owner)
	}
	if priority != "" {
		sets = append(sets, "priority=?")
		args = append(args, strings.ToUpper(priority))
	}
	if len(sets) == 0 {
		return nil
	}
	args = append(args, ws, id)
	_, err := s.db.Exec(`UPDATE voc_actions SET `+strings.Join(sets, ",")+` WHERE workspace_id=? AND action_id=?`, args...)
	return err
}

// ---------------- sync runs + schedule ----------------

func (s *Store) StartRun(ws int64, companyID, sourceID *int64, provider, trigger string, uid int64) int64 {
	res, err := s.db.Exec(`
		INSERT INTO voc_sync_runs (workspace_id, company_id, source_id, provider, trigger_type, status, started_at, triggered_by)
		VALUES (?,?,?,?,?,'RUNNING',NOW(),?)`, ws, companyID, sourceID, provider, trigger, nullIfZero(uid))
	if err != nil {
		return 0
	}
	id, _ := res.LastInsertId()
	return id
}

func nullIfZero(v int64) interface{} {
	if v == 0 {
		return nil
	}
	return v
}

func (s *Store) FinishRun(runID int64, status string, fetched, newN, analysed, actions int, msg string) {
	_, _ = s.db.Exec(`
		UPDATE voc_sync_runs SET status=?, finished_at=NOW(), items_fetched=?, items_new=?, items_analysed=?, actions_created=?, message=?
		WHERE run_id=?`, status, fetched, newN, analysed, actions, truncate(msg, 2000), runID)
}

type RunRow struct {
	RunID          int64      `json:"runId"`
	CompanyID      *int64     `json:"companyId"`
	CompanyName    string     `json:"companyName"`
	Provider       string     `json:"provider"`
	Trigger        string     `json:"trigger"`
	Status         string     `json:"status"`
	StartedAt      *time.Time `json:"startedAt"`
	FinishedAt     *time.Time `json:"finishedAt"`
	ItemsFetched   int        `json:"itemsFetched"`
	ItemsNew       int        `json:"itemsNew"`
	ItemsAnalysed  int        `json:"itemsAnalysed"`
	ActionsCreated int        `json:"actionsCreated"`
	Message        string     `json:"message"`
}

func (s *Store) ListRuns(ws int64, limit int) ([]RunRow, error) {
	if limit <= 0 {
		limit = 50
	}
	rows, err := s.db.Query(`
		SELECT r.run_id, r.company_id, COALESCE(c.company_name,'All companies'), COALESCE(r.provider,''), r.trigger_type, r.status,
		       r.started_at, r.finished_at, r.items_fetched, r.items_new, r.items_analysed, r.actions_created, COALESCE(r.message,'')
		FROM voc_sync_runs r LEFT JOIN voc_companies c ON c.company_id=r.company_id
		WHERE r.workspace_id=? ORDER BY r.run_id DESC LIMIT ?`, ws, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	list := []RunRow{}
	for rows.Next() {
		var r RunRow
		var cid sql.NullInt64
		var st, fi sql.NullTime
		if err := rows.Scan(&r.RunID, &cid, &r.CompanyName, &r.Provider, &r.Trigger, &r.Status, &st, &fi,
			&r.ItemsFetched, &r.ItemsNew, &r.ItemsAnalysed, &r.ActionsCreated, &r.Message); err != nil {
			return nil, err
		}
		if cid.Valid {
			v := cid.Int64
			r.CompanyID = &v
		}
		if st.Valid {
			t := st.Time
			r.StartedAt = &t
		}
		if fi.Valid {
			t := fi.Time
			r.FinishedAt = &t
		}
		list = append(list, r)
	}
	return list, nil
}

type Schedule struct {
	WorkspaceID    int64      `json:"workspaceId"`
	Enabled        bool       `json:"enabled"`
	CadenceMinutes int        `json:"cadenceMinutes"`
	NextRunAt      *time.Time `json:"nextRunAt"`
	LastRunAt      *time.Time `json:"lastRunAt"`
	LastStatus     string     `json:"lastStatus"`
}

func (s *Store) GetSchedule(ws int64) (Schedule, error) {
	sc := Schedule{WorkspaceID: ws, CadenceMinutes: 360}
	var next, last sql.NullTime
	var status sql.NullString
	err := s.db.QueryRow(`SELECT enabled, cadence_minutes, next_run_at, last_run_at, last_status FROM voc_schedules WHERE workspace_id=?`, ws).
		Scan(&sc.Enabled, &sc.CadenceMinutes, &next, &last, &status)
	if err == sql.ErrNoRows {
		return sc, nil
	}
	if err != nil {
		return sc, err
	}
	if next.Valid {
		t := next.Time
		sc.NextRunAt = &t
	}
	if last.Valid {
		t := last.Time
		sc.LastRunAt = &t
	}
	sc.LastStatus = status.String
	return sc, nil
}

func (s *Store) PutSchedule(ws int64, enabled bool, cadence int) (Schedule, error) {
	if cadence < 15 {
		cadence = 15
	}
	if cadence > 7*24*60 {
		cadence = 7 * 24 * 60
	}
	_, err := s.db.Exec(`
		INSERT INTO voc_schedules (workspace_id, enabled, cadence_minutes, next_run_at)
		VALUES (?,?,?, IF(?, DATE_ADD(NOW(), INTERVAL ? MINUTE), NULL))
		ON DUPLICATE KEY UPDATE enabled=VALUES(enabled), cadence_minutes=VALUES(cadence_minutes),
		  next_run_at=IF(VALUES(enabled), DATE_ADD(NOW(), INTERVAL VALUES(cadence_minutes) MINUTE), NULL)`,
		ws, enabled, cadence, enabled, cadence)
	if err != nil {
		return Schedule{}, err
	}
	return s.GetSchedule(ws)
}

// DueSchedules returns workspace ids whose next_run_at has passed.
func (s *Store) DueSchedules() ([]int64, error) {
	rows, err := s.db.Query(`SELECT workspace_id FROM voc_schedules WHERE enabled=1 AND next_run_at IS NOT NULL AND next_run_at <= NOW()`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	ids := []int64{}
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err == nil {
			ids = append(ids, id)
		}
	}
	return ids, nil
}

func (s *Store) MarkScheduleRun(ws int64, status string) {
	_, _ = s.db.Exec(`
		UPDATE voc_schedules SET last_run_at=NOW(), last_status=?, next_run_at=DATE_ADD(NOW(), INTERVAL cadence_minutes MINUTE)
		WHERE workspace_id=?`, status, ws)
}

// ---------------- stats ----------------

type Stats struct {
	TotalPosts    int                 `json:"totalPosts"`
	Analysed      int                 `json:"analysed"`
	Companies     int                 `json:"companies"`
	OpenActions   int                 `json:"openActions"`
	HighUrgency   int                 `json:"highUrgency"`
	BySentiment   map[string]int      `json:"bySentiment"`
	ByCategory    map[string]int      `json:"byCategory"`
	ByKind        map[string]int      `json:"byKind"`
	ByCompany     []map[string]interface{} `json:"byCompany"`
	Timeline      []map[string]interface{} `json:"timeline"`
	TopHashtags   []map[string]interface{} `json:"topHashtags"`
	TopThemes     []map[string]interface{} `json:"topThemes"`
	LastRun       *RunRow             `json:"lastRun"`
	Schedule      Schedule            `json:"schedule"`
}

func (s *Store) Stats(ws int64) (Stats, error) {
	st := Stats{BySentiment: map[string]int{}, ByCategory: map[string]int{}, ByKind: map[string]int{},
		ByCompany: []map[string]interface{}{}, Timeline: []map[string]interface{}{},
		TopHashtags: []map[string]interface{}{}, TopThemes: []map[string]interface{}{}}

	_ = s.db.QueryRow(`SELECT COUNT(*) FROM voc_posts WHERE workspace_id=?`, ws).Scan(&st.TotalPosts)
	_ = s.db.QueryRow(`SELECT COUNT(*) FROM voc_post_analysis WHERE workspace_id=?`, ws).Scan(&st.Analysed)
	_ = s.db.QueryRow(`SELECT COUNT(*) FROM voc_companies WHERE workspace_id=?`, ws).Scan(&st.Companies)
	_ = s.db.QueryRow(`SELECT COUNT(*) FROM voc_actions WHERE workspace_id=? AND status IN ('OPEN','IN_PROGRESS')`, ws).Scan(&st.OpenActions)
	_ = s.db.QueryRow(`SELECT COUNT(*) FROM voc_post_analysis WHERE workspace_id=? AND urgency='HIGH'`, ws).Scan(&st.HighUrgency)

	group := func(q string, into map[string]int) {
		rows, err := s.db.Query(q, ws)
		if err != nil {
			return
		}
		defer rows.Close()
		for rows.Next() {
			var k string
			var n int
			if rows.Scan(&k, &n) == nil && k != "" {
				into[k] = n
			}
		}
	}
	group(`SELECT sentiment, COUNT(*) FROM voc_post_analysis WHERE workspace_id=? GROUP BY sentiment`, st.BySentiment)
	group(`SELECT COALESCE(category,''), COUNT(*) FROM voc_post_analysis WHERE workspace_id=? GROUP BY category`, st.ByCategory)
	group(`SELECT kind, COUNT(*) FROM voc_posts WHERE workspace_id=? GROUP BY kind`, st.ByKind)

	if rows, err := s.db.Query(`
		SELECT c.company_id, c.company_name, COUNT(p.post_id),
		       SUM(a.sentiment='POSITIVE'), SUM(a.sentiment='NEGATIVE'), SUM(a.sentiment='NEUTRAL'), SUM(a.sentiment='BROADCAST')
		FROM voc_companies c
		LEFT JOIN voc_posts p ON p.company_id=c.company_id
		LEFT JOIN voc_post_analysis a ON a.post_id=p.post_id
		WHERE c.workspace_id=? GROUP BY c.company_id, c.company_name ORDER BY c.company_id`, ws); err == nil {
		defer rows.Close()
		for rows.Next() {
			var id int64
			var name string
			var total int
			var pos, neg, neu, bro sql.NullInt64
			if rows.Scan(&id, &name, &total, &pos, &neg, &neu, &bro) == nil {
				st.ByCompany = append(st.ByCompany, map[string]interface{}{
					"companyId": id, "companyName": name, "posts": total,
					"positive": pos.Int64, "negative": neg.Int64, "neutral": neu.Int64, "broadcast": bro.Int64,
				})
			}
		}
	}

	if rows, err := s.db.Query(`
		SELECT DATE(p.published_at) d, SUM(a.sentiment='POSITIVE'), SUM(a.sentiment='NEGATIVE'), SUM(a.sentiment IN ('NEUTRAL','BROADCAST')), COUNT(*)
		FROM voc_posts p LEFT JOIN voc_post_analysis a ON a.post_id=p.post_id
		WHERE p.workspace_id=? AND p.published_at >= DATE_SUB(CURDATE(), INTERVAL 21 DAY)
		GROUP BY DATE(p.published_at) ORDER BY d`, ws); err == nil {
		defer rows.Close()
		for rows.Next() {
			var d time.Time
			var pos, neg, other sql.NullInt64
			var total int
			if rows.Scan(&d, &pos, &neg, &other, &total) == nil {
				st.Timeline = append(st.Timeline, map[string]interface{}{
					"date": d.Format("2006-01-02"), "positive": pos.Int64, "negative": neg.Int64, "other": other.Int64, "total": total,
				})
			}
		}
	}

	// Hashtags / themes: aggregate in Go (JSON arrays).
	tagCount := map[string]int{}
	themeCount := map[string]int{}
	if rows, err := s.db.Query(`SELECT p.hashtags, a.themes FROM voc_posts p LEFT JOIN voc_post_analysis a ON a.post_id=p.post_id WHERE p.workspace_id=?`, ws); err == nil {
		defer rows.Close()
		for rows.Next() {
			var tags, themes []byte
			if rows.Scan(&tags, &themes) == nil {
				for _, t := range unmarshalStrings(tags) {
					tagCount[t]++
				}
				for _, t := range unmarshalStrings(themes) {
					themeCount[strings.ToLower(t)]++
				}
			}
		}
	}
	st.TopHashtags = topN(tagCount, 10, "hashtag")
	st.TopThemes = topN(themeCount, 10, "theme")

	if runs, err := s.ListRuns(ws, 1); err == nil && len(runs) > 0 {
		st.LastRun = &runs[0]
	}
	st.Schedule, _ = s.GetSchedule(ws)
	return st, nil
}

func topN(m map[string]int, n int, key string) []map[string]interface{} {
	type kv struct {
		k string
		v int
	}
	arr := make([]kv, 0, len(m))
	for k, v := range m {
		arr = append(arr, kv{k, v})
	}
	// simple insertion sort — small maps
	for i := 1; i < len(arr); i++ {
		for j := i; j > 0 && arr[j].v > arr[j-1].v; j-- {
			arr[j], arr[j-1] = arr[j-1], arr[j]
		}
	}
	out := []map[string]interface{}{}
	for i := 0; i < len(arr) && i < n; i++ {
		out = append(out, map[string]interface{}{key: arr[i].k, "count": arr[i].v})
	}
	return out
}
