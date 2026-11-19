package db

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

// ---------------------------------------------------------------------------
// Repositories
// ---------------------------------------------------------------------------

type Repository struct {
	ID                  int64  `json:"id"`
	FullName            string `json:"full_name"`
	Owner               string `json:"owner"`
	Name                string `json:"name"`
	Enabled             bool   `json:"enabled"`
	AuthorFilter        string `json:"author_filter"`
	SyncStatus          string `json:"sync_status"`
	CheckpointUpdatedAt string `json:"checkpoint_updated_at"`
	FullImportDone      bool   `json:"full_import_done"`
	LastAttemptAt       string `json:"last_attempt_at"`
	LastSuccessAt       string `json:"last_success_at"`
	LastError           string `json:"last_error"`
	ConsecutiveFailures int    `json:"consecutive_failures"`
	CreatedAt           string `json:"created_at"`
	PRCount             int    `json:"pr_count"`
	CommentCount        int    `json:"comment_count"`
}

// EnsureRepository registers owner/name if missing and returns it.
func (d *DB) EnsureRepository(ctx context.Context, owner, name string) (Repository, error) {
	owner, name = strings.TrimSpace(owner), strings.TrimSpace(name)
	if owner == "" || name == "" {
		return Repository{}, errors.New("repository owner and name are required")
	}
	full := owner + "/" + name
	_, err := d.ExecContext(ctx, `INSERT INTO repositories(full_name, owner, name, created_at)
		VALUES (?, ?, ?, ?) ON CONFLICT(full_name) DO NOTHING`, full, owner, name, d.NowTS())
	if err != nil {
		return Repository{}, err
	}
	return d.GetRepositoryByName(ctx, full)
}

const repoCols = `r.id, r.full_name, r.owner, r.name, r.enabled, r.author_filter, r.sync_status,
	COALESCE(r.checkpoint_updated_at,''), r.full_import_done, COALESCE(r.last_attempt_at,''),
	COALESCE(r.last_success_at,''), COALESCE(r.last_error,''), r.consecutive_failures, r.created_at,
	(SELECT COUNT(*) FROM pull_requests p WHERE p.repo_id = r.id),
	(SELECT COUNT(*) FROM review_comments c JOIN pull_requests p ON p.id = c.pr_id WHERE p.repo_id = r.id AND c.deleted_at IS NULL)`

func scanRepo(sc interface{ Scan(...any) error }) (Repository, error) {
	var r Repository
	err := sc.Scan(&r.ID, &r.FullName, &r.Owner, &r.Name, &r.Enabled, &r.AuthorFilter, &r.SyncStatus,
		&r.CheckpointUpdatedAt, &r.FullImportDone, &r.LastAttemptAt, &r.LastSuccessAt, &r.LastError,
		&r.ConsecutiveFailures, &r.CreatedAt, &r.PRCount, &r.CommentCount)
	return r, err
}

func (d *DB) GetRepositoryByName(ctx context.Context, full string) (Repository, error) {
	return scanRepo(d.QueryRowContext(ctx, `SELECT `+repoCols+` FROM repositories r WHERE r.full_name = ?`, full))
}

func (d *DB) GetRepository(ctx context.Context, id int64) (Repository, error) {
	return scanRepo(d.QueryRowContext(ctx, `SELECT `+repoCols+` FROM repositories r WHERE r.id = ?`, id))
}

