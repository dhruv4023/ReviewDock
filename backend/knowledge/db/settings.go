package db

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
)

// Settings configures the knowledge pipeline. Stored as a single JSON
// document under kb_settings.key = 'settings' so new fields get defaults.
type Settings struct {
	// In-app scheduler. The headless CLI ignores these (system cron decides).
	ScheduleEnabled bool `json:"schedule_enabled"`
	IntervalMinutes int  `json:"interval_minutes"`

	// Skill generation / publishing.
	SkillName      string   `json:"skill_name"`
	PublishTargets []string `json:"publish_targets"` // parent dirs; skill is linked at <dir>/<skill_name>
	AutoPublish    bool     `json:"auto_publish"`
	IncludePending bool     `json:"include_pending"` // include unapproved rules (marked) in the skill
	MinOccurrences int      `json:"min_occurrences"` // minimum evidence count for a rule to be published
	KeepVersions   int      `json:"keep_versions"`   // number of skill versions retained on disk

	// Extraction.
	IgnoredAuthors    []string `json:"ignored_authors"`      // bots / merge bots (case-insensitive)
	AutoApproveMinPRs int      `json:"auto_approve_min_prs"` // 0 = disabled; else signal rules seen in >= N PRs are auto-approved
}

// DefaultSettings returns the defaults used when nothing is stored.
func DefaultSettings() Settings {
	return Settings{
		ScheduleEnabled: true,
		IntervalMinutes: 360,
		SkillName:       "odoo-pr-review",
		PublishTargets:  []string{"~/.claude/skills"},
		AutoPublish:     true,
		IncludePending:  false,
		MinOccurrences:  1,
		KeepVersions:    10,
		IgnoredAuthors: []string{
			"robodoo", "fw-bot", "C3POdoo", "odoo-mergebot", "mergebot",
			"github-actions", "dependabot", "codecov", "runbot",
		},
		AutoApproveMinPRs: 0,
	}
}

// Normalize clamps values to sane ranges.
func (s *Settings) Normalize() {
	def := DefaultSettings()
	if s.IntervalMinutes < 15 {
		s.IntervalMinutes = 15
	}
	if s.SkillName == "" {
		s.SkillName = def.SkillName
	}
	if s.MinOccurrences < 1 {
		s.MinOccurrences = 1
	}
	if s.KeepVersions < 2 {
		s.KeepVersions = 2
	}
	if s.AutoApproveMinPRs < 0 {
		s.AutoApproveMinPRs = 0
	}
}

// LoadSettings returns stored settings merged over the defaults.
func (d *DB) LoadSettings(ctx context.Context) (Settings, error) {
	s := DefaultSettings()
	var raw string
	err := d.QueryRowContext(ctx, `SELECT value FROM kb_settings WHERE key = 'settings'`).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		return s, nil
	}
	if err != nil {
		return s, err
	}
	if err := json.Unmarshal([]byte(raw), &s); err != nil {
		return DefaultSettings(), nil
	}
	s.Normalize()
	return s, nil
}

// SaveSettings persists settings.
func (d *DB) SaveSettings(ctx context.Context, s Settings) error {
	s.Normalize()
	b, err := json.Marshal(s)
	if err != nil {
		return err
	}
	_, err = d.ExecContext(ctx, `INSERT INTO kb_settings(key, value) VALUES('settings', ?)
		ON CONFLICT(key) DO UPDATE SET value = excluded.value`, string(b))
	return err
}
