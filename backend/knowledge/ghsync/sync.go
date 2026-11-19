package ghsync

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/url"
	"strings"

	"review-dock/backend/knowledge/db"
)

// RepoStats summarises one repository sync.
type RepoStats struct {
	PRs      int
	Upserted int // inserted or updated comments
	Deleted  int
	Complete bool // reached the end of the result set (no more PRs to fetch)
}

// Syncer fetches review feedback for repositories into the knowledge DB.
type Syncer struct {
	DB  *db.DB
	GH  *Client
	Log *slog.Logger
	// MaxPRs bounds the PRs processed per repository per run (0 = no bound).
	// Large historical imports then complete over several runs.
	MaxPRs int
}

type ghUser struct {
	Login string `json:"login"`
	Type  string `json:"type"`
}

type searchItem struct {
	Number    int    `json:"number"`
	UpdatedAt string `json:"updated_at"`
}

type ghPull struct {
	ID        int64  `json:"id"`
	NodeID    string `json:"node_id"`
	Number    int    `json:"number"`
	Title     string `json:"title"`
	HTMLURL   string `json:"html_url"`
	State     string `json:"state"`
	User      ghUser `json:"user"`
	Merged    bool   `json:"merged"`
	MergedAt  string `json:"merged_at"`
	ClosedAt  string `json:"closed_at"`
	CreatedAt string `json:"created_at"`
	UpdatedAt string `json:"updated_at"`
	Base      struct {
		Ref string `json:"ref"`
	} `json:"base"`
	Head struct {
		SHA string `json:"sha"`
	} `json:"head"`
}

type ghInline struct {
	ID                  int64  `json:"id"`
	NodeID              string `json:"node_id"`
	User                ghUser `json:"user"`
	AuthorAssociation   string `json:"author_association"`
	Body                string `json:"body"`
	Path                string `json:"path"`
	Line                *int   `json:"line"`
	OriginalLine        *int   `json:"original_line"`
	Position            *int   `json:"position"`
	Side                string `json:"side"`
	CommitID            string `json:"commit_id"`
	OriginalCommitID    string `json:"original_commit_id"`
	DiffHunk            string `json:"diff_hunk"`
	HTMLURL             string `json:"html_url"`
	InReplyToID         int64  `json:"in_reply_to_id"`
	PullRequestReviewID int64  `json:"pull_request_review_id"`
	CreatedAt           string `json:"created_at"`
	UpdatedAt           string `json:"updated_at"`
}

type ghIssueComment struct {
	ID                int64  `json:"id"`
	NodeID            string `json:"node_id"`
	User              ghUser `json:"user"`
	AuthorAssociation string `json:"author_association"`
	Body              string `json:"body"`
	HTMLURL           string `json:"html_url"`
	CreatedAt         string `json:"created_at"`
	UpdatedAt         string `json:"updated_at"`
}

type ghReview struct {
	ID                int64  `json:"id"`
	NodeID            string `json:"node_id"`
	User              ghUser `json:"user"`
	AuthorAssociation string `json:"author_association"`
	Body              string `json:"body"`
	State             string `json:"state"`
	HTMLURL           string `json:"html_url"`
	CommitID          string `json:"commit_id"`
	SubmittedAt       string `json:"submitted_at"`
}

type threadInfo struct {
	NodeID   string
	Resolved bool
	Outdated bool
}

const threadsQuery = `query($owner:String!,$name:String!,$number:Int!,$cursor:String){repository(owner:$owner,name:$name){pullRequest(number:$number){reviewThreads(first:100,after:$cursor){pageInfo{hasNextPage endCursor} nodes{id isResolved isOutdated comments(first:100){nodes{databaseId}}}}}}}`

func (s *Syncer) logger() *slog.Logger {
	if s.Log == nil {
		return slog.Default()
	}
	return s.Log
}

