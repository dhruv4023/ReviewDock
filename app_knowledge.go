package main

// Wails bindings for the review-knowledge system. The desktop app is a thin
// client here: all logic lives in backend/knowledge/*, shared with the
// headless review-dock-kb CLI (same database, same lock).

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	wails "github.com/wailsapp/wails/v2/pkg/runtime"

	"review-dock/backend/github"
	"review-dock/backend/knowledge/db"
	"review-dock/backend/knowledge/extract"
	"review-dock/backend/knowledge/pipeline"
	"review-dock/backend/knowledge/skillgen"
	"review-dock/logger"
)

// startKnowledge opens the knowledge DB, registers tracked repositories and
// starts the interval scheduler. Failures are logged; the rest of the app
// keeps working without the knowledge features.
func (a *App) startKnowledge() {
	if a.storage == nil {
		return
	}
	dataDir := a.storage.DataDir()
	kb, err := pipeline.Open(dataDir, pipeline.NewLogger(dataDir, nil))
	if err != nil {
		logger.Errorf("knowledge: %v", err)
		wails.EventsEmit(a.ctx, "terminal:log", fmt.Sprintf("\u001b[31m[KNOWLEDGE] database unavailable: %v\u001b[0m\r\n", err))
		return
	}
	kb.Progress = func(e pipeline.Event) {
		wails.EventsEmit(a.ctx, "kb:progress", e)
		color := "36"
		if e.Stage == "error" {
			color = "31"
		} else if e.Stage == "done" || e.Stage == "publish" {
			color = "32"
		}
		wails.EventsEmit(a.ctx, "terminal:log", fmt.Sprintf("\u001b[%sm[KNOWLEDGE] %s\u001b[0m\r\n", color, e.Message))
	}
	a.kb = kb
	a.syncTrackedRepos()
	kb.StartScheduler(a.ctx, func() bool {
		s, err := github.GetSession(a.ctx)
		return err == nil && s != nil
	})
}

func (a *App) stopKnowledge() {
	if a.kb != nil {
		a.kb.DB.Close()
	}
}

func (a *App) knowledge() (*pipeline.Pipeline, error) {
	if a.kb == nil {
		return nil, errors.New("knowledge database is not available (see logs)")
	}
	return a.kb, nil
}

func (a *App) syncTrackedRepos() {
	if a.kb == nil || a.storage == nil {
		return
	}
	if repos, err := a.storage.ReadRepos(); err == nil {
		if _, err := a.kb.RegisterTracked(a.ctx, repos); err != nil {
			logger.Errorf("knowledge: register repos: %v", err)
		}
	}
}

// ---------------------------------------------------------------------------
// Overview & metadata
// ---------------------------------------------------------------------------

type KBCategory struct {
	ID    string `json:"id"`
	Title string `json:"title"`
}

type KBMeta struct {
	Categories []KBCategory `json:"categories"`
	Reviewers  []string     `json:"reviewers"`
	Running    bool         `json:"running"`
}

func (a *App) KBOverview() (*db.Overview, error) {
	kb, err := a.knowledge()
	if err != nil {
		return nil, err
	}
	o, err := kb.DB.GetOverview(a.ctx)
	return &o, err
}

func (a *App) KBMeta() (*KBMeta, error) {
	kb, err := a.knowledge()
	if err != nil {
		return nil, err
	}
	m := &KBMeta{Running: kb.Running()}
	for _, c := range extract.Categories {
		m.Categories = append(m.Categories, KBCategory{c.ID, c.Title})
	}
	m.Reviewers, err = kb.DB.Reviewers(a.ctx)
	return m, err
}

// ---------------------------------------------------------------------------
// Synchronization
// ---------------------------------------------------------------------------

func (a *App) KBRepositories() ([]db.Repository, error) {
	kb, err := a.knowledge()
	if err != nil {
		return nil, err
	}
	a.syncTrackedRepos()
	return kb.DB.ListRepositories(a.ctx)
}