func (d *DB) ListRepositories(ctx context.Context) ([]Repository, error) {
	rows, err := d.QueryContext(ctx, `SELECT `+repoCols+` FROM repositories r ORDER BY r.full_name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Repository
	for rows.Next() {
		r, err := scanRepo(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// UpdateRepositoryConfig changes the user-editable fields.
func (d *DB) UpdateRepositoryConfig(ctx context.Context, id int64, enabled bool, authorFilter string) error {
	authorFilter = strings.TrimSpace(authorFilter)
	if authorFilter == "" {
		authorFilter = "@me"
	}
	_, err := d.ExecContext(ctx, `UPDATE repositories SET enabled = ?, author_filter = ? WHERE id = ?`, enabled, authorFilter, id)
	return err
}

// ResetRepositoryCheckpoint forces the next sync to re-import full history.
// Existing comments are kept (upserts are idempotent).
func (d *DB) ResetRepositoryCheckpoint(ctx context.Context, id int64) error {
	_, err := d.ExecContext(ctx, `UPDATE repositories SET checkpoint_updated_at = NULL, full_import_done = 0 WHERE id = ?`, id)
	return err
}

func (d *DB) DeleteRepository(ctx context.Context, id int64) error {
	_, err := d.ExecContext(ctx, `DELETE FROM repositories WHERE id = ?`, id)
	return err
}

func (d *DB) MarkRepoAttempt(ctx context.Context, id int64) error {
	_, err := d.ExecContext(ctx, `UPDATE repositories SET sync_status = 'running', last_attempt_at = ? WHERE id = ?`, d.NowTS(), id)
	return err
}

// AdvanceCheckpoint moves the high-water mark forward (never backwards).
func (d *DB) AdvanceCheckpoint(ctx context.Context, tx *sql.Tx, id int64, updatedAt string) error {
	_, err := tx.ExecContext(ctx, `UPDATE repositories SET checkpoint_updated_at = ?
		WHERE id = ? AND (checkpoint_updated_at IS NULL OR checkpoint_updated_at < ?)`, updatedAt, id, updatedAt)
	return err
}

// FinishRepoSync records the outcome of a repository sync.
func (d *DB) FinishRepoSync(ctx context.Context, id int64, status, errMsg string, completed bool) error {
	if status == "ok" {
		_, err := d.ExecContext(ctx, `UPDATE repositories SET sync_status = 'ok', last_success_at = ?, last_error = NULL,
			consecutive_failures = 0, full_import_done = CASE WHEN ? THEN 1 ELSE full_import_done END WHERE id = ?`,
			d.NowTS(), completed, id)
		return err
	}
	_, err := d.ExecContext(ctx, `UPDATE repositories SET sync_status = ?, last_error = ?,
		consecutive_failures = consecutive_failures + 1 WHERE id = ?`, status, errMsg, id)
	return err
}

// ---------------------------------------------------------------------------
// Pull requests & comments
// ---------------------------------------------------------------------------

type PRRecord struct {
	ID        int64
	RepoID    int64
	NodeID    string
	Number    int
	Title     string
	URL       string
	Author    string
	State     string
	Merged    bool
	BaseRef   string
	HeadSHA   string
	CreatedAt string
	UpdatedAt string
	ClosedAt  string
	MergedAt  string
}

func (d *DB) UpsertPR(ctx context.Context, tx *sql.Tx, p PRRecord) error {
	_, err := tx.ExecContext(ctx, `INSERT INTO pull_requests
		(id, repo_id, node_id, number, title, url, author, state, merged, base_ref, head_sha, created_at, updated_at, closed_at, merged_at, synced_at)
		VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)
		ON CONFLICT(id) DO UPDATE SET repo_id=excluded.repo_id, node_id=excluded.node_id, number=excluded.number,
		  title=excluded.title, url=excluded.url, author=excluded.author, state=excluded.state, merged=excluded.merged,
		  base_ref=excluded.base_ref, head_sha=excluded.head_sha, created_at=excluded.created_at,
		  updated_at=excluded.updated_at, closed_at=excluded.closed_at, merged_at=excluded.merged_at, synced_at=excluded.synced_at`,
		p.ID, p.RepoID, NullStr(p.NodeID), p.Number, p.Title, p.URL, p.Author, p.State, p.Merged, NullStr(p.BaseRef),
		NullStr(p.HeadSHA), NullStr(p.CreatedAt), NullStr(p.UpdatedAt), NullStr(p.ClosedAt), NullStr(p.MergedAt), d.NowTS())
	return err
}

type CommentRecord struct {
	Source            string // inline|issue|review
	GitHubID          int64
	NodeID            string
	PRID              int64
	Author            string
	AuthorAssociation string
	IsBot             bool
	IsPRAuthor        bool
	Body              string
	ReviewState       string
	ReviewID          int64
	InReplyToID       int64
	ThreadRootID      int64
	ThreadNodeID      string
	Path              string
	Line              int
	OriginalLine      int
	Side              string
	CommitSHA         string
	OriginalCommitSHA string
	DiffHunk          string
	HTMLURL           string
	IsResolved        *bool
	IsOutdated        *bool
	CodeChanged       string
	CreatedAt         string
	UpdatedAt         string
	RawJSON           string
}

// BodyHash is the stable content hash used to detect edits.
func BodyHash(body string) string {
	h := sha256.Sum256([]byte(body))
	return hex.EncodeToString(h[:])
}

func nullInt(v int64) any {
	if v == 0 {
		return nil
	}
	return v
}

func nullBool(b *bool) any {
	if b == nil {
		return nil
	}
	return *b
}

// UpsertResult says what happened to a comment.
type UpsertResult int

const (
	Unchanged UpsertResult = iota
	Inserted
	Updated
)

// UpsertComment inserts or updates a comment keyed by (source, github_id).
// When the body changed, the previous body is preserved in comment_revisions.
// A previously soft-deleted comment that reappears is restored.
func (d *DB) UpsertComment(ctx context.Context, tx *sql.Tx, c CommentRecord) (UpsertResult, error) {
	if c.CodeChanged == "" {
		c.CodeChanged = "unknown"
	}
	hash := BodyHash(c.Body)
	now := d.NowTS()

	var (
		id                       int64
		oldHash, oldBody, oldUpd string
		deleted                  sql.NullString
		oldResolved, oldOutdated sql.NullBool
	)
	err := tx.QueryRowContext(ctx, `SELECT id, body_hash, body, COALESCE(updated_at,''), deleted_at, is_resolved, is_outdated
		FROM review_comments WHERE source = ? AND github_id = ?`, c.Source, c.GitHubID).
		Scan(&id, &oldHash, &oldBody, &oldUpd, &deleted, &oldResolved, &oldOutdated)

	if errors.Is(err, sql.ErrNoRows) {
		_, err = tx.ExecContext(ctx, `INSERT INTO review_comments
			(source, github_id, node_id, pr_id, author, author_association, is_bot, is_pr_author, body, review_state, review_id,
			 in_reply_to_id, thread_root_id, thread_node_id, path, line, original_line, side, commit_sha, original_commit_sha,
			 diff_hunk, html_url, is_resolved, is_outdated, code_changed, created_at, updated_at, body_hash, raw_json,
			 first_seen_at, last_seen_at)
			VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
			c.Source, c.GitHubID, NullStr(c.NodeID), c.PRID, c.Author, NullStr(c.AuthorAssociation), c.IsBot, c.IsPRAuthor,
			c.Body, NullStr(c.ReviewState), nullInt(c.ReviewID), nullInt(c.InReplyToID), nullInt(c.ThreadRootID),
			NullStr(c.ThreadNodeID), NullStr(c.Path), nullInt(int64(c.Line)), nullInt(int64(c.OriginalLine)), NullStr(c.Side),
			NullStr(c.CommitSHA), NullStr(c.OriginalCommitSHA), NullStr(c.DiffHunk), NullStr(c.HTMLURL),
			nullBool(c.IsResolved), nullBool(c.IsOutdated), c.CodeChanged, NullStr(c.CreatedAt), NullStr(c.UpdatedAt),
			hash, NullStr(c.RawJSON), now, now)
		if err != nil {
			return Unchanged, fmt.Errorf("insert comment %s/%d: %w", c.Source, c.GitHubID, err)
		}
		return Inserted, nil
	}
	if err != nil {
		return Unchanged, err
	}

	// Keep previously known resolution state when this fetch could not
	// determine it (e.g. the GraphQL call failed).
	if c.IsResolved == nil && oldResolved.Valid {
		v := oldResolved.Bool
		c.IsResolved = &v
	}
	if c.IsOutdated == nil && oldOutdated.Valid {
		v := oldOutdated.Bool
		c.IsOutdated = &v
	}

	result := Unchanged
	if oldHash != hash {
		if _, err := tx.ExecContext(ctx, `INSERT INTO comment_revisions(comment_id, body, body_hash, updated_at, recorded_at)
			VALUES (?,?,?,?,?)`, id, oldBody, oldHash, NullStr(oldUpd), now); err != nil {
			return Unchanged, err
		}
		result = Updated
	} else if deleted.Valid || oldUpd != c.UpdatedAt {
		result = Updated
	}

	_, err = tx.ExecContext(ctx, `UPDATE review_comments SET node_id=?, pr_id=?, author=?, author_association=?, is_bot=?,
		is_pr_author=?, body=?, review_state=?, review_id=?, in_reply_to_id=?, thread_root_id=?, thread_node_id=?, path=?,
		line=?, original_line=?, side=?, commit_sha=?, original_commit_sha=?, diff_hunk=?, html_url=?, is_resolved=?,
		is_outdated=?, code_changed=?, created_at=?, updated_at=?, body_hash=?, raw_json=?, deleted_at=NULL, last_seen_at=?
		WHERE id = ?`,
		NullStr(c.NodeID), c.PRID, c.Author, NullStr(c.AuthorAssociation), c.IsBot, c.IsPRAuthor, c.Body,
		NullStr(c.ReviewState), nullInt(c.ReviewID), nullInt(c.InReplyToID), nullInt(c.ThreadRootID), NullStr(c.ThreadNodeID),
		NullStr(c.Path), nullInt(int64(c.Line)), nullInt(int64(c.OriginalLine)), NullStr(c.Side), NullStr(c.CommitSHA),
		NullStr(c.OriginalCommitSHA), NullStr(c.DiffHunk), NullStr(c.HTMLURL), nullBool(c.IsResolved),
		nullBool(c.IsOutdated), c.CodeChanged, NullStr(c.CreatedAt), NullStr(c.UpdatedAt), hash, NullStr(c.RawJSON), now, id)
	if err != nil {
		return Unchanged, fmt.Errorf("update comment %s/%d: %w", c.Source, c.GitHubID, err)
	}
	return result, nil
}

