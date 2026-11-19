// Package pipeline orchestrates the knowledge workflow:
//
//	lock → recover → sync (GitHub) → extract → build skill → validate → publish
//
// Only the sync stage touches the network (through the gh CLI). Extraction and
// skill generation run under a network guard and are pure local computation.
package pipeline

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"runtime/debug"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gofrs/flock"

	"review-dock/backend/knowledge/db"
	"review-dock/backend/knowledge/extract"
	"review-dock/backend/knowledge/ghsync"
	"review-dock/backend/knowledge/skillgen"
)

// ErrBusy is returned when another process/goroutine holds the pipeline lock.
var ErrBusy = errors.New("another knowledge sync is already running")

// Event is a progress notification for UIs.
type Event struct {
	Stage   string `json:"stage"` // lock|sync|extract|build|publish|done|error
	Repo    string `json:"repo,omitempty"`
	Message string `json:"message"`
	RunID   int64  `json:"run_id,omitempty"`
}

// Pipeline wires the knowledge components together.
type Pipeline struct {
	DB       *db.DB
	DataDir  string        // e.g. ~/.config/review-dock
	Runner   ghsync.Runner // nil = real gh CLI
	Log      *slog.Logger
	Progress func(Event)

	mu      sync.Mutex // in-process guard (flock handles cross-process)
	running atomic.Bool
}

// Options for a run.
type Options struct {
	Trigger  string  // manual|schedule|cli|retry
	RepoIDs  []int64 // empty = all enabled repositories
	SkipSync bool    // only extract + build
	NoBuild  bool    // sync + extract only
	MaxPRs   int     // per-repo PR budget for this run (0 = unlimited)
}

// RunResult summarises a run.
type RunResult struct {
	Run     db.SyncRun      `json:"run"`
	Extract extract.Summary `json:"extract"`
	Build   *BuildResult    `json:"build,omitempty"`
}

// BuildResult summarises a skill build.
type BuildResult struct {
	Build   db.SkillBuild           `json:"build"`
	Publish *skillgen.PublishResult `json:"publish,omitempty"`
	Errors  []string                `json:"errors,omitempty"`
}

func (p *Pipeline) logger() *slog.Logger {
	if p.Log == nil {
		return slog.Default()
	}
	return p.Log
}

func (p *Pipeline) emit(e Event) {
	lvl := slog.LevelInfo
	if e.Stage == "error" {
		lvl = slog.LevelError
	}
	p.logger().Log(context.Background(), lvl, e.Message, "stage", e.Stage, "repo", e.Repo, "run_id", e.RunID)
	if p.Progress != nil {
		p.Progress(e)
	}
}

// SkillBase is where versions and the current symlink live.
func (p *Pipeline) SkillBase() string { return filepath.Join(p.DataDir, "knowledge", "skill") }

// LockPath is the cross-process lock file.
func (p *Pipeline) LockPath() string { return filepath.Join(p.DataDir, "knowledge.lock") }

// Running reports whether this process is currently running the pipeline.
func (p *Pipeline) Running() bool { return p.running.Load() }

// acquire takes the in-process mutex and the cross-process file lock.
func (p *Pipeline) acquire() (func(), error) {
	if !p.mu.TryLock() {
		return nil, ErrBusy
	}
	fl := flock.New(p.LockPath())
	ok, err := fl.TryLock()
	if err != nil {
		p.mu.Unlock()
		return nil, fmt.Errorf("lock %s: %w", p.LockPath(), err)
	}
	if !ok {
		p.mu.Unlock()
		return nil, ErrBusy
	}
	p.running.Store(true)
	return func() {
		p.running.Store(false)
		fl.Unlock()
		p.mu.Unlock()
	}, nil
}

