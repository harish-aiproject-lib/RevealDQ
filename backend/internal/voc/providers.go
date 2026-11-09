package voc

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// Provider is the extraction contract. Implementations are deterministic
// automation: no LLM, no reasoning — fetch, normalise, return.
type Provider interface {
	Name() string
	// Fetch returns items for the company published inside the window
	// [from, to]. A zero `from` means "no lower bound"; a zero `to` means now.
	Fetch(ctx context.Context, c Company, from, to time.Time) ([]Item, error)
}

// Noter is implemented by providers that have something to say about a run
// beyond the items (e.g. the scraper explaining where it stopped).
type Noter interface {
	Notes() []string
}

// Window normalises a fetch window and reports whether t is inside it.
type Window struct{ From, To time.Time }

func (w Window) Contains(t time.Time) bool {
	if !w.From.IsZero() && t.Before(w.From) {
		return false
	}
	if !w.To.IsZero() && t.After(w.To) {
		return false
	}
	return true
}

// NewProvider builds the provider for a source row.
func NewProvider(src Source, cfg Config, http *http.Client) (Provider, error) {
	switch strings.ToUpper(src.Provider) {
	case "PLAYWRIGHT":
		return &PlaywrightProvider{src: src, cfg: cfg}, nil
	case "OFFICIAL_API":
		return &OfficialProvider{src: src, cfg: cfg, http: http}, nil
	case "GENERIC_JSON":
		return &GenericJSONProvider{src: src, http: http}, nil
	case "SAMPLE":
		return &SampleProvider{}, nil
	case "IMPORT":
		return nil, fmt.Errorf("IMPORT sources are fed through the import endpoint, not synced")
	default:
		return nil, fmt.Errorf("unknown provider %q", src.Provider)
	}
}

// ---------------------------------------------------------------------------
// LinkedIn Community Management API (official, OAuth 2.0)
// ---------------------------------------------------------------------------

const (
	linkedinAPI     = "https://api.linkedin.com/rest"
	linkedinVersion = "202409"
)

// OfficialProvider talks to LinkedIn's official REST API using the OAuth token
// stored on the source. Scopes needed: r_organization_social,
// rw_organization_admin (for vanity → URN lookup and notifications).
//
// Covers:
//   - posts authored by the organisation            (/posts?q=author)
//   - comments on those posts                       (/socialActions/{urn}/comments)
//   - mentions of the organisation in others' posts (/organizationalEntityNotifications)
//
// Hashtag-wide search is not exposed by LinkedIn; use GENERIC_JSON for that.
type OfficialProvider struct {
	src  Source
	cfg  Config
	http *http.Client
}

func (p *OfficialProvider) Name() string { return "OFFICIAL_API" }

func (p *OfficialProvider) token() (string, error) {
	tok, _ := p.src.Config["accessToken"].(string)
	if tok == "" {
		return "", fmt.Errorf("LinkedIn source is not connected — complete the OAuth flow first")
	}
	if exp, ok := p.src.Config["expiresAt"].(string); ok && exp != "" {
		if t, err := time.Parse(time.RFC3339, exp); err == nil && time.Now().After(t) {
			return "", fmt.Errorf("LinkedIn access token expired — reconnect the source")
		}
	}
	return tok, nil
}

func (p *OfficialProvider) get(ctx context.Context, path string, out interface{}) error {
	tok, err := p.token()
	if err != nil {
		return err
	}
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, linkedinAPI+path, nil)
	req.Header.Set("Authorization", "Bearer "+tok)
	req.Header.Set("LinkedIn-Version", linkedinVersion)
	req.Header.Set("X-Restli-Protocol-Version", "2.0.0")
	resp, err := p.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if resp.StatusCode >= 300 {
		return fmt.Errorf("linkedin %s → %d: %s", path, resp.StatusCode, truncate(string(body), 300))
	}
	return json.Unmarshal(body, out)
}

