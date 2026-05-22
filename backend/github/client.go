package github

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"time"

	"my-github-pr/backend/models"
	"my-github-pr/logger"
)

type Client struct {
	token  string
	client *http.Client
}

func NewClient(token string) *Client {
	return &Client{
		token:  token,
		client: &http.Client{Timeout: 15 * time.Second},
	}
}

func (c *Client) sendRequest(req *http.Request, val interface{}) error {
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", c.token))
	req.Header.Set("Accept", "application/vnd.github.v3+json")
	req.Header.Set("User-Agent", "Wails-PR-Rebase-Manager")

	resp, err := c.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("github api returned status code %d", resp.StatusCode)
	}

	return json.NewDecoder(resp.Body).Decode(val)
}
func (c *Client) FetchPRs(owner, repo, username string) ([]models.PullRequest, error) {
	logger.Info("Fetching PRs for %s %s, username %s", owner, repo, username)

	// Use GitHub search API so we fetch ONLY user's PRs
	query := fmt.Sprintf(
		"repo:%s/%s is:pr author:%s state:open",
		owner,
		repo,
		username,
	)

	url := fmt.Sprintf(
		"https://api.github.com/search/issues?q=%s&per_page=50",
		url.QueryEscape(query),
	)

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}

	type GHPR struct {
		Number int    `json:"number"`
		Title  string `json:"title"`
		State  string `json:"state"`
		HTMLURL string `json:"html_url"`
		Body   string `json:"body"`
		UpdatedAt string `json:"updated_at"`
		User struct {
			Login string `json:"login"`
		} `json:"user"`
		PullRequest struct {
			URL string `json:"url"`
		} `json:"pull_request"`
	}

	type SearchResponse struct {
		Items []GHPR `json:"items"`
	}

	var result SearchResponse

	if err := c.sendRequest(req, &result); err != nil {
		return nil, err
	}

	var prList []models.PullRequest

	for _, pr := range result.Items {
		updatedTime, _ := time.Parse(time.RFC3339, pr.UpdatedAt)

		mapped := models.PullRequest{
			ID:              fmt.Sprintf("%s-%s-%d", owner, repo, pr.Number),
			Number:          pr.Number,
			Title:           pr.Title,
			RepoID:          fmt.Sprintf("%s-%s", owner, repo),
			RepoName:        fmt.Sprintf("%s/%s", owner, repo),
			State:           pr.State,
			HTMLURL:         pr.HTMLURL,
			UpdatedAt:       updatedTime,
			Description:     pr.Body,
			MergeableStatus: "unknown",
		}

		// hydratePRDetails will still populate:
		// - base branch
		// - head branch
		// - draft
		// - behind/ahead
		// - mergeability
		_ = c.hydratePRDetails(owner, repo, &mapped)

		prList = append(prList, mapped)
	}

	return prList, nil
}

func (c *Client) hydratePRDetails(owner, repo string, pr *models.PullRequest) error {
	// 1. Fetch detailed PR info (gives mergeable status)
	urlDetail := fmt.Sprintf("https://api.github.com/repos/%s/%s/pulls/%d", owner, repo, pr.Number)
	reqDetail, err := http.NewRequest("GET", urlDetail, nil)
	if err != nil {
		return err
	}

	var detail struct {
		Mergeable *bool  `json:"mergeable"`
		MState    string `json:"mergeable_state"`
	}

	if err := c.sendRequest(reqDetail, &detail); err == nil {
		if detail.Mergeable == nil {
			pr.MergeableStatus = "unknown"
		} else if *detail.Mergeable {
			pr.MergeableStatus = "mergeable"
		} else {
			pr.MergeableStatus = "conflicting"
		}
	}

	// 2. Query comparison to find ahead/behind counts
	// Base is baseBranch, Head is headBranch
	urlCompare := fmt.Sprintf("https://api.github.com/repos/%s/%s/compare/%s...%s", owner, repo, pr.BaseBranch, pr.HeadBranch)
	reqCompare, err := http.NewRequest("GET", urlCompare, nil)
	if err != nil {
		return err
	}

	var compare struct {
		AheadBy  int `json:"ahead_by"`
		BehindBy int `json:"behind_by"`
	}

	if err := c.sendRequest(reqCompare, &compare); err == nil {
		pr.AheadCount = compare.AheadBy
		pr.BehindCount = compare.BehindBy
	}

	return nil
}

// FetchCombinedCIStatus retrieves statuses for the head commit of a PR
func (c *Client) FetchCombinedCIStatus(owner, repo, ref string) (string, error) {
	url := fmt.Sprintf("https://api.github.com/repos/%s/%s/commits/%s/check-runs", owner, repo, ref)
	req, err := http.NewRequest("GET", url, nil)
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

	if err := c.sendRequest(req, &result); err != nil {
		return "unknown", err
	}

	if result.TotalCount == 0 {
		return "none", nil
	}

	successCount := 0
	failureCount := 0
	runningCount := 0

	for _, run := range result.CheckRuns {
		if run.Status != "completed" {
			runningCount++
		} else if run.Conclusion != nil {
			if *run.Conclusion == "success" {
				successCount++
			} else if *run.Conclusion == "failure" || *run.Conclusion == "action_required" {
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