func (a *App) KBAddRepository(fullName, author string) error {
	kb, err := a.knowledge()
	if err != nil {
		return err
	}
	owner, name, ok := strings.Cut(strings.TrimSpace(fullName), "/")
	if !ok || owner == "" || name == "" {
		return errors.New("expected owner/name")
	}
	r, err := kb.DB.EnsureRepository(a.ctx, owner, name)
	if err != nil {
		return err
	}
	return kb.DB.UpdateRepositoryConfig(a.ctx, r.ID, true, author)
}

func (a *App) KBUpdateRepository(id int64, enabled bool, author string) error {
	kb, err := a.knowledge()
	if err != nil {
		return err
	}
	return kb.DB.UpdateRepositoryConfig(a.ctx, id, enabled, author)
}

func (a *App) KBResetRepository(id int64) error {
	kb, err := a.knowledge()
	if err != nil {
		return err
	}
	return kb.DB.ResetRepositoryCheckpoint(a.ctx, id)
}

func (a *App) KBRemoveRepository(id int64) error {
	kb, err := a.knowledge()
	if err != nil {
		return err
	}
	return kb.DB.DeleteRepository(a.ctx, id)
}

func (a *App) KBSettings() (*db.Settings, error) {
	kb, err := a.knowledge()
	if err != nil {
		return nil, err
	}
	s, err := kb.DB.LoadSettings(a.ctx)
	return &s, err
}

func (a *App) KBSaveSettings(s db.Settings) error {
	kb, err := a.knowledge()
	if err != nil {
		return err
	}
	return kb.DB.SaveSettings(a.ctx, s)
}

// KBRunSync starts a run in the background; progress arrives as kb:progress
// events and completion as kb:done. repoIDs empty = all enabled repos.
func (a *App) KBRunSync(repoIDs []int64, trigger string) error {
	kb, err := a.knowledge()
	if err != nil {
		return err
	}
	if kb.Running() {
		return pipeline.ErrBusy
	}
	if trigger == "" {
		trigger = "manual"
	}
	if s, err := github.GetSession(a.ctx); err != nil || s == nil {
		return errors.New("not logged in to GitHub CLI; run 'gh auth login' first")
	}
	go func() {
		// a.ctx ends at shutdown: the run then stops and is recorded as interrupted.
		res, err := kb.Run(a.ctx, pipeline.Options{Trigger: trigger, RepoIDs: repoIDs})
		payload := map[string]any{"result": res}
		if err != nil {
			payload["error"] = err.Error()
		}
		wails.EventsEmit(a.ctx, "kb:done", payload)
	}()
	return nil
}

// KBRetryFailed re-runs only repositories whose last sync did not succeed.
func (a *App) KBRetryFailed() error {
	kb, err := a.knowledge()
	if err != nil {
		return err
	}
	repos, err := kb.DB.ListRepositories(a.ctx)
	if err != nil {
		return err
	}
	var ids []int64
	for _, r := range repos {
		if r.Enabled && (r.SyncStatus == "error" || r.SyncStatus == "rate_limited" || r.SyncStatus == "interrupted") {
			ids = append(ids, r.ID)
		}
	}
	if len(ids) == 0 {
		return errors.New("no failed repositories to retry")
	}
	return a.KBRunSync(ids, "retry")
}

func (a *App) KBRuns(limit int) ([]db.SyncRun, error) {
	kb, err := a.knowledge()
	if err != nil {
		return nil, err
	}
	return kb.DB.ListRuns(a.ctx, limit)
}

// ---------------------------------------------------------------------------
// Comments
// ---------------------------------------------------------------------------

type KBCommentPage struct {
	Items []db.CommentView `json:"items"`
	Total int              `json:"total"`
}

func (a *App) KBSearchComments(f db.CommentFilter) (*KBCommentPage, error) {
	kb, err := a.knowledge()
	if err != nil {
		return nil, err
	}
	items, total, err := kb.DB.SearchComments(a.ctx, f)
	return &KBCommentPage{Items: items, Total: total}, err
}

