package github

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
	"time"

	"my-github-pr/backend/git"
	"my-github-pr/backend/models"
	"my-github-pr/logger"
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

// FetchPRs retrieves open pull requests authored by the authenticated user for a repo.
func (c *Client) FetchPRs(ctx context.Context, owner, repo string, localPath string) ([]models.PullRequest, error) {
	logger.Infof("Fetching PRs for %s/%s via gh CLI", owner, repo)

	const fields = "number,title,state,url,updatedAt,body,headRefName,baseRefName,isDraft,mergeable,headRepositoryOwner,headRepository"

	out, err := c.run(ctx, "pr", "list",
		"--repo", fmt.Sprintf("%s/%s", owner, repo),
		"--author", "@me",
		"--state", "open",
		"--limit", "50",
		"--json", fields,
	)
	if err != nil {
		return nil, err
	}

	type ghPR struct {
		Number              int    `json:"number"`
		Title               string `json:"title"`
		State               string `json:"state"`
		URL                 string `json:"url"`
		UpdatedAt           string `json:"updatedAt"`
		Body                string `json:"body"`
		HeadRefName         string `json:"headRefName"`
		BaseRefName         string `json:"baseRefName"`
		IsDraft             bool   `json:"isDraft"`
		Mergeable           string `json:"mergeable"`
		HeadRepositoryOwner struct {
			Login string `json:"login"`
		} `json:"headRepositoryOwner"`
		HeadRepository struct {
			Name string `json:"name"`
		} `json:"headRepository"`
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

		ahead, behind, err := git.LocalAheadBehind(ctx, localPath, baseLabel, item.HeadRefName)
		if err != nil {
			logger.Errorf("Failed to get ahead/behind counts for branch %s: %v", item.BaseRefName, err)
		}

		state := strings.ToLower(item.State)
		if item.IsDraft {
			state = "draft"
		}

		mergeableStatus := "unknown"
		switch strings.ToUpper(item.Mergeable) {
		case "MERGEABLE":
			mergeableStatus = "mergeable"
		case "CONFLICTING":
			mergeableStatus = "conflicting"
		}

		pr := models.PullRequest{
			ID:              fmt.Sprintf("%s-%d", repoID, item.Number),
			Number:          item.Number,
			Title:           item.Title,
			RepoID:          repoID,
			RepoName:        repoName,
			BaseBranch:      item.BaseRefName,
			HeadBranch:      item.HeadRefName,
			BaseLabel:       baseLabel,
			HeadLabel:       headLabel,
			State:           state,
			IsDraft:         item.IsDraft,
			UpdatedAt:       updatedAt,
			MergeableStatus: mergeableStatus,
			LocalAheadCount:      ahead,
			LocalBehindCount:     behind,
			HTMLURL:         item.URL,
			Description:     item.Body,
		}

		result = append(result, pr)
	}

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