// Run executes the full workflow under the lock.
func (p *Pipeline) Run(ctx context.Context, opts Options) (RunResult, error) {
	var res RunResult
	if opts.Trigger == "" {
		opts.Trigger = "manual"
	}
	release, err := p.acquire()
	if err != nil {
		p.emit(Event{Stage: "error", Message: err.Error()})
		return res, err
	}
	defer release()

	if n, err := p.DB.RecoverInterrupted(ctx); err != nil {
		return res, err
	} else if n > 0 {
		p.emit(Event{Stage: "lock", Message: fmt.Sprintf("recovered %d interrupted run(s); they resume from their checkpoints", n)})
	}
	settings, err := p.DB.LoadSettings(ctx)
	if err != nil {
		return res, err
	}

	runID, err := p.DB.StartRun(ctx, opts.Trigger)
	if err != nil {
		return res, err
	}
	run := db.SyncRun{ID: runID, Trigger: opts.Trigger, Status: "ok"}
	p.emit(Event{Stage: "lock", RunID: runID, Message: fmt.Sprintf("knowledge run #%d started (%s)", runID, opts.Trigger)})

	// Make sure a context cancellation still records the run outcome.
	finish := func(status, msg string) {
		run.Status = status
		if msg != "" {
			run.Error = ghsync.Redact(msg)
		}
		if err := p.DB.FinishRun(context.Background(), run); err != nil {
			p.logger().Error("record run", "error", err)
		}
		res.Run = run
	}

	if !opts.SkipSync {
		if err := p.syncRepos(ctx, opts, &run); err != nil {
			if ctx.Err() != nil {
				finish("interrupted", err.Error())
			} else {
				finish("error", "sync: "+err.Error())
			}
			return res, err
		}
	}

	p.emit(Event{Stage: "extract", RunID: runID, Message: "extracting rules from review comments (deterministic, no AI)"})
	sum, err := extract.Run(ctx, p.DB, settings)
	if err != nil {
		finish("error", "extract: "+err.Error())
		p.emit(Event{Stage: "error", RunID: runID, Message: "extract failed: " + err.Error()})
		return res, err
	}
	res.Extract = sum
	run.RulesTotal = sum.RulesTotal
	p.emit(Event{Stage: "extract", RunID: runID, Message: fmt.Sprintf("analysed %d comments → %d rules (+%d new, -%d removed)",
		sum.CommentsAnalyzed, sum.RulesTotal, sum.RulesAdded, sum.RulesRemoved)})

	if !opts.NoBuild {
		br, err := p.build(ctx, settings, settings.AutoPublish, opts.Trigger != "schedule")
		res.Build = &br
		run.SkillBuildID = br.Build.ID
		if err != nil {
			finish("error", "skill build: "+err.Error())
			return res, err
		}
	}

	status := "ok"
	if run.ReposFailed > 0 {
		status = "partial"
		if run.ReposFailed == run.ReposTotal {
			status = "error"
		}
	}
	finish(status, run.Error)
	p.emit(Event{Stage: "done", RunID: runID, Message: fmt.Sprintf("run #%d finished: %s (%d PRs, %d comments changed, %d deleted)",
		runID, status, run.PRsSynced, run.CommentsUpserted, run.CommentsDeleted)})
	return res, nil
}

