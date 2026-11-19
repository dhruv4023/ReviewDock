// Package ghsync collects pull-request review feedback from GitHub through
// the gh CLI (`gh api`), so authentication stays entirely inside gh and the
// application never handles a token. It is the only knowledge package that
// performs network I/O.
package ghsync

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os/exec"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

// Runner executes `gh api <args...>` and returns stdout.
type Runner interface {
	API(ctx context.Context, args ...string) ([]byte, error)
}

// GHRunner runs the real gh binary.
type GHRunner struct{ Binary string }

func (g GHRunner) API(ctx context.Context, args ...string) ([]byte, error) {
	bin := g.Binary
	if bin == "" {
		bin = "gh"
	}
	cmd := exec.CommandContext(ctx, bin, append([]string{"api"}, args...)...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = err.Error()
		}
		return out, &APIError{Msg: Redact(msg)}
	}
	return out, nil
}

// APIError is a failed gh api call with a redacted message.
type APIError struct{ Msg string }

func (e *APIError) Error() string { return e.Msg }

// ErrRateLimited signals that the primary rate limit is exhausted; the sync
// stops cleanly and resumes from its checkpoint on the next run.
var ErrRateLimited = errors.New("github rate limit exhausted")

var reSecret = regexp.MustCompile(`(gh[pousr]_[A-Za-z0-9]{20,}|github_pat_[A-Za-z0-9_]{20,}|(?i:authorization:\s*\S+\s+)\S+|(?i:token\s+)[A-Za-z0-9_\-.]{20,})`)

// Redact masks anything that looks like a GitHub credential.
func Redact(s string) string { return reSecret.ReplaceAllString(s, "[REDACTED]") }

// Retryable reports whether an error is transient.
func Retryable(err error) bool {
	if err == nil {
		return false
	}
	m := strings.ToLower(err.Error())
	for _, s := range []string{"rate limit", "secondary rate", "abuse", "http 429", "http 500", "http 502", "http 503", "http 504",
		"timeout", "timed out", "connection reset", "connection refused", "eof", "tls handshake", "temporary failure",
		"no such host", "i/o timeout", "something went wrong"} {
		if strings.Contains(m, s) {
			return true
		}
	}
	return false
}

// Client adds pagination, retry/backoff and rate-limit awareness to a Runner.
type Client struct {
	R          Runner
	Log        *slog.Logger
	MaxRetries int                                        // default 5
	BaseDelay  time.Duration                              // default 2s; doubles each attempt, capped at 60s
	Sleep      func(context.Context, time.Duration) error // injectable for tests
	MinRemain  int                                        // stop when core remaining drops below this (default 50)
	Calls      int                                        // number of API calls made (for stats/tests)
}

func NewClient(r Runner, log *slog.Logger) *Client {
	return &Client{R: r, Log: log}
}

func (c *Client) logger() *slog.Logger {
	if c.Log == nil {
		return slog.Default()
	}
	return c.Log
}

func (c *Client) sleep(ctx context.Context, d time.Duration) error {
	if c.Sleep != nil {
		return c.Sleep(ctx, d)
	}
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}

// call runs one gh api request with exponential backoff on transient errors.
func (c *Client) call(ctx context.Context, args ...string) ([]byte, error) {
	retries := c.MaxRetries
	if retries <= 0 {
		retries = 5
	}
	delay := c.BaseDelay
	if delay <= 0 {
		delay = 2 * time.Second
	}
	var lastErr error
	for attempt := 0; attempt <= retries; attempt++ {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		c.Calls++
		out, err := c.R.API(ctx, args...)
		if err == nil {
			return out, nil
		}
		lastErr = err
		if !Retryable(err) || attempt == retries {
			break
		}
		c.logger().Warn("github call failed, retrying", "endpoint", endpointOf(args), "attempt", attempt+1, "delay", delay.String(), "error", Redact(err.Error()))
		if err := c.sleep(ctx, delay); err != nil {
			return nil, err
		}
		delay *= 2
		if delay > 60*time.Second {
			delay = 60 * time.Second
		}
	}
	return nil, fmt.Errorf("%s: %s", endpointOf(args), Redact(lastErr.Error()))
}