// SyncRepo performs an incremental (or, without checkpoint, full historical)
// sync of one repository. Progress is committed per PR, so an interrupted run
// resumes from the last committed PR.
func (s *Syncer) SyncRepo(ctx context.Context, repo db.Repository) (RepoStats, error) {
	var st RepoStats
	log := s.logger().With("repo", repo.FullName)
	checkpoint := repo.CheckpointUpdatedAt
	author := strings.TrimSpace(repo.AuthorFilter)
	if author == "" {
		author = "@me"
	}

	for round := 0; ; round++ {
		q := fmt.Sprintf("repo:%s is:pr author:%s", repo.FullName, author)
		if checkpoint != "" {
			q += " updated:>=" + checkpoint
		}
		log.Info("searching pull requests", "query", q, "round", round)
		startCheckpoint := checkpoint
		fullWindow := false

		for page := 1; page <= 10; page++ { // search API caps at 1000 results
			if err := s.GH.EnsureQuota(ctx, true); err != nil {
				return st, err
			}
			var res struct {
				TotalCount int          `json:"total_count"`
				Items      []searchItem `json:"items"`
			}
			path := fmt.Sprintf("search/issues?q=%s&sort=updated&order=asc&per_page=100&page=%d", url.QueryEscape(q), page)
			if err := s.GH.Get(ctx, path, &res); err != nil {
				return st, err
			}
			for _, it := range res.Items {
				if err := ctx.Err(); err != nil {
					return st, err
				}
				if s.MaxPRs > 0 && st.PRs >= s.MaxPRs {
					log.Info("per-run PR budget reached; will continue next run", "prs", st.PRs)
					return st, nil
				}
				if skip, err := s.alreadySynced(ctx, repo.ID, it); err != nil {
					return st, err
				} else if skip {
					continue
				}
				if err := s.GH.EnsureQuota(ctx, false); err != nil {
					return st, err
				}
				up, del, err := s.syncPR(ctx, repo, it)
				if err != nil {
					return st, fmt.Errorf("PR #%d: %w", it.Number, err)
				}
				st.PRs++
				st.Upserted += up
				st.Deleted += del
				if it.UpdatedAt > checkpoint {
					checkpoint = it.UpdatedAt
				}
			}
			if len(res.Items) < 100 {
				st.Complete = true
				return st, nil
			}
			fullWindow = page == 10
		}
		// 1000-result window exhausted: re-query from the advanced checkpoint.
		if !fullWindow || checkpoint == startCheckpoint {
			log.Warn("search window did not advance; stopping to avoid a loop", "checkpoint", checkpoint)
			return st, nil
		}
	}
}

// alreadySynced skips PRs at the inclusive checkpoint boundary that are
// stored with the same updated_at.
func (s *Syncer) alreadySynced(ctx context.Context, repoID int64, it searchItem) (bool, error) {
	var upd string
	err := s.DB.QueryRowContext(ctx, `SELECT COALESCE(updated_at,'') FROM pull_requests WHERE repo_id=? AND number=?`, repoID, it.Number).Scan(&upd)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return upd != "" && upd == it.UpdatedAt, nil
}