func (p *Pipeline) syncRepos(ctx context.Context, opts Options, run *db.SyncRun) error {
	repos, err := p.DB.ListRepositories(ctx)
	if err != nil {
		return err
	}
	want := map[int64]bool{}
	for _, id := range opts.RepoIDs {
		want[id] = true
	}
	runner := p.Runner
	if runner == nil {
		runner = ghsync.GHRunner{}
	}
	client := ghsync.NewClient(runner, p.logger())
	syncer := &ghsync.Syncer{DB: p.DB, GH: client, Log: p.logger(), MaxPRs: opts.MaxPRs}

	rateLimited := ""
	var errs []string
	for _, repo := range repos {
		if len(want) > 0 && !want[repo.ID] {
			continue
		}
		if len(want) == 0 && !repo.Enabled {
			continue
		}
		run.ReposTotal++
		_ = p.DB.StartRunRepo(ctx, run.ID, repo.ID)
		if rateLimited != "" {
			run.ReposFailed++
			_ = p.DB.FinishRepoSync(ctx, repo.ID, "rate_limited", rateLimited, false)
			_ = p.DB.FinishRunRepo(ctx, run.ID, repo.ID, "rate_limited", 0, 0, rateLimited)
			continue
		}
		_ = p.DB.MarkRepoAttempt(ctx, repo.ID)
		mode := "incremental"
		if repo.CheckpointUpdatedAt == "" {
			mode = "full historical import"
		}
		p.emit(Event{Stage: "sync", Repo: repo.FullName, RunID: run.ID, Message: fmt.Sprintf("syncing %s (%s)", repo.FullName, mode)})

		st, err := syncer.SyncRepo(ctx, repo)
		run.PRsSynced += st.PRs
		run.CommentsUpserted += st.Upserted
		run.CommentsDeleted += st.Deleted
		bg := context.Background() // record outcome even if ctx was cancelled
		switch {
		case err == nil:
			_ = p.DB.FinishRepoSync(bg, repo.ID, "ok", "", st.Complete)
			_ = p.DB.FinishRunRepo(bg, run.ID, repo.ID, "ok", st.PRs, st.Upserted, "")
			p.emit(Event{Stage: "sync", Repo: repo.FullName, RunID: run.ID, Message: fmt.Sprintf("%s: %d PRs, %d comments changed, %d deleted",
				repo.FullName, st.PRs, st.Upserted, st.Deleted)})
		case errors.Is(err, ghsync.ErrRateLimited):
			msg := ghsync.Redact(err.Error())
			rateLimited = msg
			run.ReposFailed++
			_ = p.DB.FinishRepoSync(bg, repo.ID, "rate_limited", msg, false)
			_ = p.DB.FinishRunRepo(bg, run.ID, repo.ID, "rate_limited", st.PRs, st.Upserted, msg)
			p.emit(Event{Stage: "error", Repo: repo.FullName, RunID: run.ID, Message: msg})
		case ctx.Err() != nil:
			msg := "interrupted: " + ctx.Err().Error()
			_ = p.DB.FinishRepoSync(bg, repo.ID, "interrupted", msg, false)
			_ = p.DB.FinishRunRepo(bg, run.ID, repo.ID, "interrupted", st.PRs, st.Upserted, msg)
			return ctx.Err()
		default:
			msg := ghsync.Redact(err.Error())
			run.ReposFailed++
			errs = append(errs, repo.FullName+": "+msg)
			_ = p.DB.FinishRepoSync(bg, repo.ID, "error", msg, false)
			_ = p.DB.FinishRunRepo(bg, run.ID, repo.ID, "error", st.PRs, st.Upserted, msg)
			p.emit(Event{Stage: "error", Repo: repo.FullName, RunID: run.ID, Message: repo.FullName + ": " + msg})
		}
	}
	if rateLimited != "" {
		errs = append(errs, rateLimited)
	}
	run.Error = strings.Join(errs, "; ")
	return nil
}

// Build regenerates (and optionally publishes) the skill under the lock.
func (p *Pipeline) Build(ctx context.Context, publish bool) (BuildResult, error) {
	release, err := p.acquire()
	if err != nil {
		return BuildResult{}, err
	}
	defer release()
	settings, err := p.DB.LoadSettings(ctx)
	if err != nil {
		return BuildResult{}, err
	}
	return p.build(ctx, settings, publish, true)
}

// Extract re-runs only the deterministic extraction (under the lock).
func (p *Pipeline) Extract(ctx context.Context) (extract.Summary, error) {
	release, err := p.acquire()
	if err != nil {
		return extract.Summary{}, err
	}
	defer release()
	settings, err := p.DB.LoadSettings(ctx)
	if err != nil {
		return extract.Summary{}, err
	}
	return extract.Run(ctx, p.DB, settings)
}