// resolveURN turns a vanity handle into urn:li:organization:{id}.
func (p *OfficialProvider) resolveURN(ctx context.Context, c Company) (string, error) {
	if c.URN != "" {
		return c.URN, nil
	}
	var res struct {
		Elements []struct {
			ID int64 `json:"id"`
		} `json:"elements"`
	}
	q := "/organizations?q=vanityName&vanityName=" + url.QueryEscape(c.Handle)
	if err := p.get(ctx, q, &res); err != nil {
		return "", err
	}
	if len(res.Elements) == 0 {
		return "", fmt.Errorf("no LinkedIn organization found for handle %q", c.Handle)
	}
	return fmt.Sprintf("urn:li:organization:%d", res.Elements[0].ID), nil
}

func (p *OfficialProvider) Fetch(ctx context.Context, c Company, since, until time.Time) ([]Item, error) {
	items, err := p.fetch(ctx, c, since)
	if err != nil {
		return nil, err
	}
	return filterWindow(items, Window{From: since, To: until}), nil
}

func (p *OfficialProvider) fetch(ctx context.Context, c Company, since time.Time) ([]Item, error) {
	urn, err := p.resolveURN(ctx, c)
	if err != nil {
		return nil, err
	}
	var items []Item

	// 1) Organisation's own posts (broadcast signals).
	var posts struct {
		Elements []map[string]interface{} `json:"elements"`
	}
	if err := p.get(ctx, "/posts?q=author&author="+url.QueryEscape(urn)+"&count=50&sortBy=LAST_MODIFIED", &posts); err != nil {
		return nil, err
	}
	for _, el := range posts.Elements {
		it := officialPostToItem(el, c, urn)
		if !since.IsZero() && it.PublishedAt.Before(since) {
			continue
		}
		items = append(items, it)

		// 2) Comments on each post — the actual customer voice.
		var comments struct {
			Elements []map[string]interface{} `json:"elements"`
		}
		cpath := "/socialActions/" + url.PathEscape(it.ExternalID) + "/comments?count=100"
		if err := p.get(ctx, cpath, &comments); err == nil {
			for _, cel := range comments.Elements {
				ci := officialCommentToItem(cel, it.ExternalID, c)
				if !since.IsZero() && ci.PublishedAt.Before(since) {
					continue
				}
				items = append(items, ci)
			}
		}
	}

	// 3) Mentions of the organisation in other people's posts/comments.
	var notif struct {
		Elements []map[string]interface{} `json:"elements"`
	}
	npath := "/organizationalEntityNotifications?q=criteria&actions=List(SHARE_MENTION,COMMENT_MENTION)&organizationalEntity=" +
		url.QueryEscape(urn) + "&count=100"
	if err := p.get(ctx, npath, &notif); err == nil {
		for _, n := range notif.Elements {
			it := officialNotificationToItem(n, c, urn)
			if it.ExternalID == "" {
				continue
			}
			if !since.IsZero() && it.PublishedAt.Before(since) {
				continue
			}
			items = append(items, it)
		}
	}
	return items, nil
}

func officialPostToItem(el map[string]interface{}, c Company, orgURN string) Item {
	id, _ := el["id"].(string)
	commentary, _ := el["commentary"].(string)
	created := epochMillis(el, "createdAt", "publishedAt")
	it := Item{
		ExternalID:  id,
		Kind:        "POST",
		AuthorName:  c.Name,
		AuthorHandle: c.Handle,
		AuthorType:  "ORGANIZATION",
		Content:     commentary,
		URL:         "https://www.linkedin.com/feed/update/" + id,
		Hashtags:    extractHashtags(commentary),
		Mentions:    extractMentions(commentary),
		PublishedAt: created,
		Raw:         el,
	}
	if rs, ok := el["reshareContext"]; ok && rs != nil {
		it.Kind = "REPOST"
	}
	return it
}

