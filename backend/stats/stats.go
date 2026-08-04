package stats

import "time"

// PRStats contains computed metrics for a single pull request.
// It is serialized to JSON and returned to the Wails frontend.
type PRStats struct {
	TotalComments            int               `json:"total_comments"`
	PRConversationComments   int               `json:"pr_conversation_comments"` // issue/timeline comments
	ReviewComments           int               `json:"review_comments"`           // non-empty review bodies
	AuthorReplies            int               `json:"author_replies"`
	ReviewCommentsByReviewer map[string]int    `json:"review_comments_by_reviewer"`
	ReviewerStates           map[string]string `json:"reviewer_states"` // APPROVED / CHANGES_REQUESTED / COMMENTED / DISMISSED
	Commits                  int               `json:"commits"`
	ChangedFiles             int               `json:"changed_files"`
	Additions                int               `json:"additions"`
	Deletions                int               `json:"deletions"`
	DirectlyMerged           bool              `json:"directly_merged"`
	PRAgeDays                float64           `json:"pr_age_days"`
	Labels                   []string          `json:"labels"`
	Author                   string            `json:"author"`
	CreatedAt                string            `json:"created_at"`
	LastUpdated              time.Time         `json:"last_updated"` // when this stats snapshot was computed
	FromCache                bool              `json:"from_cache"`
}

// PRStatsCache is a single entry in the on-disk stats cache, keyed by PR ID.
type PRStatsCache struct {
	// PRUpdatedAt is a snapshot of the PR's updated_at at the time the stats
	// were computed.  When the PR's current updated_at differs, the cache is stale.
	PRUpdatedAt time.Time `json:"pr_updated_at"`
	Stats       PRStats   `json:"stats"`
}
