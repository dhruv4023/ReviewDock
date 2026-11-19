// Package kbtest provides an in-memory stand-in for `gh api` used by the
// knowledge-system tests. It never touches the network.
package kbtest

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"sync"

	"review-dock/backend/knowledge/ghsync"
)

type User struct {
	Login string `json:"login"`
	Type  string `json:"type"`
}

type Inline struct {
	ID        int64  `json:"id"`
	User      User   `json:"user"`
	Body      string `json:"body"`
	Path      string `json:"path"`
	Line      *int   `json:"line"`
	OrigLine  *int   `json:"original_line"`
	Position  *int   `json:"position"`
	DiffHunk  string `json:"diff_hunk"`
	HTMLURL   string `json:"html_url"`
	InReplyTo int64  `json:"in_reply_to_id,omitempty"`
	ReviewID  int64  `json:"pull_request_review_id,omitempty"`
	CommitID  string `json:"commit_id"`
	CreatedAt string `json:"created_at"`
	UpdatedAt string `json:"updated_at"`
}

type Comment struct {
	ID        int64  `json:"id"`
	User      User   `json:"user"`
	Body      string `json:"body"`
	HTMLURL   string `json:"html_url"`
	CreatedAt string `json:"created_at"`
	UpdatedAt string `json:"updated_at"`
}

type Review struct {
	ID          int64  `json:"id"`
	User        User   `json:"user"`
	Body        string `json:"body"`
	State       string `json:"state"`
	HTMLURL     string `json:"html_url"`
	SubmittedAt string `json:"submitted_at"`
}

type Thread struct {
	RootID   int64
	Resolved bool
	Outdated bool
}

type PR struct {
	ID        int64
	Number    int
	Title     string
	Author    string
	State     string
	Merged    bool
	CreatedAt string
	UpdatedAt string
	Inline    []Inline
	Issue     []Comment
	Reviews   []Review
	Threads   []Thread
}

// Fake implements ghsync.Runner.
type Fake struct {
	mu   sync.Mutex
	Repo string // owner/name
	PRs  map[int]*PR

	CoreRemaining int
	// FailTimes[substring] = n: the next n calls whose args contain substring fail with FailMsg.
	FailTimes map[string]int
	FailMsg   string
	// FailAlways[substring] = message: matching calls always fail.
	FailAlways map[string]string

	Calls []string
}

func New(repo string) *Fake {
	return &Fake{Repo: repo, PRs: map[int]*PR{}, CoreRemaining: 5000, FailTimes: map[string]int{}, FailAlways: map[string]string{}}
}

func IntP(v int) *int { return &v }

func (f *Fake) Add(pr *PR) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if pr.State == "" {
		pr.State = "open"
	}
	f.PRs[pr.Number] = pr
}

func (f *Fake) PR(n int) *PR {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.PRs[n]
}

func (f *Fake) CallsMatching(sub string) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	n := 0
	for _, c := range f.Calls {
		if strings.Contains(c, sub) {
			n++
		}
	}
	return n
}

