package voc

import "time"

// Company is a LinkedIn organisation the workspace listens to.
type Company struct {
	ID          int64      `json:"companyId"`
	WorkspaceID int64      `json:"workspaceId"`
	Name        string     `json:"companyName"`
	Handle      string     `json:"linkedinHandle"` // vanity name, e.g. "microsoft"
	URN         string     `json:"linkedinUrn"`    // urn:li:organization:1035 (resolved lazily)
	Hashtags    []string   `json:"hashtags"`
	IsActive    bool       `json:"isActive"`
	LastSyncAt  *time.Time `json:"lastSyncAt"`
	CreatedAt   time.Time  `json:"createdAt"`
	PostCount   int        `json:"postCount"`
}

// Source describes one way of reaching LinkedIn for a workspace.
type Source struct {
	ID           int64                  `json:"sourceId"`
	WorkspaceID  int64                  `json:"workspaceId"`
	Channel      string                 `json:"channel"`
	Provider     string                 `json:"provider"` // OFFICIAL_API | GENERIC_JSON | IMPORT | SAMPLE
	Label        string                 `json:"label"`
	AccountEmail string                 `json:"accountEmail"`
	Config       map[string]interface{} `json:"config"`
	Status       string                 `json:"status"` // CONFIGURED | CONNECTED | EXPIRED | ERROR
	IsActive     bool                   `json:"isActive"`
	LastSyncAt   *time.Time             `json:"lastSyncAt"`
	CreatedAt    time.Time              `json:"createdAt"`
}

// Item is the normalised LinkedIn signal every provider must emit. It is the
// contract for the GENERIC_JSON provider and the IMPORT endpoint as well.
type Item struct {
	ExternalID       string                 `json:"externalId"`
	Kind             string                 `json:"kind"` // POST | COMMENT | REPOST
	ParentExternalID string                 `json:"parentExternalId,omitempty"`
	AuthorName       string                 `json:"authorName"`
	AuthorHandle     string                 `json:"authorHandle,omitempty"`
	AuthorHeadline   string                 `json:"authorHeadline,omitempty"`
	AuthorType       string                 `json:"authorType"` // PERSON | ORGANIZATION
	Content          string                 `json:"content"`
	URL              string                 `json:"url,omitempty"`
	Hashtags         []string               `json:"hashtags"`
	Mentions         []string               `json:"mentions"`
	LikeCount        int                    `json:"likeCount"`
	CommentCount     int                    `json:"commentCount"`
	ShareCount       int                    `json:"shareCount"`
	PublishedAt      time.Time              `json:"publishedAt"`
	Raw              map[string]interface{} `json:"raw,omitempty"`
}

// Analysis is what the Python agent returns per item.
type Analysis struct {
	ExternalID      string   `json:"externalId"`
	Sentiment       string   `json:"sentiment"` // POSITIVE | NEGATIVE | NEUTRAL | BROADCAST
	Score           float64  `json:"score"`
	Category        string   `json:"category"`
	Themes          []string `json:"themes"`
	Summary         string   `json:"summary"`
	Urgency         string   `json:"urgency"` // LOW | MEDIUM | HIGH
	SuggestedAction string   `json:"suggestedAction"`
}

// ActionProposal is an action the agent proposes from a set of analysed signals.
type ActionProposal struct {
	Title       string   `json:"title"`
	Description string   `json:"description"`
	Category    string   `json:"category"`
	Priority    string   `json:"priority"`
	DedupeKey   string   `json:"dedupeKey"`
	Evidence    []string `json:"evidence"` // external ids
	SignalCount int      `json:"signalCount"`
}