func (s *Syncer) syncPR(ctx context.Context, repo db.Repository, it searchItem) (int, int, error) {
	base := fmt.Sprintf("repos/%s/%s", repo.Owner, repo.Name)
	var pr ghPull
	if err := s.GH.Get(ctx, fmt.Sprintf("%s/pulls/%d", base, it.Number), &pr); err != nil {
		return 0, 0, err
	}

	var inline []ghInline
	var inlineRaw []json.RawMessage
	if err := s.GH.Paginate(ctx, fmt.Sprintf("%s/pulls/%d/comments", base, it.Number), func(m json.RawMessage) error {
		var c ghInline
		if err := json.Unmarshal(m, &c); err != nil {
			return err
		}
		inline, inlineRaw = append(inline, c), append(inlineRaw, m)
		return nil
	}); err != nil {
		return 0, 0, err
	}
	var issue []ghIssueComment
	var issueRaw []json.RawMessage
	if err := s.GH.Paginate(ctx, fmt.Sprintf("%s/issues/%d/comments", base, it.Number), func(m json.RawMessage) error {
		var c ghIssueComment
		if err := json.Unmarshal(m, &c); err != nil {
			return err
		}
		issue, issueRaw = append(issue, c), append(issueRaw, m)
		return nil
	}); err != nil {
		return 0, 0, err
	}
	var reviews []ghReview
	var reviewRaw []json.RawMessage
	if err := s.GH.Paginate(ctx, fmt.Sprintf("%s/pulls/%d/reviews", base, it.Number), func(m json.RawMessage) error {
		var r ghReview
		if err := json.Unmarshal(m, &r); err != nil {
			return err
		}
		reviews, reviewRaw = append(reviews, r), append(reviewRaw, m)
		return nil
	}); err != nil {
		return 0, 0, err
	}

	threads, terr := s.fetchThreads(ctx, repo, it.Number)
	if terr != nil {
		// Resolution status is best-effort: keep previous values.
		s.logger().Warn("review thread status unavailable", "repo", repo.FullName, "pr", it.Number, "error", Redact(terr.Error()))
	}

	prAuthor := strings.ToLower(pr.User.Login)
	reviewState := map[int64]string{}
	for _, r := range reviews {
		reviewState[r.ID] = r.State
	}

	upserted, deleted := 0, 0
	err := s.DB.InTx(ctx, func(tx *sql.Tx) error {
		if err := s.DB.UpsertPR(ctx, tx, db.PRRecord{
			ID: pr.ID, RepoID: repo.ID, NodeID: pr.NodeID, Number: pr.Number, Title: pr.Title, URL: pr.HTMLURL,
			Author: pr.User.Login, State: pr.State, Merged: pr.Merged || pr.MergedAt != "", BaseRef: pr.Base.Ref,
			HeadSHA: pr.Head.SHA, CreatedAt: pr.CreatedAt, UpdatedAt: firstNonEmpty(it.UpdatedAt, pr.UpdatedAt),
			ClosedAt: pr.ClosedAt, MergedAt: pr.MergedAt,
		}); err != nil {
			return err
		}
		merged := pr.Merged || pr.MergedAt != ""

		count := func(r db.UpsertResult) {
			if r != db.Unchanged {
				upserted++
			}
		}

		var seen []int64
		for i, c := range inline {
			root := c.ID
			if c.InReplyToID != 0 {
				root = c.InReplyToID
			}
			rec := db.CommentRecord{
				Source: "inline", GitHubID: c.ID, NodeID: c.NodeID, PRID: pr.ID, Author: c.User.Login,
				AuthorAssociation: c.AuthorAssociation, IsBot: c.User.Type == "Bot",
				IsPRAuthor: strings.ToLower(c.User.Login) == prAuthor, Body: c.Body,
				ReviewState: reviewState[c.PullRequestReviewID], ReviewID: c.PullRequestReviewID,
				InReplyToID: c.InReplyToID, ThreadRootID: root, Path: c.Path, Line: deref(c.Line),
				OriginalLine: deref(c.OriginalLine), Side: c.Side, CommitSHA: c.CommitID, OriginalCommitSHA: c.OriginalCommitID,
				DiffHunk: c.DiffHunk, HTMLURL: c.HTMLURL, CreatedAt: c.CreatedAt, UpdatedAt: c.UpdatedAt, RawJSON: string(inlineRaw[i]),
			}
			outdated := c.Line == nil && c.Position == nil
			if t, ok := threads[root]; ok {
				res, out := t.Resolved, t.Outdated
				rec.IsResolved, rec.IsOutdated, rec.ThreadNodeID = &res, &out, t.NodeID
				outdated = outdated || t.Outdated
			} else if terr == nil {
				out := outdated
				rec.IsOutdated = &out
			}
			rec.CodeChanged = CodeChanged(outdated, merged)
			r, err := s.DB.UpsertComment(ctx, tx, rec)
			if err != nil {
				return err
			}
			count(r)
			seen = append(seen, c.ID)
		}
		n, err := s.DB.MarkMissingDeleted(ctx, tx, pr.ID, "inline", seen)
		if err != nil {
			return err
		}
		deleted += n

		seen = nil
		for i, c := range issue {
			r, err := s.DB.UpsertComment(ctx, tx, db.CommentRecord{
				Source: "issue", GitHubID: c.ID, NodeID: c.NodeID, PRID: pr.ID, Author: c.User.Login,
				AuthorAssociation: c.AuthorAssociation, IsBot: c.User.Type == "Bot",
				IsPRAuthor: strings.ToLower(c.User.Login) == prAuthor, Body: c.Body, HTMLURL: c.HTMLURL,
				CreatedAt: c.CreatedAt, UpdatedAt: c.UpdatedAt, RawJSON: string(issueRaw[i]),
			})
			if err != nil {
				return err
			}
			count(r)
			seen = append(seen, c.ID)
		}
		if n, err = s.DB.MarkMissingDeleted(ctx, tx, pr.ID, "issue", seen); err != nil {
			return err
		}
		deleted += n

		seen = nil
		for i, r := range reviews {
			res, err := s.DB.UpsertComment(ctx, tx, db.CommentRecord{
				Source: "review", GitHubID: r.ID, NodeID: r.NodeID, PRID: pr.ID, Author: r.User.Login,
				AuthorAssociation: r.AuthorAssociation, IsBot: r.User.Type == "Bot",
				IsPRAuthor: strings.ToLower(r.User.Login) == prAuthor, Body: r.Body, ReviewState: r.State,
				HTMLURL: r.HTMLURL, CommitSHA: r.CommitID, CreatedAt: r.SubmittedAt, UpdatedAt: r.SubmittedAt,
				RawJSON: string(reviewRaw[i]),
			})
			if err != nil {
				return err
			}
			count(res)
			seen = append(seen, r.ID)
		}
		if n, err = s.DB.MarkMissingDeleted(ctx, tx, pr.ID, "review", seen); err != nil {
			return err
		}
		deleted += n

		return s.DB.AdvanceCheckpoint(ctx, tx, repo.ID, it.UpdatedAt)
	})
	if err == nil {
		s.logger().Info("pull request synced", "repo", repo.FullName, "pr", it.Number,
			"inline", len(inline), "issue", len(issue), "reviews", len(reviews), "changed", upserted, "deleted", deleted)
	}
	return upserted, deleted, err
}