func officialCommentToItem(el map[string]interface{}, parent string, c Company) Item {
	id, _ := el["$URN"].(string)
	if id == "" {
		id, _ = el["id"].(string)
	}
	text := ""
	if m, ok := el["message"].(map[string]interface{}); ok {
		text, _ = m["text"].(string)
	}
	actor, _ := el["actor"].(string)
	created := epochMillis(el, "created", "createdAt")
	authorType := "PERSON"
	if strings.Contains(actor, "organization") {
		authorType = "ORGANIZATION"
	}
	return Item{
		ExternalID:       id,
		Kind:             "COMMENT",
		ParentExternalID: parent,
		AuthorName:       actor, // resolving names requires extra profile calls; kept raw
		AuthorHandle:     actor,
		AuthorType:       authorType,
		Content:          text,
		Hashtags:         extractHashtags(text),
		Mentions:         append(extractMentions(text), c.Handle),
		PublishedAt:      created,
		Raw:              el,
	}
}

func officialNotificationToItem(el map[string]interface{}, c Company, orgURN string) Item {
	action, _ := el["action"].(string)
	target, _ := el["generatedActivity"].(string)
	if target == "" {
		target, _ = el["sourceActivity"].(string)
	}
	actor, _ := el["actor"].(string)
	created := epochMillis(el, "lastModifiedAt", "createdAt")
	kind := "POST"
	if strings.Contains(action, "COMMENT") {
		kind = "COMMENT"
	}
	text, _ := el["message"].(string)
	return Item{
		ExternalID:   target,
		Kind:         kind,
		AuthorName:   actor,
		AuthorHandle: actor,
		AuthorType:   "PERSON",
		Content:      text,
		Mentions:     []string{c.Handle},
		Hashtags:     extractHashtags(text),
		PublishedAt:  created,
		Raw:          el,
	}
}

// ---------------------------------------------------------------------------
// Generic JSON provider (any HTTP endpoint that returns normalised Items)
// ---------------------------------------------------------------------------

// GenericJSONProvider calls a configurable endpoint and expects either a bare
// JSON array of Item or {"items":[...]} — the same shape as the import
// endpoint. Use it to plug in a third-party LinkedIn data provider or your own
// hashtag-listening job. Placeholders in the endpoint: {handle} {hashtags}
// {company} {since}.
//
// config: { "endpoint": "https://…?handle={handle}&tags={hashtags}&since={since}",
//           "headerName": "Authorization", "headerValue": "Bearer …",
//           "method": "GET" }
type GenericJSONProvider struct {
	src  Source
	http *http.Client
}

func (p *GenericJSONProvider) Name() string { return "GENERIC_JSON" }

func (p *GenericJSONProvider) Fetch(ctx context.Context, c Company, since, until time.Time) ([]Item, error) {
	endpoint, _ := p.src.Config["endpoint"].(string)
	if endpoint == "" {
		return nil, fmt.Errorf("GENERIC_JSON source has no endpoint configured")
	}
	sinceStr, untilStr := "", ""
	if !since.IsZero() {
		sinceStr = since.UTC().Format(time.RFC3339)
	}
	if !until.IsZero() {
		untilStr = until.UTC().Format(time.RFC3339)
	}
	rep := strings.NewReplacer(
		"{handle}", url.QueryEscape(c.Handle),
		"{company}", url.QueryEscape(c.Name),
		"{hashtags}", url.QueryEscape(strings.Join(c.Hashtags, ",")),
		"{since}", url.QueryEscape(sinceStr),
		"{until}", url.QueryEscape(untilStr),
	)
	target := rep.Replace(endpoint)
	method, _ := p.src.Config["method"].(string)
	if method == "" {
		method = http.MethodGet
	}
	var body io.Reader
	if method == http.MethodPost {
		payload, _ := json.Marshal(map[string]interface{}{
			"handle": c.Handle, "company": c.Name, "hashtags": c.Hashtags, "since": sinceStr, "until": untilStr,
		})
		body = bytes.NewReader(payload)
	}
	req, _ := http.NewRequestWithContext(ctx, method, target, body)
	req.Header.Set("Accept", "application/json")
	if method == http.MethodPost {
		req.Header.Set("Content-Type", "application/json")
	}
	if hn, _ := p.src.Config["headerName"].(string); hn != "" {
		hv, _ := p.src.Config["headerValue"].(string)
		req.Header.Set(hn, hv)
	}
	resp, err := p.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 16<<20))
	if resp.StatusCode >= 300 {
		return nil, fmt.Errorf("provider %d: %s", resp.StatusCode, truncate(string(raw), 300))
	}
	items, err := ParseItems(raw)
	if err != nil {
		return nil, err
	}
	return filterWindow(items, Window{From: since, To: until}), nil
}

