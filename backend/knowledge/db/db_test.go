package db

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"
)

func openTest(t *testing.T) *DB {
	t.Helper()
	d, err := Open(filepath.Join(t.TempDir(), "kb.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { d.Close() })
	return d
}

func TestMigrationsAreIdempotent(t *testing.T) {
	path := filepath.Join(t.TempDir(), "kb.db")
	d, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	v1, _ := d.SchemaVersion(context.Background())
	d.Close()
	d, err = Open(path) // re-open: nothing to apply, no error
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	v2, _ := d.SchemaVersion(context.Background())
	if v1 != 1 || v2 != 1 {
		t.Fatalf("schema versions %d/%d, want 1", v1, v2)
	}
	var n int
	d.QueryRow(`SELECT COUNT(*) FROM schema_migrations`).Scan(&n)
	if n != 1 {
		t.Fatalf("migration recorded %d times", n)
	}
	for _, table := range []string{"repositories", "pull_requests", "review_comments", "comment_revisions", "rules", "rule_evidence", "sync_runs", "skill_builds"} {
		if err := d.QueryRow(`SELECT COUNT(*) FROM ` + table).Scan(&n); err != nil {
			t.Errorf("table %s: %v", table, err)
		}
	}
}

func seedPR(t *testing.T, d *DB) (Repository, int64) {
	ctx := context.Background()
	repo, err := d.EnsureRepository(ctx, "odoo", "odoo")
	if err != nil {
		t.Fatal(err)
	}
	err = d.InTx(ctx, func(tx *sql.Tx) error {
		return d.UpsertPR(ctx, tx, PRRecord{ID: 77, RepoID: repo.ID, Number: 7, Title: "t", Author: "me", UpdatedAt: "2026-01-01T00:00:00Z"})
	})
	if err != nil {
		t.Fatal(err)
	}
	return repo, 77
}

func upsert(t *testing.T, d *DB, c CommentRecord) UpsertResult {
	t.Helper()
	var r UpsertResult
	if err := d.InTx(context.Background(), func(tx *sql.Tx) error {
		var err error
		r, err = d.UpsertComment(context.Background(), tx, c)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	return r
}

func TestCommentDedupEditsAndDeletes(t *testing.T) {
	d := openTest(t)
	ctx := context.Background()
	_, pr := seedPR(t, d)
	c := CommentRecord{Source: "inline", GitHubID: 500, PRID: pr, Author: "rev", Body: "Please avoid sudo", UpdatedAt: "2026-01-01T00:00:00Z"}

	if r := upsert(t, d, c); r != Inserted {
		t.Fatalf("first upsert = %v", r)
	}
	if r := upsert(t, d, c); r != Unchanged {
		t.Fatalf("repeat upsert = %v, want Unchanged", r)
	}
	var n int
	d.QueryRow(`SELECT COUNT(*) FROM review_comments`).Scan(&n)
	if n != 1 {
		t.Fatalf("duplicates created: %d rows", n)
	}
	// Same GitHub id from another source is a different comment.
	upsert(t, d, CommentRecord{Source: "issue", GitHubID: 500, PRID: pr, Author: "rev", Body: "x"})
	d.QueryRow(`SELECT COUNT(*) FROM review_comments`).Scan(&n)
	if n != 2 {
		t.Fatalf("source not part of identity: %d rows", n)
	}

	// Edit: original body preserved in revisions.
	c.Body, c.UpdatedAt = "Please avoid sudo here, use check_access", "2026-01-02T00:00:00Z"
	if r := upsert(t, d, c); r != Updated {
		t.Fatalf("edit upsert = %v", r)
	}
	var id int64
	d.QueryRow(`SELECT id FROM review_comments WHERE source='inline' AND github_id=500`).Scan(&id)
	revs, _ := d.CommentRevisions(ctx, id)
	if len(revs) != 1 || revs[0] != "Please avoid sudo" {
		t.Fatalf("revisions = %v", revs)
	}

	// Deletion detection and restore.
	var deleted int
	d.InTx(ctx, func(tx *sql.Tx) error {
		deleted, _ = d.MarkMissingDeleted(ctx, tx, pr, "inline", []int64{})
		return nil
	})
	if deleted != 1 {
		t.Fatalf("deleted = %d", deleted)
	}
	var del sql.NullString
	d.QueryRow(`SELECT deleted_at FROM review_comments WHERE id=?`, id).Scan(&del)
	if !del.Valid {
		t.Fatal("deleted_at not set")
	}
	upsert(t, d, c)
	d.QueryRow(`SELECT deleted_at FROM review_comments WHERE id=?`, id).Scan(&del)
	if del.Valid {
		t.Fatal("reappearing comment not restored")
	}
}

func TestResolutionKeptWhenUnknown(t *testing.T) {
	d := openTest(t)
	_, pr := seedPR(t, d)
	yes := true
	upsert(t, d, CommentRecord{Source: "inline", GitHubID: 1, PRID: pr, Body: "x", IsResolved: &yes})
	upsert(t, d, CommentRecord{Source: "inline", GitHubID: 1, PRID: pr, Body: "x", IsResolved: nil})
	var res sql.NullBool
	d.QueryRow(`SELECT is_resolved FROM review_comments WHERE github_id=1`).Scan(&res)
	if !res.Valid || !res.Bool {
		t.Fatal("known resolution lost when a later fetch could not determine it")
	}
}

func TestCheckpointNeverMovesBackwards(t *testing.T) {
	d := openTest(t)
	ctx := context.Background()
	repo, _ := seedPR(t, d)
	for _, ts := range []string{"2026-02-01T00:00:00Z", "2026-01-01T00:00:00Z"} {
		d.InTx(ctx, func(tx *sql.Tx) error { return d.AdvanceCheckpoint(ctx, tx, repo.ID, ts) })
	}
	r, _ := d.GetRepository(ctx, repo.ID)
	if r.CheckpointUpdatedAt != "2026-02-01T00:00:00Z" {
		t.Fatalf("checkpoint = %s", r.CheckpointUpdatedAt)
	}
}

func TestSettingsDefaultsAndRoundTrip(t *testing.T) {
	d := openTest(t)
	ctx := context.Background()
	s, err := d.LoadSettings(ctx)
	if err != nil || s.SkillName != "odoo-pr-review" || s.IncludePending || s.AutoApproveMinPRs != 0 {
		t.Fatalf("defaults wrong: %+v %v", s, err)
	}
	s.IntervalMinutes = 1 // clamped to 15
	s.PublishTargets = []string{"/tmp/x"}
	if err := d.SaveSettings(ctx, s); err != nil {
		t.Fatal(err)
	}
	s2, _ := d.LoadSettings(ctx)
	if s2.IntervalMinutes != 15 || s2.PublishTargets[0] != "/tmp/x" {
		t.Fatalf("round trip: %+v", s2)
	}
}

func TestRecoverInterrupted(t *testing.T) {
	d := openTest(t)
	ctx := context.Background()
	repo, _ := seedPR(t, d)
	id, _ := d.StartRun(ctx, "cli")
	d.StartRunRepo(ctx, id, repo.ID)
	d.MarkRepoAttempt(ctx, repo.ID)
	n, err := d.RecoverInterrupted(ctx)
	if err != nil || n != 1 {
		t.Fatalf("recovered %d, %v", n, err)
	}
	runs, _ := d.ListRuns(ctx, 1)
	if runs[0].Status != "interrupted" || runs[0].Repos[0].Status != "interrupted" {
		t.Fatalf("run not marked interrupted: %+v", runs[0])
	}
	r, _ := d.GetRepository(ctx, repo.ID)
	if r.SyncStatus != "interrupted" {
		t.Fatalf("repo status %s", r.SyncStatus)
	}
}