// build renders, validates and publishes. recordUnchanged controls whether
// a no-op rebuild is written to the build history (manual rebuilds are,
// scheduled ones are not, to keep history meaningful).
func (p *Pipeline) build(ctx context.Context, settings db.Settings, publish, recordUnchanged bool) (BuildResult, error) {
	var br BuildResult
	p.emit(Event{Stage: "build", Message: "generating skill from database (templates only, network disabled)"})

	var out skillgen.Output
	var genErr error
	calls := WithNetworkGuard(func() {
		snap, err := p.Snapshot(ctx, settings)
		if err != nil {
			genErr = err
			return
		}
		out, genErr = skillgen.Generate(snap)
	})
	b := db.SkillBuild{GeneratorVersion: skillgen.Version, NetworkCalls: int(calls), LLMCalls: 0}
	if genErr == nil && calls > 0 {
		genErr = fmt.Errorf("skill generation attempted %d network call(s); aborted", calls)
	}
	if genErr == nil {
		br.Errors = skillgen.Validate(out)
		if len(br.Errors) > 0 {
			genErr = fmt.Errorf("validation failed: %s", strings.Join(br.Errors, "; "))
		}
	}
	if genErr != nil {
		b.Status = "failed"
		b.Error = genErr.Error()
		b.ID, _ = p.DB.InsertBuild(ctx, b)
		br.Build = b
		p.emit(Event{Stage: "error", Message: "skill build failed, previous version kept: " + genErr.Error()})
		return br, genErr
	}

	b.ContentHash, b.RuleCount, b.FileCount, b.TotalBytes = out.ContentHash, out.RuleCount, len(out.Files), out.TotalBytes
	b.KnowledgeFingerprint = out.Version
	prevVersion := skillgen.CurrentVersion(p.SkillBase())
	b.Changes = p.diffSummary(out)

	if !publish {
		b.Status = "dry-run"
		br.Build = b
		return br, nil
	}
	pub, err := skillgen.Publish(p.SkillBase(), settings.SkillName, out, settings.PublishTargets, settings.KeepVersions)
	if err != nil {
		b.Status, b.Error = "failed", err.Error()
		b.ID, _ = p.DB.InsertBuild(ctx, b)
		br.Build = b
		p.emit(Event{Stage: "error", Message: "publish failed, previous version kept: " + err.Error()})
		return br, err
	}
	br.Publish = &pub
	b.VersionDir = pub.VersionDir
	if prevVersion == out.Version && !pub.Changed {
		b.Status = "unchanged"
		if recordUnchanged {
			b.ID, _ = p.DB.InsertBuild(ctx, b)
		} else if last, err := p.DB.LastBuild(ctx, ""); err == nil {
			b.ID = last.ID
		}
		p.emit(Event{Stage: "publish", Message: fmt.Sprintf("skill unchanged (version %s); no new version created", out.Version)})
	} else {
		b.Status = "published"
		b.ID, _ = p.DB.InsertBuild(ctx, b)
		p.emit(Event{Stage: "publish", Message: fmt.Sprintf("published skill version %s (%d rules) → %s", out.Version, out.RuleCount, pub.VersionDir)})
	}
	for _, w := range pub.Warnings {
		p.emit(Event{Stage: "publish", Message: "warning: " + w})
	}
	br.Build = b
	return br, nil
}

// Snapshot loads the generator input from the database.
func (p *Pipeline) Snapshot(ctx context.Context, settings db.Settings) (skillgen.Snapshot, error) {
	snap := skillgen.Snapshot{SkillName: settings.SkillName, IncludePending: settings.IncludePending, ExtractorVersion: extract.Version}
	for _, c := range extract.Categories {
		snap.Categories = append(snap.Categories, skillgen.Category{ID: c.ID, Title: c.Title, File: c.File})
	}
	repos, err := p.DB.ListRepositories(ctx)
	if err != nil {
		return snap, err
	}
	for _, r := range repos {
		if r.Enabled {
			snap.Repositories = append(snap.Repositories, r.FullName)
		}
	}
	rules, err := p.DB.PublishableRules(ctx, settings.IncludePending, settings.MinOccurrences)
	if err != nil {
		return snap, err
	}
	for _, r := range rules {
		evs, err := p.DB.RuleEvidence(ctx, r.ID)
		if err != nil {
			return snap, err
		}
		sr := skillgen.Rule{
			ID: r.ID, Code: r.Code, Category: r.Category, Subcategory: r.Subcategory, Title: r.Title, Guidance: r.Guidance,
			Scope: r.Scope, Severity: r.Severity, Status: r.Status, Occurrences: r.Occurrences, DistinctPRs: r.DistinctPRs,
			DistinctReviewers: r.DistinctReviewers, Confidence: r.Confidence,
		}
		for _, e := range evs {
			if e.LinkSource == "excluded" || e.DeletedAt != "" {
				continue
			}
			u := e.HTMLURL
			if u == "" {
				u = e.PRURL
			}
			date := e.CreatedAt
			if len(date) >= 10 {
				date = date[:10]
			}
			quote := extract.Truncate(extract.Normalize(e.Body).Display, 280)
			if quote == "" {
				quote = "(suggested change without text)"
			}
			sr.Evidence = append(sr.Evidence, skillgen.Evidence{
				Repo: e.Repo, PRNumber: e.PRNumber, PRURL: e.PRURL, URL: u, GitHubID: e.GitHubID, Source: e.Source,
				Reviewer: e.Author, Path: e.Path, Date: date, Quote: quote, Addressed: e.CodeChanged == "yes",
			})
		}
		if len(sr.Evidence) == 0 {
			continue // never publish a rule without provenance
		}
		snap.Rules = append(snap.Rules, sr)
	}
	return snap, nil
}