// filterWindow keeps items published inside w. Comments whose parent post is
// outside the window are kept (the parent is needed for context) when the
// parent is itself part of the batch.
func filterWindow(items []Item, w Window) []Item {
	if w.From.IsZero() && w.To.IsZero() {
		return items
	}
	keepParents := map[string]bool{}
	for _, it := range items {
		if it.Kind == "COMMENT" && it.ParentExternalID != "" && w.Contains(it.PublishedAt) {
			keepParents[it.ParentExternalID] = true
		}
	}
	out := make([]Item, 0, len(items))
	for _, it := range items {
		if w.Contains(it.PublishedAt) || keepParents[it.ExternalID] {
			out = append(out, it)
		}
	}
	return out
}

// ParseItems accepts `[...]` or `{"items":[...]}`.
func ParseItems(raw []byte) ([]Item, error) {
	var arr []Item
	if err := json.Unmarshal(raw, &arr); err == nil {
		return normaliseItems(arr), nil
	}
	var wrapped struct {
		Items []Item `json:"items"`
	}
	if err := json.Unmarshal(raw, &wrapped); err != nil {
		return nil, fmt.Errorf("expected a JSON array of items or {\"items\": [...]}: %w", err)
	}
	return normaliseItems(wrapped.Items), nil
}

func normaliseItems(items []Item) []Item {
	for i := range items {
		it := &items[i]
		it.Kind = strings.ToUpper(strings.TrimSpace(it.Kind))
		if it.Kind == "" {
			it.Kind = "POST"
		}
		it.AuthorType = strings.ToUpper(strings.TrimSpace(it.AuthorType))
		if it.AuthorType == "" {
			it.AuthorType = "PERSON"
		}
		if len(it.Hashtags) == 0 {
			it.Hashtags = extractHashtags(it.Content)
		}
		if len(it.Mentions) == 0 {
			it.Mentions = extractMentions(it.Content)
		}
		if it.PublishedAt.IsZero() {
			it.PublishedAt = time.Now().UTC()
		}
		if it.ExternalID == "" {
			it.ExternalID = fmt.Sprintf("import:%x", hashString(it.AuthorName+"|"+it.Content+"|"+it.PublishedAt.Format(time.RFC3339)))
		}
	}
	return items
}

// ---------------------------------------------------------------------------
// helpers
// ---------------------------------------------------------------------------

func epochMillis(m map[string]interface{}, keys ...string) time.Time {
	for _, k := range keys {
		switch v := m[k].(type) {
		case float64:
			if v > 1e11 {
				return time.UnixMilli(int64(v)).UTC()
			}
			return time.Unix(int64(v), 0).UTC()
		case map[string]interface{}:
			if t, ok := v["time"].(float64); ok {
				return time.UnixMilli(int64(t)).UTC()
			}
		case string:
			if t, err := time.Parse(time.RFC3339, v); err == nil {
				return t
			}
		}
	}
	return time.Now().UTC()
}

func extractHashtags(s string) []string {
	out := []string{}
	for _, f := range strings.Fields(s) {
		if strings.HasPrefix(f, "#") && len(f) > 1 {
			out = append(out, strings.ToLower(strings.Trim(f, "#.,!?:;()")))
		}
	}
	return out
}

func extractMentions(s string) []string {
	out := []string{}
	for _, f := range strings.Fields(s) {
		if strings.HasPrefix(f, "@") && len(f) > 1 {
			out = append(out, strings.ToLower(strings.Trim(f, "@.,!?:;()")))
		}
	}
	return out
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}

func hashString(s string) uint32 {
	var h uint32 = 2166136261
	for i := 0; i < len(s); i++ {
		h ^= uint32(s[i])
		h *= 16777619
	}
	return h
}
