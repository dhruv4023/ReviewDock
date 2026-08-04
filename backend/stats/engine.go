package stats

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

// ---------------------------------------------------------------------------
// gh CLI JSON shapes
// ---------------------------------------------------------------------------

type ghActor struct {
	Login string `json:"login"`
}

type ghComment struct {
	Author ghActor `json:"author"`
	Body   string  `json:"body"`
}

type ghReview struct {
	Author   ghActor     `json:"author"`
	State    string      `json:"state"` // APPROVED, CHANGES_REQUESTED, COMMENTED, DISMISSED
	Body     string      `json:"body"`
	Comments []ghComment `json:"comments"` // inline diff/code review comments within this review
}

type ghLabel struct {
	Name string `json:"name"`
}

type ghPRDetail struct {
	Number         int          `json:"number"`
	Author         ghActor      `json:"author"`
	Comments       []ghComment  `json:"comments"`
	Reviews        []ghReview   `json:"reviews"`
	LatestReviews  []ghReview   `json:"latestReviews"`
	Commits        []ghActor    `json:"commits"` // we only need the count, ghActor is a zero-field stand-in
	Additions      int          `json:"additions"`
	Deletions      int          `json:"deletions"`
	ChangedFiles   int          `json:"changedFiles"`
	CreatedAt      string       `json:"createdAt"`
	State          string       `json:"state"`   // OPEN / CLOSED / MERGED
	ReviewDecision string       `json:"reviewDecision"`
	Labels         []ghLabel    `json:"labels"`
}

// ---------------------------------------------------------------------------
// ComputePRStats
// ---------------------------------------------------------------------------

// ComputePRStats fetches rich PR data from GitHub via the gh CLI and derives
// the full set of metrics defined in PRStats.  It does NOT interact with the
// cache — callers are responsible for cache read/write.
func ComputePRStats(ctx context.Context, owner, repo string, prNumber int) (*PRStats, error) {
	const fields = "number,author,comments,reviews,latestReviews,commits," +
		"additions,deletions,changedFiles,createdAt,state,reviewDecision,labels"

	cmd := exec.CommandContext(ctx, "gh", "pr", "view", fmt.Sprintf("%d", prNumber),
		"--repo", fmt.Sprintf("%s/%s", owner, repo),
		"--json", fields,
	)
	out, err := cmd.Output()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			return nil, fmt.Errorf("gh pr view %d: %s", prNumber, strings.TrimSpace(string(exitErr.Stderr)))
		}
		return nil, fmt.Errorf("gh pr view %d: %w", prNumber, err)
	}

	var detail ghPRDetail
	if err := json.Unmarshal(out, &detail); err != nil {
		return nil, fmt.Errorf("parsing gh pr view output: %w", err)
	}

	author := detail.Author.Login

	// ----- Source 1: Conversation / issue comments -----
	// Skip deleted or minimised comments (GitHub returns them with an empty body).
	conversationComments := 0
	authorReplies := 0
	for _, c := range detail.Comments {
		if strings.TrimSpace(c.Body) == "" {
			continue
		}
		conversationComments++
		if strings.EqualFold(c.Author.Login, author) {
			authorReplies++
		}
	}

	// ----- Source 2 & 3: Review bodies + inline diff/code review comments -----
	// GitHub's "Conversation (N)" total includes all three sources above.
	reviewCommentsByReviewer := make(map[string]int)
	reviewerStates := make(map[string]string)
	reviewBodyCount := 0
	inlineCommentCount := 0

	for _, r := range detail.Reviews {
		// Count the top-level review body (e.g. "LGTM" comment on submit).
		if strings.TrimSpace(r.Body) != "" {
			reviewCommentsByReviewer[r.Author.Login]++
			reviewBodyCount++
		}
		// Count inline diff/code review comments nested inside this review.
		for _, c := range r.Comments {
			if strings.TrimSpace(c.Body) == "" {
				continue
			}
			reviewCommentsByReviewer[c.Author.Login]++
			inlineCommentCount++
		}
	}

	// latestReviews gives the most-recent state per reviewer — no duplicates.
	for _, r := range detail.LatestReviews {
		if r.Author.Login != "" {
			reviewerStates[r.Author.Login] = r.State
		}
	}

	reviewComments := reviewBodyCount + inlineCommentCount
	// total matches what GitHub shows in the "Conversation (N)" counter.
	totalComments := conversationComments + reviewComments

	// ----- Labels -----
	labels := make([]string, 0, len(detail.Labels))
	for _, l := range detail.Labels {
		labels = append(labels, l.Name)
	}

	// ----- PR age (days since creation) -----
	var prAgeDays float64
	if detail.CreatedAt != "" {
		if createdAt, parseErr := time.Parse(time.RFC3339, detail.CreatedAt); parseErr == nil {
			prAgeDays = time.Since(createdAt).Hours() / 24
		}
	}

	// ----- Directly merged (merged with no approving review at merge time) -----
	directlyMerged := strings.EqualFold(detail.State, "MERGED") &&
		detail.ReviewDecision != "APPROVED"

	return &PRStats{
		TotalComments:            totalComments,
		PRConversationComments:   conversationComments, // non-empty issue/timeline comments only
		ReviewComments:           reviewComments,       // review bodies + inline diff comments
		AuthorReplies:            authorReplies,
		ReviewCommentsByReviewer: reviewCommentsByReviewer,
		ReviewerStates:           reviewerStates,
		Commits:                  len(detail.Commits),
		ChangedFiles:             detail.ChangedFiles,
		Additions:                detail.Additions,
		Deletions:                detail.Deletions,
		DirectlyMerged:           directlyMerged,
		PRAgeDays:                prAgeDays,
		Labels:                   labels,
		Author:                   author,
		CreatedAt:                detail.CreatedAt,
		LastUpdated:              time.Now(),
		FromCache:                false,
	}, nil
}