// MarkMissingDeleted soft-deletes comments of a PR/source that were not
// returned by the latest complete fetch. Returns the number marked.
func (d *DB) MarkMissingDeleted(ctx context.Context, tx *sql.Tx, prID int64, source string, seen []int64) (int, error) {
	if seen == nil {
		seen = []int64{}
	}
	ids, _ := json.Marshal(seen)
	res, err := tx.ExecContext(ctx, `UPDATE review_comments SET deleted_at = ?
		WHERE pr_id = ? AND source = ? AND deleted_at IS NULL
		AND github_id NOT IN (SELECT value FROM json_each(?))`, d.NowTS(), prID, source, string(ids))
	if err != nil {
		return 0, err
	}
	n, _ := res.RowsAffected()
	return int(n), nil
}

// ---------------------------------------------------------------------------
// Sync runs
// ---------------------------------------------------------------------------

type SyncRun struct {
	ID               int64         `json:"id"`
	Trigger          string        `json:"trigger"`
	StartedAt        string        `json:"started_at"`
	FinishedAt       string        `json:"finished_at"`
	Status           string        `json:"status"`
	ReposTotal       int           `json:"repos_total"`
	ReposFailed      int           `json:"repos_failed"`
	PRsSynced        int           `json:"prs_synced"`
	CommentsUpserted int           `json:"comments_upserted"`
	CommentsDeleted  int           `json:"comments_deleted"`
	RulesTotal       int           `json:"rules_total"`
	SkillBuildID     int64         `json:"skill_build_id"`
	Error            string        `json:"error"`
	Repos            []SyncRunRepo `json:"repos,omitempty"`
}