func endpointOf(args []string) string {
	for _, a := range args {
		if !strings.HasPrefix(a, "-") {
			if i := strings.Index(a, "?"); i > 0 {
				return a[:i]
			}
			return a
		}
	}
	return "gh api"
}

// Get fetches a single REST resource.
func (c *Client) Get(ctx context.Context, path string, v any) error {
	out, err := c.call(ctx, path)
	if err != nil {
		return err
	}
	return json.Unmarshal(out, v)
}

// Paginate fetches every page of a REST list endpoint (per_page=100).
func (c *Client) Paginate(ctx context.Context, path string, each func(json.RawMessage) error) error {
	sep := "?"
	if strings.Contains(path, "?") {
		sep = "&"
	}
	for page := 1; page <= 1000; page++ {
		out, err := c.call(ctx, fmt.Sprintf("%s%sper_page=100&page=%d", path, sep, page))
		if err != nil {
			return err
		}
		var items []json.RawMessage
		if err := json.Unmarshal(out, &items); err != nil {
			return fmt.Errorf("%s: decode page %d: %w", path, page, err)
		}
		for _, it := range items {
			if err := each(it); err != nil {
				return err
			}
		}
		if len(items) < 100 {
			return nil
		}
	}
	return fmt.Errorf("%s: too many pages", path)
}

// GraphQL runs a query; vars of type int are passed with -F, strings with -f.
func (c *Client) GraphQL(ctx context.Context, query string, vars map[string]any, v any) error {
	args := []string{"graphql", "-f", "query=" + query}
	keys := make([]string, 0, len(vars))
	for k := range vars {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		switch val := vars[k].(type) {
		case int:
			args = append(args, "-F", k+"="+strconv.Itoa(val))
		case string:
			args = append(args, "-f", k+"="+val)
		}
	}
	out, err := c.call(ctx, args...)
	if err != nil {
		return err
	}
	var env struct {
		Data   json.RawMessage `json:"data"`
		Errors []struct {
			Message string `json:"message"`
		} `json:"errors"`
	}
	if err := json.Unmarshal(out, &env); err != nil {
		return err
	}
	if len(env.Errors) > 0 {
		return &APIError{Msg: "graphql: " + Redact(env.Errors[0].Message)}
	}
	return json.Unmarshal(env.Data, v)
}

// RateInfo is the remaining REST quota.
type RateInfo struct {
	CoreRemaining   int
	CoreReset       time.Time
	SearchRemaining int
	SearchReset     time.Time
}

// RateLimit queries /rate_limit (this endpoint does not consume quota).
func (c *Client) RateLimit(ctx context.Context) (RateInfo, error) {
	var r struct {
		Resources struct {
			Core struct {
				Remaining int
				Reset     int64
			} `json:"core"`
			Search struct {
				Remaining int
				Reset     int64
			} `json:"search"`
		} `json:"resources"`
	}
	if err := c.Get(ctx, "rate_limit", &r); err != nil {
		return RateInfo{}, err
	}
	return RateInfo{
		CoreRemaining: r.Resources.Core.Remaining, CoreReset: time.Unix(r.Resources.Core.Reset, 0).UTC(),
		SearchRemaining: r.Resources.Search.Remaining, SearchReset: time.Unix(r.Resources.Search.Reset, 0).UTC(),
	}, nil
}

// EnsureQuota waits for the search bucket (short resets) and returns
// ErrRateLimited when the core bucket is too low to continue.
func (c *Client) EnsureQuota(ctx context.Context, needSearch bool) error {
	ri, err := c.RateLimit(ctx)
	if err != nil {
		// Unknown quota: proceed; per-call backoff still protects us.
		c.logger().Warn("rate limit check failed", "error", Redact(err.Error()))
		return nil
	}
	min := c.MinRemain
	if min <= 0 {
		min = 50
	}
	if ri.CoreRemaining < min {
		return fmt.Errorf("%w: %d core requests left, resets at %s", ErrRateLimited, ri.CoreRemaining, ri.CoreReset.Format(time.RFC3339))
	}
	if needSearch && ri.SearchRemaining < 2 {
		wait := time.Until(ri.SearchReset) + time.Second
		if wait > 0 && wait <= 90*time.Second {
			c.logger().Info("waiting for search rate limit reset", "wait", wait.String())
			return c.sleep(ctx, wait)
		}
	}
	return nil
}