func (f *Fake) API(ctx context.Context, args ...string) ([]byte, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	joined := strings.Join(args, " ")
	f.Calls = append(f.Calls, joined)
	for sub, msg := range f.FailAlways {
		if strings.Contains(joined, sub) {
			return nil, &ghsync.APIError{Msg: msg}
		}
	}
	for sub, n := range f.FailTimes {
		if n > 0 && strings.Contains(joined, sub) {
			f.FailTimes[sub] = n - 1
			msg := f.FailMsg
			if msg == "" {
				msg = "HTTP 502: Bad Gateway"
			}
			return nil, &ghsync.APIError{Msg: msg}
		}
	}
	if args[0] == "graphql" {
		return f.graphql(args)
	}
	path, query := args[0], url.Values{}
	if i := strings.Index(path, "?"); i >= 0 {
		query, _ = url.ParseQuery(path[i+1:])
		path = path[:i]
	}
	if path == "rate_limit" {
		return json.Marshal(map[string]any{"resources": map[string]any{
			"core": map[string]any{"remaining": f.CoreRemaining, "reset": 1900000000}, "search": map[string]any{"remaining": 30, "reset": 1900000000}}})
	}
	if path == "search/issues" {
		return f.search(query)
	}
	prefix := "repos/" + f.Repo + "/"
	if !strings.HasPrefix(path, prefix) {
		return nil, &ghsync.APIError{Msg: "HTTP 404: Not Found (" + path + ")"}
	}
	parts := strings.Split(strings.TrimPrefix(path, prefix), "/")
	if len(parts) < 2 {
		return nil, &ghsync.APIError{Msg: "HTTP 404: Not Found"}
	}
	n, _ := strconv.Atoi(parts[1])
	pr := f.PRs[n]
	if pr == nil {
		return nil, &ghsync.APIError{Msg: "HTTP 404: Not Found"}
	}
	switch {
	case parts[0] == "pulls" && len(parts) == 2:
		return json.Marshal(map[string]any{"id": pr.ID, "node_id": fmt.Sprintf("PR_%d", pr.ID), "number": pr.Number,
			"title": pr.Title, "html_url": fmt.Sprintf("https://github.com/%s/pull/%d", f.Repo, pr.Number),
			"state": pr.State, "user": User{Login: pr.Author, Type: "User"}, "merged": pr.Merged,
			"created_at": pr.CreatedAt, "updated_at": pr.UpdatedAt, "base": map[string]string{"ref": "master"},
			"head": map[string]string{"sha": "abc"}})
	case parts[0] == "pulls" && parts[2] == "comments":
		return page(pr.Inline, query)
	case parts[0] == "issues" && parts[2] == "comments":
		return page(pr.Issue, query)
	case parts[0] == "pulls" && parts[2] == "reviews":
		return page(pr.Reviews, query)
	}
	return nil, &ghsync.APIError{Msg: "HTTP 404: Not Found"}
}

func page[T any](items []T, q url.Values) ([]byte, error) {
	per, _ := strconv.Atoi(q.Get("per_page"))
	p, _ := strconv.Atoi(q.Get("page"))
	if per <= 0 {
		per = 30
	}
	if p <= 0 {
		p = 1
	}
	start := (p - 1) * per
	if start > len(items) {
		start = len(items)
	}
	end := start + per
	if end > len(items) {
		end = len(items)
	}
	out := items[start:end]
	if out == nil {
		out = []T{}
	}
	return json.Marshal(out)
}

func (f *Fake) search(q url.Values) ([]byte, error) {
	query := q.Get("q")
	since := ""
	for _, tok := range strings.Fields(query) {
		if strings.HasPrefix(tok, "updated:>=") {
			since = strings.TrimPrefix(tok, "updated:>=")
		}
	}
	var items []map[string]any
	var prs []*PR
	for _, pr := range f.PRs {
		if since == "" || pr.UpdatedAt >= since {
			prs = append(prs, pr)
		}
	}
	sort.Slice(prs, func(i, j int) bool {
		if prs[i].UpdatedAt != prs[j].UpdatedAt {
			return prs[i].UpdatedAt < prs[j].UpdatedAt
		}
		return prs[i].Number < prs[j].Number
	})
	for _, pr := range prs {
		items = append(items, map[string]any{"number": pr.Number, "updated_at": pr.UpdatedAt})
	}
	b, _ := page(items, q)
	var pageItems []json.RawMessage
	json.Unmarshal(b, &pageItems)
	return json.Marshal(map[string]any{"total_count": len(items), "items": pageItems})
}

func (f *Fake) graphql(args []string) ([]byte, error) {
	var number int
	for i, a := range args {
		if i > 0 && args[i-1] == "-F" && strings.HasPrefix(a, "number=") {
			number, _ = strconv.Atoi(strings.TrimPrefix(a, "number="))
		}
	}
	pr := f.PRs[number]
	var nodes []map[string]any
	if pr != nil {
		for _, t := range pr.Threads {
			nodes = append(nodes, map[string]any{"id": fmt.Sprintf("T_%d", t.RootID), "isResolved": t.Resolved, "isOutdated": t.Outdated,
				"comments": map[string]any{"nodes": []map[string]any{{"databaseId": t.RootID}}}})
		}
	}
	if nodes == nil {
		nodes = []map[string]any{}
	}
	return json.Marshal(map[string]any{"data": map[string]any{"repository": map[string]any{"pullRequest": map[string]any{
		"reviewThreads": map[string]any{"pageInfo": map[string]any{"hasNextPage": false, "endCursor": ""}, "nodes": nodes}}}}})
}
