package ghsync_test

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"review-dock/backend/knowledge/db"
	"review-dock/backend/knowledge/ghsync"
	"review-dock/backend/knowledge/kbtest"
)

func setup(t *testing.T) (*db.DB, *kbtest.Fake, *ghsync.Syncer, db.Repository) {
	t.Helper()
	d, err := db.Open(filepath.Join(t.TempDir(), "kb.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { d.Close() })
	f := kbtest.New("odoo/odoo")
	kbtest.OdooFixture(f)
	c := ghsync.NewClient(f, nil)
	c.Sleep = func(context.Context, time.Duration) error { return nil }
	repo, _ := d.EnsureRepository(context.Background(), "odoo", "odoo")
	return d, f, &ghsync.Syncer{DB: d, GH: c}, repo
}

func reload(t *testing.T, d *db.DB, id int64) db.Repository {
	r, err := d.GetRepository(context.Background(), id)
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func count(t *testing.T, d *db.DB, q string, args ...any) int {
	var n int
	if err := d.QueryRow(q, args...).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func TestFullImportNormalizesAllSources(t *testing.T) {
	d, _, s, repo := setup(t)
	st, err := s.SyncRepo(context.Background(), repo)
	if err != nil {
		t.Fatal(err)
	}
	if st.PRs != 4 || !st.Complete {
		t.Fatalf("stats %+v", st)
	}
	if n := count(t, d, `SELECT COUNT(*) FROM pull_requests`); n != 4 {
		t.Fatalf("prs = %d", n)
	}
	// 10 inline + 6 issue + 2 reviews
	if n := count(t, d, `SELECT COUNT(*) FROM review_comments`); n != 18 {
		t.Fatalf("comments = %d", n)
	}

	var (
		path, hunk, url, cc string
		root, origLine      int64
		resolved, outdated  bool
		state               string
	)
	d.QueryRow(`SELECT path, diff_hunk, html_url, thread_root_id, original_line, is_resolved, is_outdated, code_changed, review_state
		FROM review_comments WHERE source='inline' AND github_id=1001`).Scan(&path, &hunk, &url, &root, &origLine, &resolved, &outdated, &cc, &state)
	if path != "addons/sale/models/sale_order.py" || !strings.Contains(hunk, "order_line") || root != 1001 || origLine != 120 {
		t.Errorf("inline fields: %s %q %d %d", path, hunk, root, origLine)
	}
	if !resolved || !outdated || cc != "yes" || state != "CHANGES_REQUESTED" {
		t.Errorf("thread state: resolved=%v outdated=%v code_changed=%s review_state=%s", resolved, outdated, cc, state)
	}
	// Reply belongs to the root's thread and is flagged as PR author.
	var replyRoot int64
	var isAuthor bool
	d.QueryRow(`SELECT thread_root_id, is_pr_author FROM review_comments WHERE github_id=1002`).Scan(&replyRoot, &isAuthor)
	if replyRoot != 1001 || !isAuthor {
		t.Errorf("reply: root=%d author=%v", replyRoot, isAuthor)
	}
	// Merged PR, unresolved line still present → feedback did not change code.
	d.QueryRow(`SELECT code_changed FROM review_comments WHERE github_id=1202`).Scan(&cc)
	if cc != "no" {
		t.Errorf("code_changed for unchanged line in merged PR = %s", cc)
	}
	if r := reload(t, d, repo.ID); r.CheckpointUpdatedAt != "2026-03-15T10:00:00Z" {
		t.Errorf("checkpoint = %s", r.CheckpointUpdatedAt)
	}
}

func TestRepeatedSyncIsIdempotentAndIncremental(t *testing.T) {
	d, f, s, repo := setup(t)
	ctx := context.Background()
	if _, err := s.SyncRepo(ctx, repo); err != nil {
		t.Fatal(err)
	}
	before := count(t, d, `SELECT COUNT(*) FROM review_comments`)
	prCalls := f.CallsMatching("/pulls/1")

	st, err := s.SyncRepo(ctx, reload(t, d, repo.ID))
	if err != nil {
		t.Fatal(err)
	}
	if st.PRs != 0 || st.Upserted != 0 {
		t.Fatalf("unchanged history re-synced: %+v", st)
	}
	if after := count(t, d, `SELECT COUNT(*) FROM review_comments`); after != before {
		t.Fatalf("duplicates: %d → %d", before, after)
	}
	if f.CallsMatching("/pulls/1") != prCalls {
		t.Fatal("unchanged PRs were fetched again")
	}
	last := f.Calls[len(f.Calls)-1]
	if !strings.Contains(last, "updated%3A%3E%3D2026-03-15T10%3A00%3A00Z") {
		t.Fatalf("incremental query lacks checkpoint qualifier: %s", last)
	}
}

func TestUpdatedAndDeletedCommentsAreTracked(t *testing.T) {
	d, f, s, repo := setup(t)
	ctx := context.Background()
	s.SyncRepo(ctx, repo)

	pr := f.PR(102)
	pr.UpdatedAt = "2026-04-01T00:00:00Z"
	pr.Inline[0].Body = "Please reuse format_amount() from tools instead of duplicating this."
	pr.Inline[0].UpdatedAt = "2026-04-01T00:00:00Z"
	pr.Issue = nil // reviewer deleted their comment

	st, err := s.SyncRepo(ctx, reload(t, d, repo.ID))
	if err != nil {
		t.Fatal(err)
	}
	if st.PRs != 1 || st.Deleted != 1 {
		t.Fatalf("stats %+v", st)
	}
	var body string
	var id int64
	d.QueryRow(`SELECT id, body FROM review_comments WHERE github_id=1101`).Scan(&id, &body)
	if !strings.Contains(body, "format_amount") {
		t.Fatalf("body not updated: %s", body)
	}
	revs, _ := d.CommentRevisions(ctx, id)
	if len(revs) != 1 || !strings.Contains(revs[0], "existing helper") {
		t.Fatalf("original body not preserved: %v", revs)
	}
	if n := count(t, d, `SELECT COUNT(*) FROM review_comments WHERE github_id=2101 AND deleted_at IS NOT NULL`); n != 1 {
		t.Fatal("deleted comment not soft-deleted")
	}
}

func TestTransientErrorsAreRetriedWithBackoff(t *testing.T) {
	_, f, s, repo := setup(t)
	var delays []time.Duration
	s.GH.Sleep = func(_ context.Context, d time.Duration) error { delays = append(delays, d); return nil }
	f.FailTimes["pulls/101/comments"] = 3
	if _, err := s.SyncRepo(context.Background(), repo); err != nil {
		t.Fatal(err)
	}
	if len(delays) != 3 || delays[0] != 2*time.Second || delays[1] != 4*time.Second || delays[2] != 8*time.Second {
		t.Fatalf("backoff delays = %v", delays)
	}
}

func TestPermanentErrorStopsAndResumesFromCheckpoint(t *testing.T) {
	d, f, s, repo := setup(t)
	ctx := context.Background()
	f.FailAlways["pulls/103/reviews"] = "HTTP 404: Not Found"
	_, err := s.SyncRepo(ctx, repo)
	if err == nil || !strings.Contains(err.Error(), "PR #103") {
		t.Fatalf("expected failure on PR 103, got %v", err)
	}
	// 101 and 102 committed; 103 rolled back entirely.
	if n := count(t, d, `SELECT COUNT(*) FROM pull_requests`); n != 2 {
		t.Fatalf("committed PRs = %d", n)
	}
	if n := count(t, d, `SELECT COUNT(*) FROM review_comments c JOIN pull_requests p ON p.id=c.pr_id WHERE p.number=103`); n != 0 {
		t.Fatal("partial PR data committed")
	}
	if r := reload(t, d, repo.ID); r.CheckpointUpdatedAt != "2026-02-03T10:00:00Z" {
		t.Fatalf("checkpoint = %s", r.CheckpointUpdatedAt)
	}

	delete(f.FailAlways, "pulls/103/reviews")
	before := f.CallsMatching("pulls/101")
	st, err := s.SyncRepo(ctx, reload(t, d, repo.ID))
	if err != nil {
		t.Fatal(err)
	}
	if st.PRs != 2 || f.CallsMatching("pulls/101") != before {
		t.Fatalf("resume re-fetched completed work or missed PRs: %+v", st)
	}
	if n := count(t, d, `SELECT COUNT(*) FROM pull_requests`); n != 4 {
		t.Fatalf("PRs after resume = %d", n)
	}
}

func TestRateLimitStopsCleanly(t *testing.T) {
	d, f, s, repo := setup(t)
	f.CoreRemaining = 3
	_, err := s.SyncRepo(context.Background(), repo)
	if !errors.Is(err, ghsync.ErrRateLimited) {
		t.Fatalf("err = %v", err)
	}
	if n := count(t, d, `SELECT COUNT(*) FROM pull_requests`); n != 0 {
		t.Fatalf("synced %d PRs with no quota", n)
	}
}

func TestPerRunBudget(t *testing.T) {
	d, _, s, repo := setup(t)
	s.MaxPRs = 2
	st, _ := s.SyncRepo(context.Background(), repo)
	if st.PRs != 2 || st.Complete {
		t.Fatalf("stats %+v", st)
	}
	s.MaxPRs = 0
	st, _ = s.SyncRepo(context.Background(), reload(t, d, repo.ID))
	if st.PRs != 2 || !st.Complete {
		t.Fatalf("second run %+v", st)
	}
}

func TestGraphQLFailureIsNonFatal(t *testing.T) {
	d, f, s, repo := setup(t)
	f.FailAlways["graphql"] = "HTTP 403: Resource not accessible by integration"
	if _, err := s.SyncRepo(context.Background(), repo); err != nil {
		t.Fatal(err)
	}
	var res *bool
	d.QueryRow(`SELECT is_resolved FROM review_comments WHERE github_id=1001`).Scan(&res)
	if res != nil {
		t.Fatal("resolution invented without GraphQL data")
	}
}

func TestSecretsAreRedacted(t *testing.T) {
	msg := "HTTP 401: Bad credentials token ghp_abcdefghijklmnopqrstuvwxyz0123456789 Authorization: bearer gho_ABCDEFGHIJKLMNOPQRSTUVWXYZ012345"
	got := ghsync.Redact(msg)
	if strings.Contains(got, "ghp_") || strings.Contains(got, "gho_") {
		t.Fatalf("not redacted: %s", got)
	}
	_, f, s, repo := setup(t)
	f.FailAlways["search/issues"] = "HTTP 401: bad token github_pat_11ABCDEFGHIJKLMNOPQRSTUVWXYZ_abcdefgh"
	_, err := s.SyncRepo(context.Background(), repo)
	if err == nil || strings.Contains(err.Error(), "github_pat_") {
		t.Fatalf("token leaked in error: %v", err)
	}
}

func TestCodeChanged(t *testing.T) {
	if ghsync.CodeChanged(true, false) != "yes" || ghsync.CodeChanged(false, true) != "no" || ghsync.CodeChanged(false, false) != "unknown" {
		t.Fatal("CodeChanged heuristic")
	}
}