// CodeChanged derives whether review feedback led to a code change:
// an outdated thread means the commented lines were modified afterwards.
func CodeChanged(outdated, merged bool) string {
	switch {
	case outdated:
		return "yes"
	case merged:
		return "no"
	}
	return "unknown"
}

func (s *Syncer) fetchThreads(ctx context.Context, repo db.Repository, number int) (map[int64]threadInfo, error) {
	out := map[int64]threadInfo{}
	cursor := ""
	for i := 0; i < 50; i++ {
		vars := map[string]any{"owner": repo.Owner, "name": repo.Name, "number": number}
		if cursor != "" {
			vars["cursor"] = cursor
		}
		var data struct {
			Repository struct {
				PullRequest struct {
					ReviewThreads struct {
						PageInfo struct {
							HasNextPage bool   `json:"hasNextPage"`
							EndCursor   string `json:"endCursor"`
						} `json:"pageInfo"`
						Nodes []struct {
							ID         string `json:"id"`
							IsResolved bool   `json:"isResolved"`
							IsOutdated bool   `json:"isOutdated"`
							Comments   struct {
								Nodes []struct {
									DatabaseID int64 `json:"databaseId"`
								} `json:"nodes"`
							} `json:"comments"`
						} `json:"nodes"`
					} `json:"reviewThreads"`
				} `json:"pullRequest"`
			} `json:"repository"`
		}
		if err := s.GH.GraphQL(ctx, threadsQuery, vars, &data); err != nil {
			return nil, err
		}
		rt := data.Repository.PullRequest.ReviewThreads
		for _, n := range rt.Nodes {
			if len(n.Comments.Nodes) == 0 {
				continue
			}
			// The first comment of a thread is its root.
			out[n.Comments.Nodes[0].DatabaseID] = threadInfo{NodeID: n.ID, Resolved: n.IsResolved, Outdated: n.IsOutdated}
		}
		if !rt.PageInfo.HasNextPage {
			return out, nil
		}
		cursor = rt.PageInfo.EndCursor
	}
	return out, nil
}

func deref(p *int) int {
	if p == nil {
		return 0
	}
	return *p
}

func firstNonEmpty(a, b string) string {
	if a != "" {
		return a
	}
	return b
}
