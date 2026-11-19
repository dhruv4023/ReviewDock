package db

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"sort"
	"strings"
	"time"
)

// Settings configures the knowledge pipeline. Stored as a single JSON
// document under kb_settings.key = 'settings' so new fields get defaults.
type Settings struct {
	// In-app scheduler, same model as the scheduled rebase: run at the given
	// wall-clock times ("09:00 AM") while review-dock is open. Nothing runs
	// when the app is closed.
	ScheduleEnabled bool     `json:"schedule_enabled"`
	ScheduleTimes   []string `json:"schedule_times"`

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
		ScheduleEnabled: false,
		ScheduleTimes:   []string{},
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
	times := make([]string, 0, len(s.ScheduleTimes))
	seen := map[string]bool{}
	for _, t := range s.ScheduleTimes {
		if n, ok := NormalizeTime(t); ok && !seen[n] {
			seen[n] = true
			times = append(times, n)
		}
	}
	sort.Slice(times, func(i, j int) bool { return clockMinutes(times[i]) < clockMinutes(times[j]) })
	s.ScheduleTimes = times
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

// NormalizeTime parses a schedule time ("9:00 am", "09:00 AM", "21:00") and
// returns it in the app's canonical "03:04 PM" form.
func NormalizeTime(t string) (string, bool) {
	t = strings.ToUpper(strings.TrimSpace(t))
	for _, layout := range []string{"03:04 PM", "3:04 PM", "15:04"} {
		if v, err := time.Parse(layout, t); err == nil {
			return v.Format("03:04 PM"), true
		}
	}
	return "", false
}

func clockMinutes(t string) int {
	v, _ := time.Parse("03:04 PM", t)
	return v.Hour()*60 + v.Minute()
}