type SyncRunRepo struct {
	RepoID     int64  `json:"repo_id"`
	FullName   string `json:"full_name"`
	Status     string `json:"status"`
	PRsSynced  int    `json:"prs_synced"`
	Comments   int    `json:"comments"`
	Error      string `json:"error"`
	StartedAt  string `json:"started_at"`
	FinishedAt string `json:"finished_at"`
}

// RecoverInterrupted marks runs/repos left in 'running' by a crashed process.
// Must only be called while holding the pipeline lock.
func (d *DB) RecoverInterrupted(ctx context.Context) (int, error) {
	now := d.NowTS()
	res, err := d.ExecContext(ctx, `UPDATE sync_runs SET status='interrupted', finished_at=?, error=COALESCE(error,'process ended before the run finished') WHERE status='running'`, now)
	if err != nil {
		return 0, err
	}
	n, _ := res.RowsAffected()
	if _, err := d.ExecContext(ctx, `UPDATE sync_run_repos SET status='interrupted', finished_at=? WHERE status='running'`, now); err != nil {
		return int(n), err
	}
	_, err = d.ExecContext(ctx, `UPDATE repositories SET sync_status='interrupted' WHERE sync_status='running'`)
	return int(n), err
}

func (d *DB) StartRun(ctx context.Context, trigger string) (int64, error) {
	res, err := d.ExecContext(ctx, `INSERT INTO sync_runs(trigger, started_at, status) VALUES (?, ?, 'running')`, trigger, d.NowTS())
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

func (d *DB) FinishRun(ctx context.Context, r SyncRun) error {
	_, err := d.ExecContext(ctx, `UPDATE sync_runs SET finished_at=?, status=?, repos_total=?, repos_failed=?, prs_synced=?,
		comments_upserted=?, comments_deleted=?, rules_total=?, skill_build_id=?, error=? WHERE id=?`,
		d.NowTS(), r.Status, r.ReposTotal, r.ReposFailed, r.PRsSynced, r.CommentsUpserted, r.CommentsDeleted,
		r.RulesTotal, nullInt(r.SkillBuildID), NullStr(r.Error), r.ID)
	return err
}

func (d *DB) StartRunRepo(ctx context.Context, runID, repoID int64) error {
	_, err := d.ExecContext(ctx, `INSERT OR REPLACE INTO sync_run_repos(run_id, repo_id, status, started_at) VALUES (?,?, 'running', ?)`,
		runID, repoID, d.NowTS())
	return err
}

func (d *DB) FinishRunRepo(ctx context.Context, runID, repoID int64, status string, prs, comments int, errMsg string) error {
	_, err := d.ExecContext(ctx, `UPDATE sync_run_repos SET status=?, prs_synced=?, comments=?, error=?, finished_at=? WHERE run_id=? AND repo_id=?`,
		status, prs, comments, NullStr(errMsg), d.NowTS(), runID, repoID)
	return err
}

// ListRuns returns the most recent runs, newest first, with per-repo details.
func (d *DB) ListRuns(ctx context.Context, limit int) ([]SyncRun, error) {
	if limit <= 0 {
		limit = 50
	}
	rows, err := d.QueryContext(ctx, `SELECT id, trigger, started_at, COALESCE(finished_at,''), status, repos_total, repos_failed,
		prs_synced, comments_upserted, comments_deleted, rules_total, COALESCE(skill_build_id,0), COALESCE(error,'')
		FROM sync_runs ORDER BY id DESC LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	var runs []SyncRun
	for rows.Next() {
		var r SyncRun
		if err := rows.Scan(&r.ID, &r.Trigger, &r.StartedAt, &r.FinishedAt, &r.Status, &r.ReposTotal, &r.ReposFailed,
			&r.PRsSynced, &r.CommentsUpserted, &r.CommentsDeleted, &r.RulesTotal, &r.SkillBuildID, &r.Error); err != nil {
			rows.Close()
			return nil, err
		}
		runs = append(runs, r)
	}
	rows.Close()
	for i := range runs {
		rr, err := d.QueryContext(ctx, `SELECT s.repo_id, r.full_name, s.status, s.prs_synced, s.comments, COALESCE(s.error,''),
			s.started_at, COALESCE(s.finished_at,'') FROM sync_run_repos s JOIN repositories r ON r.id = s.repo_id
			WHERE s.run_id = ? ORDER BY r.full_name`, runs[i].ID)
		if err != nil {
			return nil, err
		}
		for rr.Next() {
			var x SyncRunRepo
			if err := rr.Scan(&x.RepoID, &x.FullName, &x.Status, &x.PRsSynced, &x.Comments, &x.Error, &x.StartedAt, &x.FinishedAt); err != nil {
				rr.Close()
				return nil, err
			}
			runs[i].Repos = append(runs[i].Repos, x)
		}
		rr.Close()
	}
	return runs, nil
}