func (a *App) KBCommentThread(commentID int64) ([]db.CommentView, error) {
	kb, err := a.knowledge()
	if err != nil {
		return nil, err
	}
	return kb.DB.Thread(a.ctx, commentID)
}

func (a *App) KBCommentRevisions(commentID int64) ([]string, error) {
	kb, err := a.knowledge()
	if err != nil {
		return nil, err
	}
	return kb.DB.CommentRevisions(a.ctx, commentID)
}

// ---------------------------------------------------------------------------
// Rules
// ---------------------------------------------------------------------------

type KBRuleDetail struct {
	Rule     db.Rule          `json:"rule"`
	Evidence []db.CommentView `json:"evidence"`
	Merged   []db.Rule        `json:"merged"`
}

func (a *App) KBRules(f db.RuleFilter) ([]db.Rule, error) {
	kb, err := a.knowledge()
	if err != nil {
		return nil, err
	}
	return kb.DB.ListRules(a.ctx, f)
}

func (a *App) KBRule(id int64) (*KBRuleDetail, error) {
	kb, err := a.knowledge()
	if err != nil {
		return nil, err
	}
	r, err := kb.DB.GetRule(a.ctx, id)
	if err != nil {
		return nil, err
	}
	ev, err := kb.DB.RuleEvidence(a.ctx, id)
	if err != nil {
		return nil, err
	}
	all, err := kb.DB.ListRules(a.ctx, db.RuleFilter{Status: db.StatusMerged, Limit: 2000})
	if err != nil {
		return nil, err
	}
	d := &KBRuleDetail{Rule: r, Evidence: ev}
	for _, m := range all {
		if m.MergedIntoID == id {
			d.Merged = append(d.Merged, m)
		}
	}
	return d, nil
}

func (a *App) KBSetRuleStatus(ids []int64, status, note string) error {
	kb, err := a.knowledge()
	if err != nil {
		return err
	}
	for _, id := range ids {
		if err := kb.DB.SetRuleStatus(a.ctx, id, status, note); err != nil {
			return fmt.Errorf("%s: %w", db.RuleCode(id), err)
		}
	}
	return nil
}

func (a *App) KBEditRule(id int64, e db.RuleEdit) error {
	kb, err := a.knowledge()
	if err != nil {
		return err
	}
	return kb.DB.EditRule(a.ctx, id, e)
}

// KBMergeRules merges sources into target and refreshes statistics.
func (a *App) KBMergeRules(target int64, sources []int64) error {
	kb, err := a.knowledge()
	if err != nil {
		return err
	}
	if err := kb.DB.MergeRules(a.ctx, target, sources); err != nil {
		return err
	}
	_, err = kb.Extract(a.ctx)
	return err
}

func (a *App) KBCreateRuleFromComment(commentID int64, e db.RuleEdit) (int64, error) {
	kb, err := a.knowledge()
	if err != nil {
		return 0, err
	}
	id, err := kb.DB.CreateManualRule(a.ctx, commentID, e)
	if err != nil {
		return 0, err
	}
	_, err = kb.Extract(a.ctx)
	return id, err
}

// KBSetEvidence links/excludes a comment for a rule and refreshes statistics.
func (a *App) KBSetEvidence(ruleID, commentID int64, mode string) error {
	kb, err := a.knowledge()
	if err != nil {
		return err
	}
	if err := kb.DB.SetEvidenceLink(a.ctx, ruleID, commentID, mode); err != nil {
		return err
	}
	_, err = kb.Extract(a.ctx)
	return err
}

func (a *App) KBExtract() (*extract.Summary, error) {
	kb, err := a.knowledge()
	if err != nil {
		return nil, err
	}
	s, err := kb.Extract(a.ctx)
	return &s, err
}

// ---------------------------------------------------------------------------
// Skill
// ---------------------------------------------------------------------------

