package voc

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// PlaywrightProvider extracts signals from LinkedIn's public (logged-out)
// pages through the deterministic scraper hosted next to the agents
// (agents/linkedin_scraper.py, POST /tools/linkedin/scrape).
//
// No credentials, no LinkedIn API. The scraper covers:
//   - the company page feed (recent posts/reposts by the organisation)
//   - third-party posts naming the company or its hashtags, discovered via a
//     web search index and date-filtered by activity id before loading
//   - comments on those posts (visible page of the thread)
//
// Optional source config:
//
//	{ "maxPosts": 25, "includeCompanyPosts": true, "includeDiscovery": true }
type PlaywrightProvider struct {
	src   Source
	cfg   Config
	notes []string
}

func (p *PlaywrightProvider) Name() string   { return "PLAYWRIGHT" }
func (p *PlaywrightProvider) Notes() []string { return p.notes }

// scrapeClient allows long runs: one scrape opens up to a few dozen pages
// with polite delays between them.
var scrapeClient = &http.Client{Timeout: 12 * time.Minute}

func (p *PlaywrightProvider) Fetch(ctx context.Context, c Company, from, to time.Time) ([]Item, error) {
	p.notes = nil
	if strings.TrimSpace(c.Handle) == "" && strings.TrimSpace(c.Name) == "" {
		return nil, fmt.Errorf("company needs a LinkedIn handle or a name to scrape")
	}
	if to.IsZero() {
		to = time.Now().UTC()
	}
	if from.IsZero() {
		from = to.Add(-DefaultWindow)
	}
	maxPosts := 25
	if v, ok := p.src.Config["maxPosts"].(float64); ok && v > 0 {
		maxPosts = int(v)
	}
	includeCompany, includeDiscovery := true, true
	if v, ok := p.src.Config["includeCompanyPosts"].(bool); ok {
		includeCompany = v
	}
	if v, ok := p.src.Config["includeDiscovery"].(bool); ok {
		includeDiscovery = v
	}

	payload, _ := json.Marshal(map[string]interface{}{
		"handle":              c.Handle,
		"company":             c.Name,
		"hashtags":            c.Hashtags,
		"dateFrom":            from.UTC().Format(time.RFC3339),
		"dateTo":              to.UTC().Format(time.RFC3339),
		"maxPosts":            maxPosts,
		"includeCompanyPosts": includeCompany,
		"includeDiscovery":    includeDiscovery,
	})
	url := strings.TrimRight(p.cfg.AgentURL, "/") + "/tools/linkedin/scrape"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := scrapeClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("scraper unreachable at %s: %w", p.cfg.AgentURL, err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 32<<20))
	if resp.StatusCode >= 300 {
		var e struct {
			Detail string `json:"detail"`
		}
		_ = json.Unmarshal(raw, &e)
		if e.Detail == "" {
			e.Detail = truncate(string(raw), 300)
		}
		return nil, fmt.Errorf("scraper → %d: %s", resp.StatusCode, e.Detail)
	}
	var out struct {
		Items []Item                 `json:"items"`
		Notes []string               `json:"notes"`
		Stats map[string]interface{} `json:"stats"`
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, fmt.Errorf("scraper returned malformed JSON: %w", err)
	}
	p.notes = out.Notes
	return normaliseItems(out.Items), nil
}
