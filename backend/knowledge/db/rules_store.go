package db

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
)

// Rule statuses.
const (
	StatusPending    = "pending"
	StatusApproved   = "approved"
	StatusRejected   = "rejected"
	StatusSuperseded = "superseded"
	StatusMerged     = "merged"
)

var validStatus = map[string]bool{StatusPending: true, StatusApproved: true, StatusRejected: true, StatusSuperseded: true}

type Rule struct {
	ID                int64   `json:"id"`
	Code              string  `json:"code"` // R-0001
	Key               string  `json:"rule_key"`
	Origin            string  `json:"origin"`
	Category          string  `json:"category"`
	Subcategory       string  `json:"subcategory"`
	Title             string  `json:"title"`
	Guidance          string  `json:"guidance"`
	Scope             string  `json:"scope"`
	Severity          string  `json:"severity"`
	Status            string  `json:"status"`
	MergedIntoID      int64   `json:"merged_into_id"`
	Edited            bool    `json:"edited"`
	Occurrences       int     `json:"occurrences"`
	DistinctPRs       int     `json:"distinct_prs"`
	DistinctReviewers int     `json:"distinct_reviewers"`
	RequirementRatio  float64 `json:"requirement_ratio"`
	AddressedRatio    float64 `json:"addressed_ratio"`
	Confidence        float64 `json:"confidence"`
	FirstSeenAt       string  `json:"first_seen_at"`
	LastSeenAt        string  `json:"last_seen_at"`
	Note              string  `json:"note"`
	CreatedAt         string  `json:"created_at"`
	UpdatedAt         string  `json:"updated_at"`
	StatusChangedAt   string  `json:"status_changed_at"`
}

// RuleCode formats the public, stable rule identifier.
func RuleCode(id int64) string { return fmt.Sprintf("R-%04d", id) }

const ruleCols = `id, rule_key, origin, category, subcategory, title, guidance, scope, severity, status,
	COALESCE(merged_into_id,0), edited, occurrences, distinct_prs, distinct_reviewers, requirement_ratio, addressed_ratio,
	confidence, COALESCE(first_seen_at,''), COALESCE(last_seen_at,''), note, created_at, updated_at, COALESCE(status_changed_at,'')`

func scanRule(sc interface{ Scan(...any) error }) (Rule, error) {
	var r Rule
	err := sc.Scan(&r.ID, &r.Key, &r.Origin, &r.Category, &r.Subcategory, &r.Title, &r.Guidance, &r.Scope, &r.Severity,
		&r.Status, &r.MergedIntoID, &r.Edited, &r.Occurrences, &r.DistinctPRs, &r.DistinctReviewers, &r.RequirementRatio,
		&r.AddressedRatio, &r.Confidence, &r.FirstSeenAt, &r.LastSeenAt, &r.Note, &r.CreatedAt, &r.UpdatedAt, &r.StatusChangedAt)
	r.Code = RuleCode(r.ID)
	return r, err
}

type RuleFilter struct {
	Status   string `json:"status"`   // "" = all
	Category string `json:"category"` // "" = all
	Query    string `json:"query"`
	MinOcc   int    `json:"min_occurrences"`
	Limit    int    `json:"limit"`
}