type KBSkillInfo struct {
	Version    string                   `json:"version"`
	SkillName  string                   `json:"skill_name"`
	CurrentDir string                   `json:"current_dir"`
	Current    string                   `json:"current_link"`
	Targets    []string                 `json:"targets"`
	Files      map[string]string        `json:"files"`
	FileOrder  []string                 `json:"file_order"`
	Builds     []db.SkillBuild          `json:"builds"`
	ZeroToken  pipeline.ZeroTokenReport `json:"zero_token"`
}

func (a *App) KBSkillInfo() (*KBSkillInfo, error) {
	kb, err := a.knowledge()
	if err != nil {
		return nil, err
	}
	s, err := kb.DB.LoadSettings(a.ctx)
	if err != nil {
		return nil, err
	}
	info := &KBSkillInfo{
		Version: skillgen.CurrentVersion(kb.SkillBase()), SkillName: s.SkillName, CurrentDir: skillgen.CurrentDir(kb.SkillBase()),
		Current: filepath.Join(kb.SkillBase(), "current"), Files: map[string]string{},
	}
	for _, t := range s.PublishTargets {
		info.Targets = append(info.Targets, filepath.Join(skillgen.ExpandHome(t), s.SkillName))
	}
	if info.CurrentDir != "" {
		filepath.WalkDir(info.CurrentDir, func(p string, d os.DirEntry, err error) error {
			if err != nil || d.IsDir() {
				return nil
			}
			rel, _ := filepath.Rel(info.CurrentDir, p)
			if b, err := os.ReadFile(p); err == nil {
				info.Files[filepath.ToSlash(rel)] = string(b)
				info.FileOrder = append(info.FileOrder, filepath.ToSlash(rel))
			}
			return nil
		})
		sort.Slice(info.FileOrder, func(i, j int) bool {
			if info.FileOrder[i] == "SKILL.md" || info.FileOrder[j] == "SKILL.md" {
				return info.FileOrder[i] == "SKILL.md"
			}
			return info.FileOrder[i] < info.FileOrder[j]
		})
	}
	if info.Builds, err = kb.DB.ListBuilds(a.ctx, 30); err != nil {
		return nil, err
	}
	last, _ := kb.DB.LastBuild(a.ctx, "")
	info.ZeroToken = pipeline.CheckZeroToken(last)
	return info, nil
}

func (a *App) KBPreviewSkill() (*pipeline.Preview, error) {
	kb, err := a.knowledge()
	if err != nil {
		return nil, err
	}
	p, err := kb.Preview(a.ctx)
	return &p, err
}

// KBRebuildSkill regenerates the skill from the database (no network, no
// AI). publish=false validates without publishing.
func (a *App) KBRebuildSkill(publish bool) (*pipeline.BuildResult, error) {
	kb, err := a.knowledge()
	if err != nil {
		return nil, err
	}
	br, err := kb.Build(a.ctx, publish)
	return &br, err
}

// KBReviewRules returns a compact checklist of the published skill's rules
// for the {{REVIEW_RULES}} placeholder of the review prompt. It reads the
// published artifact (metadata.json), the same interface agents consume.
func (a *App) KBReviewRules() (string, error) {
	kb, err := a.knowledge()
	if err != nil {
		return "", nil // prompt generation must not fail without the KB
	}
	b, err := os.ReadFile(filepath.Join(kb.SkillBase(), "current", "metadata.json"))
	if err != nil {
		return "", nil
	}
	var m struct {
		Version string `json:"version"`
		Rules   []struct {
			ID          string `json:"id"`
			Category    string `json:"category"`
			Severity    string `json:"severity"`
			Occurrences int    `json:"occurrences"`
			Title       string `json:"title"`
		} `json:"rules"`
	}
	if err := json.Unmarshal(b, &m); err != nil || len(m.Rules) == 0 {
		return "", nil
	}
	var sb strings.Builder
	fmt.Fprintf(&sb, "Historical reviewer rules (review-dock skill %s). Check the diff against each one that applies:\n", m.Version)
	for _, r := range m.Rules {
		fmt.Fprintf(&sb, "- %s [%s, %s, raised %d×] %s\n", r.ID, r.Category, r.Severity, r.Occurrences, r.Title)
	}
	return sb.String(), nil
}
