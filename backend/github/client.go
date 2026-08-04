package github

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"sort"
	"strings"
	"time"

	"review-dock/backend/git"
	"review-dock/backend/models"
	"review-dock/logger"
)

// Client wraps the gh CLI for all GitHub data interactions.
// Authentication is managed entirely by the gh CLI (gh auth login).
type Client struct{}

func NewClient() *Client {
	return &Client{}
}

// run executes a gh CLI subcommand and returns its stdout.
func (c *Client) run(ctx context.Context, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, "gh", args...)
	out, err := cmd.Output()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			return nil, fmt.Errorf("gh %s: %s", strings.Join(args, " "), strings.TrimSpace(string(exitErr.Stderr)))
		}
		return nil, fmt.Errorf("gh %s: %w", strings.Join(args, " "), err)
	}
	return out, nil
}

// FetchPRs retrieves pull requests (all states) authored by the authenticated user for a repo.
func (c *Client) FetchPRs(ctx context.Context, owner, repo string, localPath string) ([]models.PullRequest, error) {
	logger.Infof("Fetching PRs for %s/%s via gh CLI", owner, repo)

	const fields = "number,title,state,url,updatedAt,createdAt,body,headRefName,baseRefName," +
		"isDraft,mergeable,headRepositoryOwner,headRepository,author,labels,reviewRequests"

	out, err := c.run(ctx, "pr", "list",
		"--repo", fmt.Sprintf("%s/%s", owner, repo),
		"--author", "@me",
		"--state", "all",
		"--limit", "100",
		"--json", fields,
	)
	if err != nil {
		return nil, err
	}

	type ghReviewRequest struct {
		Login string `json:"login"`
	}
	type ghLabel struct {
		Name string `json:"name"`
	}
	type ghAuthor struct {
		Login string `json:"login"`
	}
	type ghPR struct {
		Number              int              `json:"number"`
		Title               string           `json:"title"`
		State               string           `json:"state"`
		URL                 string           `json:"url"`
		UpdatedAt           string           `json:"updatedAt"`
		CreatedAt           string           `json:"createdAt"`
		Body                string           `json:"body"`
		HeadRefName         string           `json:"headRefName"`
		BaseRefName         string           `json:"baseRefName"`
		IsDraft             bool             `json:"isDraft"`
		Mergeable           string           `json:"mergeable"`
		HeadRepositoryOwner struct {
			Login string `json:"login"`
		} `json:"headRepositoryOwner"`
		HeadRepository struct {
			Name string `json:"name"`
		} `json:"headRepository"`
		Author         ghAuthor          `json:"author"`
		Labels         []ghLabel         `json:"labels"`
		ReviewRequests []ghReviewRequest `json:"reviewRequests"`
	}

	var items []ghPR
	if err := json.Unmarshal(out, &items); err != nil {
		return nil, fmt.Errorf("parsing gh pr list output: %w", err)
	}

	repoID := fmt.Sprintf("%s-%s", owner, repo)
	repoName := fmt.Sprintf("%s/%s", owner, repo)

	var result []models.PullRequest
	for _, item := range items {
		updatedAt, _ := time.Parse(time.RFC3339, item.UpdatedAt)
		createdAt, _ := time.Parse(time.RFC3339, item.CreatedAt)

		headLabel := item.HeadRefName
		if upstream, err := git.GetUpstreamByBranch(ctx, localPath, item.HeadRefName); err == nil {
			headLabel = fmt.Sprintf("%s/%s", upstream, item.HeadRefName)
		} else {
			logger.Errorf("Failed to get upstream for branch %s: %v", item.HeadRefName, err)
		}

		baseLabel := item.BaseRefName
		if upstream, err := git.GetUpstreamByBranch(ctx, localPath, baseLabel); err == nil {
			baseLabel = fmt.Sprintf("%s/%s", upstream, baseLabel)
		} else {
			logger.Errorf("Failed to get upstream for branch %s: %v", item.BaseRefName, err)
		}

		// Skip ahead/behind computation for closed/merged PRs since the branch
		// may have been deleted on the remote after merging.
		var localAhead, localBehind, ahead, behind int
		if strings.ToLower(item.State) == "open" || item.IsDraft {
			localAhead, localBehind, _ = git.LocalAheadBehind(ctx, localPath, baseLabel, item.HeadRefName)
			ahead, behind, _ = git.LocalAheadBehind(ctx, localPath, baseLabel, headLabel)
		}

		state := strings.ToLower(item.State)
		// A draft is only meaningful for open PRs — closed/merged drafts should
		// appear under the 'closed' state so they don't pollute the draft view.
		if item.IsDraft && state == "open" {
			state = "draft"
		}

		mergeableStatus := "unknown"
		switch strings.ToUpper(item.Mergeable) {
		case "MERGEABLE":
			mergeableStatus = "mergeable"
		case "CONFLICTING":
			mergeableStatus = "conflicting"
		}

		// Collect labels (name only).
		labels := make([]string, 0, len(item.Labels))
		for _, l := range item.Labels {
			labels = append(labels, l.Name)
		}

		// Collect requested reviewers (login only).
		requestedReviewers := make([]string, 0, len(item.ReviewRequests))
		for _, rr := range item.ReviewRequests {
			requestedReviewers = append(requestedReviewers, rr.Login)
		}

		pr := models.PullRequest{
			ID:                 fmt.Sprintf("%s-%d", repoID, item.Number),
			Number:             item.Number,
			Title:              item.Title,
			RepoID:             repoID,
			RepoName:           repoName,
			BaseBranch:         item.BaseRefName,
			HeadBranch:         item.HeadRefName,
			BaseLabel:          baseLabel,
			HeadLabel:          headLabel,
			State:              state,
			IsDraft:            item.IsDraft,
			CreatedAt:          createdAt,
			UpdatedAt:          updatedAt,
			MergeableStatus:    mergeableStatus,
			AheadCount:         ahead,
			BehindCount:        behind,
			LocalAheadCount:    localAhead,
			LocalBehindCount:   localBehind,
			HTMLURL:            item.URL,
			Description:        item.Body,
			Author:             item.Author.Login,
			Labels:             labels,
			RequestedReviewers: requestedReviewers,
		}

		result = append(result, pr)
	}

	sort.Slice(result, func(i, j int) bool {
		return result[i].AheadCount > result[j].AheadCount
	})

	return result, nil
}


// FetchCombinedCIStatus returns an aggregated CI result for the given head ref.
// Uses `gh api` because no gh command surfaces check-runs by arbitrary ref.
func (c *Client) FetchCombinedCIStatus(ctx context.Context, owner, repo, ref string) (string, error) {
	out, err := c.run(ctx, "api",
		fmt.Sprintf("repos/%s/%s/commits/%s/check-runs", owner, repo, ref),
	)
	if err != nil {
		return "unknown", err
	}

	var result struct {
		TotalCount int `json:"total_count"`
		CheckRuns  []struct {
			Status     string  `json:"status"`
			Conclusion *string `json:"conclusion"`
		} `json:"check_runs"`
	}
	if err := json.Unmarshal(out, &result); err != nil {
		return "unknown", err
	}

	if result.TotalCount == 0 {
		return "none", nil
	}

	failureCount, runningCount := 0, 0
	for _, run := range result.CheckRuns {
		if run.Status != "completed" {
			runningCount++
		} else if run.Conclusion != nil {
			switch *run.Conclusion {
			case "failure", "action_required":
				failureCount++
			}
		}
	}

	if failureCount > 0 {
		return "failure", nil
	}
	if runningCount > 0 {
		return "running", nil
	}
	return "success", nil
}