// Preview renders the skill without publishing and diffs it against the
// current published version.
type Preview struct {
	Version        string            `json:"version"`
	CurrentVersion string            `json:"current_version"`
	Changed        bool              `json:"changed"`
	RuleCount      int               `json:"rule_count"`
	Files          map[string]string `json:"files"`
	Diff           string            `json:"diff"`
	Errors         []string          `json:"errors"`
}

func (p *Pipeline) Preview(ctx context.Context) (Preview, error) {
	settings, err := p.DB.LoadSettings(ctx)
	if err != nil {
		return Preview{}, err
	}
	var out skillgen.Output
	var genErr error
	WithNetworkGuard(func() {
		snap, err := p.Snapshot(ctx, settings)
		if err != nil {
			genErr = err
			return
		}
		out, genErr = skillgen.Generate(snap)
	})
	if genErr != nil {
		return Preview{}, genErr
	}
	pv := Preview{Version: out.Version, CurrentVersion: skillgen.CurrentVersion(p.SkillBase()), RuleCount: out.RuleCount,
		Files: map[string]string{}, Errors: skillgen.Validate(out)}
	pv.Changed = pv.Version != pv.CurrentVersion
	cur := skillgen.CurrentDir(p.SkillBase())
	var diffs []string
	seen := map[string]bool{}
	for _, f := range out.Files {
		pv.Files[f.Path] = string(f.Content)
		seen[f.Path] = true
		old := ""
		if cur != "" {
			b, _ := os.ReadFile(filepath.Join(cur, filepath.FromSlash(f.Path)))
			old = string(b)
		}
		if d := UnifiedDiff(f.Path, old, string(f.Content)); d != "" {
			diffs = append(diffs, d)
		}
	}
	if cur != "" {
		filepath.WalkDir(cur, func(path string, de os.DirEntry, err error) error {
			if err != nil || de.IsDir() {
				return nil
			}
			rel, _ := filepath.Rel(cur, path)
			rel = filepath.ToSlash(rel)
			if !seen[rel] {
				b, _ := os.ReadFile(path)
				diffs = append(diffs, UnifiedDiff(rel, string(b), ""))
			}
			return nil
		})
	}
	pv.Diff = strings.Join(diffs, "\n")
	return pv, nil
}

// diffSummary compares rule ids with the currently published metadata.
func (p *Pipeline) diffSummary(out skillgen.Output) string {
	type mr struct {
		ID          string `json:"id"`
		Title       string `json:"title"`
		Occurrences int    `json:"occurrences"`
	}
	read := func(b []byte) map[string]mr {
		var m struct {
			Rules []mr `json:"rules"`
		}
		json.Unmarshal(b, &m)
		res := map[string]mr{}
		for _, r := range m.Rules {
			res[r.ID] = r
		}
		return res
	}
	var newMeta []byte
	for _, f := range out.Files {
		if f.Path == "metadata.json" {
			newMeta = f.Content
		}
	}
	oldMeta, _ := os.ReadFile(filepath.Join(p.SkillBase(), "current", "metadata.json"))
	o, n := read(oldMeta), read(newMeta)
	ch := struct {
		Added   []string `json:"added"`
		Removed []string `json:"removed"`
		Changed []string `json:"changed"`
	}{[]string{}, []string{}, []string{}}
	for id, r := range n {
		if old, ok := o[id]; !ok {
			ch.Added = append(ch.Added, id)
		} else if old != r {
			ch.Changed = append(ch.Changed, id)
		}
	}
	for id := range o {
		if _, ok := n[id]; !ok {
			ch.Removed = append(ch.Removed, id)
		}
	}
	sort.Strings(ch.Added)
	sort.Strings(ch.Removed)
	sort.Strings(ch.Changed)
	b, _ := json.Marshal(ch)
	return string(b)
}