func (d *DB) ListRules(ctx context.Context, f RuleFilter) ([]Rule, error) {
	q := `SELECT ` + ruleCols + ` FROM rules WHERE 1=1`
	var args []any
	if f.Status != "" {
		q += ` AND status = ?`
		args = append(args, f.Status)
	}
	if f.Category != "" {
		q += ` AND category = ?`
		args = append(args, f.Category)
	}
	if f.MinOcc > 0 {
		q += ` AND occurrences >= ?`
		args = append(args, f.MinOcc)
	}
	if s := strings.TrimSpace(f.Query); s != "" {
		q += ` AND (title LIKE ? OR guidance LIKE ? OR note LIKE ? OR rule_key LIKE ?)`
		like := "%" + s + "%"
		args = append(args, like, like, like, like)
	}
	q += ` ORDER BY occurrences DESC, distinct_prs DESC, id ASC`
	if f.Limit <= 0 || f.Limit > 2000 {
		f.Limit = 500
	}
	q += fmt.Sprintf(" LIMIT %d", f.Limit)
	rows, err := d.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Rule
	for rows.Next() {
		r, err := scanRule(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

func (d *DB) GetRule(ctx context.Context, id int64) (Rule, error) {
	return scanRule(d.QueryRowContext(ctx, `SELECT `+ruleCols+` FROM rules WHERE id = ?`, id))
}

// SetRuleStatus approves/rejects/supersedes/re-opens a rule.
func (d *DB) SetRuleStatus(ctx context.Context, id int64, status, note string) error {
	if !validStatus[status] {
		return fmt.Errorf("invalid status %q", status)
	}
	res, err := d.ExecContext(ctx, `UPDATE rules SET status=?, merged_into_id=NULL, status_changed_at=?, updated_at=?,
		note = CASE WHEN ? <> '' THEN ? ELSE note END WHERE id=?`, status, d.NowTS(), d.NowTS(), note, note, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return sql.ErrNoRows
	}
	return nil
}

type RuleEdit struct {
	Title       string `json:"title"`
	Guidance    string `json:"guidance"`
	Category    string `json:"category"`
	Subcategory string `json:"subcategory"`
	Severity    string `json:"severity"`
	Note        string `json:"note"`
}

// EditRule changes rule text; the extractor will no longer overwrite it.
func (d *DB) EditRule(ctx context.Context, id int64, e RuleEdit) error {
	e.Title = strings.TrimSpace(e.Title)
	if e.Title == "" {
		return errors.New("rule title cannot be empty")
	}
	if e.Severity == "" {
		e.Severity = "context-dependent"
	}
	_, err := d.ExecContext(ctx, `UPDATE rules SET title=?, guidance=?, category=?, subcategory=?, severity=?, note=?, edited=1, updated_at=? WHERE id=?`,
		e.Title, strings.TrimSpace(e.Guidance), e.Category, e.Subcategory, e.Severity, e.Note, d.NowTS(), id)
	return err
}

// MergeRules folds sources into target: their evidence counts for the target
// and they are hidden from the skill.
func (d *DB) MergeRules(ctx context.Context, target int64, sources []int64) error {
	return d.InTx(ctx, func(tx *sql.Tx) error {
		for _, s := range sources {
			if s == target {
				continue
			}
			if _, err := tx.ExecContext(ctx, `UPDATE rules SET status='merged', merged_into_id=?, status_changed_at=?, updated_at=? WHERE id=?`,
				target, d.NowTS(), d.NowTS(), s); err != nil {
				return err
			}
			// Re-point anything previously merged into the source.
			if _, err := tx.ExecContext(ctx, `UPDATE rules SET merged_into_id=? WHERE merged_into_id=?`, target, s); err != nil {
				return err
			}
		}
		return nil
	})
}

// CreateManualRule creates a rule from a selected comment (manual promotion).
func (d *DB) CreateManualRule(ctx context.Context, commentID int64, e RuleEdit) (int64, error) {
	if strings.TrimSpace(e.Title) == "" {
		return 0, errors.New("rule title cannot be empty")
	}
	if e.Category == "" {
		e.Category = "general"
	}
	if e.Severity == "" {
		e.Severity = "context-dependent"
	}
	var id int64
	err := d.InTx(ctx, func(tx *sql.Tx) error {
		now := d.NowTS()
		res, err := tx.ExecContext(ctx, `INSERT INTO rules(rule_key, origin, category, subcategory, title, guidance, severity, status, edited, note, created_at, updated_at)
			VALUES (?, 'manual', ?, ?, ?, ?, ?, 'pending', 1, ?, ?, ?)`, fmt.Sprintf("manual:c%d:%s", commentID, now),
			e.Category, e.Subcategory, strings.TrimSpace(e.Title), strings.TrimSpace(e.Guidance), e.Severity, e.Note, now, now)
		if err != nil {
			return err
		}
		id, _ = res.LastInsertId()
		if commentID > 0 {
			_, err = tx.ExecContext(ctx, `INSERT INTO rule_evidence(rule_id, comment_id, link_source, feedback_kind) VALUES (?,?, 'manual', 'requirement')`, id, commentID)
		}
		return err
	})
	return id, err
}

// SetEvidenceLink adds a manual link, or excludes/restores an auto link.
// mode: "manual" | "excluded" | "remove".
func (d *DB) SetEvidenceLink(ctx context.Context, ruleID, commentID int64, mode string) error {
	switch mode {
	case "manual", "excluded":
		_, err := d.ExecContext(ctx, `INSERT INTO rule_evidence(rule_id, comment_id, link_source) VALUES (?,?,?)
			ON CONFLICT(rule_id, comment_id) DO UPDATE SET link_source=excluded.link_source`, ruleID, commentID, mode)
		return err
	case "remove":
		_, err := d.ExecContext(ctx, `DELETE FROM rule_evidence WHERE rule_id=? AND comment_id=?`, ruleID, commentID)
		return err
	}
	return fmt.Errorf("invalid evidence mode %q", mode)
}

// ---------------------------------------------------------------------------
// Comments
// ---------------------------------------------------------------------------

type CommentView struct {
	ID           int64   `json:"id"`
	Source       string  `json:"source"`
	GitHubID     int64   `json:"github_id"`
	Repo         string  `json:"repo"`
	PRNumber     int     `json:"pr_number"`
	PRTitle      string  `json:"pr_title"`
	PRURL        string  `json:"pr_url"`
	PRAuthor     string  `json:"pr_author"`
	Author       string  `json:"author"`
	Body         string  `json:"body"`
	Path         string  `json:"path"`
	Line         int     `json:"line"`
	OriginalLine int     `json:"original_line"`
	DiffHunk     string  `json:"diff_hunk"`
	HTMLURL      string  `json:"html_url"`
	ReviewState  string  `json:"review_state"`
	ThreadRootID int64   `json:"thread_root_id"`
	IsResolved   *bool   `json:"is_resolved"`
	IsOutdated   *bool   `json:"is_outdated"`
	CodeChanged  string  `json:"code_changed"`
	CreatedAt    string  `json:"created_at"`
	UpdatedAt    string  `json:"updated_at"`
	DeletedAt    string  `json:"deleted_at"`
	Kind         string  `json:"kind"`
	Category     string  `json:"category"`
	Subcategory  string  `json:"subcategory"`
	Signals      string  `json:"signals"`
	RuleIDs      []int64 `json:"rule_ids"`
	LinkSource   string  `json:"link_source,omitempty"` // when listed as rule evidence
	Revisions    int     `json:"revisions"`
}

type CommentFilter struct {
	Repo     string `json:"repo"`
	Reviewer string `json:"reviewer"`
	Category string `json:"category"`
	Kind     string `json:"kind"`
	Query    string `json:"query"`
	Since    string `json:"since"` // YYYY-MM-DD
	Until    string `json:"until"`
	PRNumber int    `json:"pr_number"`
	Deleted  bool   `json:"include_deleted"`
	Limit    int    `json:"limit"`
	Offset   int    `json:"offset"`
}

const commentViewCols = `c.id, c.source, c.github_id, r.full_name, p.number, p.title, p.url, p.author, c.author, c.body,
	COALESCE(c.path,''), COALESCE(c.line,0), COALESCE(c.original_line,0), COALESCE(c.diff_hunk,''), COALESCE(c.html_url,''),
	COALESCE(c.review_state,''), COALESCE(c.thread_root_id,0), c.is_resolved, c.is_outdated, c.code_changed,
	COALESCE(c.created_at,''), COALESCE(c.updated_at,''), COALESCE(c.deleted_at,''),
	COALESCE(a.kind,''), COALESCE(a.category,''), COALESCE(a.subcategory,''), COALESCE(a.signals,''),
	(SELECT COUNT(*) FROM comment_revisions v WHERE v.comment_id = c.id)`

const commentViewFrom = ` FROM review_comments c JOIN pull_requests p ON p.id = c.pr_id JOIN repositories r ON r.id = p.repo_id
	LEFT JOIN comment_analysis a ON a.comment_id = c.id`

func scanCommentView(sc interface{ Scan(...any) error }, extra ...any) (CommentView, error) {
	var v CommentView
	var res, out sql.NullBool
	dest := []any{&v.ID, &v.Source, &v.GitHubID, &v.Repo, &v.PRNumber, &v.PRTitle, &v.PRURL, &v.PRAuthor, &v.Author, &v.Body,
		&v.Path, &v.Line, &v.OriginalLine, &v.DiffHunk, &v.HTMLURL, &v.ReviewState, &v.ThreadRootID, &res, &out,
		&v.CodeChanged, &v.CreatedAt, &v.UpdatedAt, &v.DeletedAt, &v.Kind, &v.Category, &v.Subcategory, &v.Signals, &v.Revisions}
	dest = append(dest, extra...)
	if err := sc.Scan(dest...); err != nil {
		return v, err
	}
	if res.Valid {
		b := res.Bool
		v.IsResolved = &b
	}
	if out.Valid {
		b := out.Bool
		v.IsOutdated = &b
	}
	return v, nil
}

// SearchComments returns matching comments (newest first) and the total count.
func (d *DB) SearchComments(ctx context.Context, f CommentFilter) ([]CommentView, int, error) {
	where := ` WHERE 1=1`
	var args []any
	if !f.Deleted {
		where += ` AND c.deleted_at IS NULL`
	}
	if f.Repo != "" {
		where += ` AND r.full_name = ?`
		args = append(args, f.Repo)
	}
	if f.Reviewer != "" {
		where += ` AND c.author = ?`
		args = append(args, f.Reviewer)
	}
	if f.Category != "" {
		where += ` AND a.category = ?`
		args = append(args, f.Category)
	}
	if f.Kind != "" {
		where += ` AND a.kind = ?`
		args = append(args, f.Kind)
	}
	if f.PRNumber > 0 {
		where += ` AND p.number = ?`
		args = append(args, f.PRNumber)
	}
	if f.Since != "" {
		where += ` AND c.created_at >= ?`
		args = append(args, f.Since)
	}
	if f.Until != "" {
		where += ` AND c.created_at < ?`
		args = append(args, f.Until+"T99")
	}
	if q := strings.TrimSpace(f.Query); q != "" {
		where += ` AND (c.body LIKE ? OR c.path LIKE ? OR p.title LIKE ?)`
		like := "%" + q + "%"
		args = append(args, like, like, like)
	}
	var total int
	if err := d.QueryRowContext(ctx, `SELECT COUNT(*)`+commentViewFrom+where, args...).Scan(&total); err != nil {
		return nil, 0, err
	}
	if f.Limit <= 0 || f.Limit > 500 {
		f.Limit = 100
	}
	rows, err := d.QueryContext(ctx, `SELECT `+commentViewCols+commentViewFrom+where+
		// Grouped by PR (most recently updated PR first) so each PR's comments
		// are contiguous, even across pages; chronological inside a PR.
		fmt.Sprintf(` ORDER BY p.updated_at DESC, p.id DESC, c.created_at ASC, c.id ASC LIMIT %d OFFSET %d`, f.Limit, f.Offset), args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	var out []CommentView
	for rows.Next() {
		v, err := scanCommentView(rows)
		if err != nil {
			return nil, 0, err
		}
		out = append(out, v)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, err
	}
	return out, total, d.attachRuleIDs(ctx, out)
}

func (d *DB) attachRuleIDs(ctx context.Context, cs []CommentView) error {
	for i := range cs {
		rows, err := d.QueryContext(ctx, `SELECT rule_id FROM rule_evidence WHERE comment_id=? AND link_source IN ('auto','manual') ORDER BY rule_id`, cs[i].ID)
		if err != nil {
			return err
		}
		for rows.Next() {
			var id int64
			rows.Scan(&id)
			cs[i].RuleIDs = append(cs[i].RuleIDs, id)
		}
		rows.Close()
	}
	return nil
}

// RuleEvidence returns the comments linked to a rule (and to rules merged into it).
func (d *DB) RuleEvidence(ctx context.Context, ruleID int64) ([]CommentView, error) {
	rows, err := d.QueryContext(ctx, `SELECT `+commentViewCols+`, e.link_source`+commentViewFrom+`
		JOIN rule_evidence e ON e.comment_id = c.id
		WHERE e.rule_id = ? OR e.rule_id IN (SELECT id FROM rules WHERE merged_into_id = ?)
		ORDER BY c.created_at ASC, c.id ASC`, ruleID, ruleID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []CommentView
	for rows.Next() {
		var ls string
		v, err := scanCommentView(rows, &ls)
		if err != nil {
			return nil, err
		}
		v.LinkSource = ls
		out = append(out, v)
	}
	return out, rows.Err()
}

// Thread returns every comment in the same inline thread (or just the comment).
func (d *DB) Thread(ctx context.Context, commentID int64) ([]CommentView, error) {
	var root sql.NullInt64
	var prID int64
	if err := d.QueryRowContext(ctx, `SELECT thread_root_id, pr_id FROM review_comments WHERE id=?`, commentID).Scan(&root, &prID); err != nil {
		return nil, err
	}
	q := `SELECT ` + commentViewCols + commentViewFrom + ` WHERE c.id = ?`
	args := []any{commentID}
	if root.Valid {
		q = `SELECT ` + commentViewCols + commentViewFrom + ` WHERE c.source='inline' AND c.pr_id=? AND c.thread_root_id=?`
		args = []any{prID, root.Int64}
	}
	rows, err := d.QueryContext(ctx, q+` ORDER BY c.created_at, c.id`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []CommentView
	for rows.Next() {
		v, err := scanCommentView(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

// CommentRevisions lists previous bodies of an edited comment, oldest first.
func (d *DB) CommentRevisions(ctx context.Context, commentID int64) ([]string, error) {
	rows, err := d.QueryContext(ctx, `SELECT body FROM comment_revisions WHERE comment_id=? ORDER BY id`, commentID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var b string
		rows.Scan(&b)
		out = append(out, b)
	}
	return out, rows.Err()
}

// ---------------------------------------------------------------------------
// Overview
// ---------------------------------------------------------------------------

type CountItem struct {
	Key   string `json:"key"`
	Count int    `json:"count"`
}

type Overview struct {
	Repositories   int         `json:"repositories"`
	PRs            int         `json:"prs"`
	Comments       int         `json:"comments"`
	ReviewerComms  int         `json:"reviewer_comments"`
	Rules          int         `json:"rules"`
	RulesByStatus  []CountItem `json:"rules_by_status"`
	KindCounts     []CountItem `json:"kind_counts"`
	CategoryCounts []CountItem `json:"category_counts"`
	TopReviewers   []CountItem `json:"top_reviewers"`
	TopRules       []Rule      `json:"top_rules"`
	LastRun        *SyncRun    `json:"last_run"`
	LastBuild      *SkillBuild `json:"last_build"`
	SchemaVersion  int         `json:"schema_version"`
	DBPath         string      `json:"db_path"`
}

func (d *DB) counts(ctx context.Context, q string, args ...any) ([]CountItem, error) {
	rows, err := d.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []CountItem
	for rows.Next() {
		var c CountItem
		if err := rows.Scan(&c.Key, &c.Count); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

func (d *DB) GetOverview(ctx context.Context) (Overview, error) {
	var o Overview
	o.DBPath = d.Path
	o.SchemaVersion, _ = d.SchemaVersion(ctx)
	if err := d.QueryRowContext(ctx, `SELECT (SELECT COUNT(*) FROM repositories), (SELECT COUNT(*) FROM pull_requests),
		(SELECT COUNT(*) FROM review_comments WHERE deleted_at IS NULL),
		(SELECT COUNT(*) FROM review_comments WHERE deleted_at IS NULL AND is_pr_author = 0 AND is_bot = 0),
		(SELECT COUNT(*) FROM rules)`).Scan(&o.Repositories, &o.PRs, &o.Comments, &o.ReviewerComms, &o.Rules); err != nil {
		return o, err
	}
	var err error
	if o.RulesByStatus, err = d.counts(ctx, `SELECT status, COUNT(*) FROM rules GROUP BY status ORDER BY status`); err != nil {
		return o, err
	}
	if o.KindCounts, err = d.counts(ctx, `SELECT kind, COUNT(*) FROM comment_analysis GROUP BY kind ORDER BY 2 DESC, 1`); err != nil {
		return o, err
	}
	if o.CategoryCounts, err = d.counts(ctx, `SELECT a.category, COUNT(*) FROM comment_analysis a
		WHERE a.kind IN ('requirement','suggestion') GROUP BY a.category ORDER BY 2 DESC, 1`); err != nil {
		return o, err
	}
	if o.TopReviewers, err = d.counts(ctx, `SELECT author, COUNT(*) FROM review_comments
		WHERE deleted_at IS NULL AND is_pr_author = 0 AND is_bot = 0 AND author <> '' GROUP BY author ORDER BY 2 DESC, 1 LIMIT 10`); err != nil {
		return o, err
	}
	if o.TopRules, err = d.ListRules(ctx, RuleFilter{Limit: 10}); err != nil {
		return o, err
	}
	if runs, err := d.ListRuns(ctx, 1); err == nil && len(runs) > 0 {
		o.LastRun = &runs[0]
	}
	if b, err := d.LastBuild(ctx, ""); err == nil {
		o.LastBuild = &b
	}
	return o, nil
}

// Reviewers lists distinct reviewer logins (for filters).
func (d *DB) Reviewers(ctx context.Context) ([]string, error) {
	rows, err := d.QueryContext(ctx, `SELECT DISTINCT author FROM review_comments WHERE is_pr_author = 0 AND is_bot = 0 AND author <> '' ORDER BY author`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var s string
		rows.Scan(&s)
		out = append(out, s)
	}
	return out, rows.Err()
}
