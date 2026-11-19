package db

import (
	"context"
	"fmt"
)

type SkillBuild struct {
	ID                   int64  `json:"id"`
	CreatedAt            string `json:"created_at"`
	Status               string `json:"status"` // published|unchanged|failed|dry-run
	ContentHash          string `json:"content_hash"`
	KnowledgeFingerprint string `json:"knowledge_fingerprint"`
	GeneratorVersion     string `json:"generator_version"`
	RuleCount            int    `json:"rule_count"`
	FileCount            int    `json:"file_count"`
	TotalBytes           int    `json:"total_bytes"`
	VersionDir           string `json:"version_dir"`
	NetworkCalls         int    `json:"network_calls"`
	LLMCalls             int    `json:"llm_calls"`
	Changes              string `json:"changes"`
	Error                string `json:"error"`
}

func (d *DB) InsertBuild(ctx context.Context, b SkillBuild) (int64, error) {
	if b.CreatedAt == "" {
		b.CreatedAt = d.NowTS()
	}
	res, err := d.ExecContext(ctx, `INSERT INTO skill_builds(created_at, status, content_hash, knowledge_fingerprint, generator_version,
		rule_count, file_count, total_bytes, version_dir, network_calls, llm_calls, changes, error)
		VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?)`, b.CreatedAt, b.Status, NullStr(b.ContentHash), NullStr(b.KnowledgeFingerprint),
		b.GeneratorVersion, b.RuleCount, b.FileCount, b.TotalBytes, NullStr(b.VersionDir), b.NetworkCalls, b.LLMCalls,
		NullStr(b.Changes), NullStr(b.Error))
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

const buildCols = `id, created_at, status, COALESCE(content_hash,''), COALESCE(knowledge_fingerprint,''), generator_version,
	rule_count, file_count, total_bytes, COALESCE(version_dir,''), network_calls, llm_calls, COALESCE(changes,''), COALESCE(error,'')`

func scanBuild(sc interface{ Scan(...any) error }) (SkillBuild, error) {
	var b SkillBuild
	err := sc.Scan(&b.ID, &b.CreatedAt, &b.Status, &b.ContentHash, &b.KnowledgeFingerprint, &b.GeneratorVersion,
		&b.RuleCount, &b.FileCount, &b.TotalBytes, &b.VersionDir, &b.NetworkCalls, &b.LLMCalls, &b.Changes, &b.Error)
	return b, err
}

// LastBuild returns the newest build, optionally restricted to a status.
func (d *DB) LastBuild(ctx context.Context, status string) (SkillBuild, error) {
	if status == "" {
		return scanBuild(d.QueryRowContext(ctx, `SELECT `+buildCols+` FROM skill_builds ORDER BY id DESC LIMIT 1`))
	}
	return scanBuild(d.QueryRowContext(ctx, `SELECT `+buildCols+` FROM skill_builds WHERE status = ? ORDER BY id DESC LIMIT 1`, status))
}

func (d *DB) ListBuilds(ctx context.Context, limit int) ([]SkillBuild, error) {
	if limit <= 0 {
		limit = 50
	}
	rows, err := d.QueryContext(ctx, fmt.Sprintf(`SELECT %s FROM skill_builds ORDER BY id DESC LIMIT %d`, buildCols, limit))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []SkillBuild
	for rows.Next() {
		b, err := scanBuild(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, b)
	}
	return out, rows.Err()
}

// PublishableRules returns rules eligible for the skill, in a stable order.
// Merged, rejected and superseded rules are never included.
func (d *DB) PublishableRules(ctx context.Context, includePending bool, minOcc int) ([]Rule, error) {
	q := `SELECT ` + ruleCols + ` FROM rules WHERE occurrences >= ? AND (status = 'approved'`
	if includePending {
		q += ` OR status = 'pending'`
	}
	q += `) ORDER BY id`
	rows, err := d.QueryContext(ctx, q, minOcc)
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
