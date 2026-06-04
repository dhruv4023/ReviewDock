package models

import "time"

type Repository struct {
	ID            string    `json:"id"`
	Owner         string    `json:"owner"`
	Name          string    `json:"name"`
	LocalPath     string    `json:"local_path"`
	SyncStatus    string    `json:"sync_status"` // "synced", "error", "pending"
	LastFetchedAt time.Time `json:"last_fetched_at"`
}

type PullRequest struct {
	ID               string    `json:"id"`
	Number           int       `json:"number"`
	Title            string    `json:"title"`
	RepoID           string    `json:"repo_id"`
	RepoName         string    `json:"repo_name"`
	BaseBranch       string    `json:"base_branch"`
	HeadBranch       string    `json:"head_branch"`
	BaseLabel        string    `json:"base_label"`
	HeadLabel        string    `json:"head_label"`
	State            string    `json:"state"` // "open", "closed", "draft"
	IsDraft          bool      `json:"is_draft"`
	UpdatedAt        time.Time `json:"updated_at"`
	AheadCount       int       `json:"ahead_count"`       // local branch commits not on remote
	BehindCount      int       `json:"behind_count"`      // remote tracking commits not in local
	LocalAheadCount  int       `json:"local_ahead_count"`  // local branch commits not on remote
	LocalBehindCount int       `json:"local_behind_count"` // remote tracking commits not in local
	MergeableStatus  string    `json:"mergeable_status"`   // "mergeable", "conflicting", "unknown"
	HTMLURL          string    `json:"html_url"`
	Description      string    `json:"description"`
	CIStatus         string    `json:"ci_status"` // "success", "failure", "running", "none", "unknown"
}

type User struct {
	Login     string `json:"login"`
	ID        int64  `json:"id"`
	AvatarURL string `json:"avatar_url"`
	HTMLURL   string `json:"html_url"`
}

type Session struct {
	AccessToken string `json:"access_token"`
	TokenType   string `json:"token_type"`
	Scope       string `json:"scope"`
	User        *User  `json:"user"`
}

type Settings struct {
	ConcurrencyLimit      int      `json:"concurrency_limit"`
	DefaultRemotePriority []string `json:"default_remote_priority"`
	AmendCommitTimestamp  bool     `json:"amend_commit_timestamp"`
	ForcePushAfterRebase  bool     `json:"force_push_after_rebase"`
	AutoRefreshInterval   int      `json:"auto_refresh_interval_mins"`
	Theme                 string   `json:"theme"`
}

type RebaseRequest struct {
	ID        string `json:"id"` // PR ID or number
	RepoID    string `json:"repo_id"`
	HeadLabel string `json:"head_label"`
	BaseLabel string `json:"base_label"`
}