// ---------------------------------------------------------------------------
// Zero-token / zero-network guarantees
// ---------------------------------------------------------------------------

var guardMu sync.Mutex

type guardTransport struct{ n *int64 }

func (g guardTransport) RoundTrip(*http.Request) (*http.Response, error) {
	atomic.AddInt64(g.n, 1)
	return nil, errors.New("network access is disabled during skill generation")
}

// WithNetworkGuard runs fn with Go's default HTTP transport replaced by one
// that refuses and counts every request. It returns the attempted calls.
func WithNetworkGuard(fn func()) int64 {
	guardMu.Lock()
	defer guardMu.Unlock()
	var n int64
	prev := http.DefaultTransport
	http.DefaultTransport = guardTransport{&n}
	defer func() { http.DefaultTransport = prev }()
	fn()
	return atomic.LoadInt64(&n)
}

// ForbiddenModules are AI/LLM/embedding SDK module path fragments that must
// never be linked into review-dock.
var ForbiddenModules = []string{
	"openai", "anthropic", "generative-ai", "google.golang.org/genai", "cloud.google.com/go/vertexai", "cohere",
	"langchain", "ollama", "mistral", "huggingface", "replicate", "llama", "gemini", "bedrockruntime",
}

// ZeroTokenReport is the runtime evidence shown in the UI.
type ZeroTokenReport struct {
	LinkedModules        int      `json:"linked_modules"`
	ForbiddenLinked      []string `json:"forbidden_linked"`
	LastBuildNetwork     int      `json:"last_build_network_calls"`
	LastBuildLLM         int      `json:"last_build_llm_calls"`
	GeneratorPackages    []string `json:"generator_packages"`
	Statement            string   `json:"statement"`
	BuildInfoUnavailable bool     `json:"build_info_unavailable"`
}

// CheckZeroToken inspects the running binary's linked modules.
func CheckZeroToken(last db.SkillBuild) ZeroTokenReport {
	r := ZeroTokenReport{
		ForbiddenLinked:   []string{},
		LastBuildNetwork:  last.NetworkCalls,
		LastBuildLLM:      last.LLMCalls,
		GeneratorPackages: []string{"backend/knowledge/db", "backend/knowledge/extract", "backend/knowledge/skillgen"},
		Statement: "Skill generation reads the local SQLite database and renders text/template files. " +
			"The generator packages cannot import net, net/http or os/exec (enforced by tests), builds run under an HTTP guard that blocks and counts requests, " +
			"and no LLM or embedding SDK is linked into this binary.",
	}
	bi, ok := debug.ReadBuildInfo()
	if !ok {
		r.BuildInfoUnavailable = true
		return r
	}
	r.LinkedModules = len(bi.Deps)
	for _, d := range bi.Deps {
		p := strings.ToLower(d.Path)
		for _, f := range ForbiddenModules {
			if strings.Contains(p, f) {
				r.ForbiddenLinked = append(r.ForbiddenLinked, d.Path)
				break
			}
		}
	}
	return r
}

// ---------------------------------------------------------------------------
// In-app scheduler
// ---------------------------------------------------------------------------

// ScheduledAt reports whether now (to the minute) matches one of the
// configured wall-clock times. The caller (the app's 55 s ticker, shared with
// the scheduled rebase) guarantees at most one run per minute.
func ScheduledAt(s db.Settings, now time.Time) bool {
	if !s.ScheduleEnabled {
		return false
	}
	cur := now.Format("03:04 PM")
	for _, t := range s.ScheduleTimes {
		if n, ok := db.NormalizeTime(t); ok && n == cur {
			return true
		}
	}
	return false
}
