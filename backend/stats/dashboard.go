package stats

import "time"

// PRDashboardInput is the minimal PR data the frontend passes when requesting
// dashboard statistics.  Using a lightweight input avoids re-serialising the
// full PullRequest model across the Wails bridge.
type PRDashboardInput struct {
	ID        string `json:"id"`
	RepoID    string `json:"repo_id"`
	Number    int    `json:"number"`
	UpdatedAt string `json:"updated_at"`
	CreatedAt string `json:"created_at"`
	State     string `json:"state"`   // "open", "draft", "closed"
	IsDraft   bool   `json:"is_draft"`
	RepoName  string `json:"repo_name"`
}

// ReviewerSummary summarises a single reviewer's activity across all PRs.
type ReviewerSummary struct {
	Approved         int `json:"approved"`
	ChangesRequested int `json:"changes_requested"`
	Commented        int `json:"commented"`
	TotalComments    int `json:"total_comments"`
}

// DashboardStats aggregates statistics across all currently tracked pull requests.
type DashboardStats struct {
	TotalPRs            int                        `json:"total_prs"`
	OpenPRs             int                        `json:"open_prs"`
	DraftPRs            int                        `json:"draft_prs"`
	ClosedPRs           int                        `json:"closed_prs"`
	TotalComments       int                        `json:"total_comments"`
	TotalCommits        int                        `json:"total_commits"`
	TotalAdditions      int                        `json:"total_additions"`
	TotalDeletions      int                        `json:"total_deletions"`
	DirectlyMergedCount int                        `json:"directly_merged_count"`
	AvgPRAgeDays        float64                    `json:"avg_pr_age_days"`
	ReviewerActivity    map[string]ReviewerSummary `json:"reviewer_activity"`
	PRsByRepo           map[string]int             `json:"prs_by_repo"`
	PRsByMonth          map[string]int             `json:"prs_by_month"` // "2006-01" → count
	StaleRecomputed     int                        `json:"stale_recomputed"`
	CacheHits           int                        `json:"cache_hits"`
	LastUpdated         time.Time                  `json:"last_updated"`
}
