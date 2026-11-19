package pipeline

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gofrs/flock"

	"review-dock/backend/knowledge/db"
	"review-dock/backend/knowledge/ghsync"
	"review-dock/backend/knowledge/kbtest"
	"review-dock/backend/knowledge/skillgen"
)

func newPipeline(t *testing.T) (*Pipeline, *kbtest.Fake, string) {
	t.Helper()
	dir := t.TempDir()
	p, err := Open(dir, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { p.DB.Close() })
	f := kbtest.New("odoo/odoo")
	kbtest.OdooFixture(f)
	p.Runner = f
	agents := filepath.Join(dir, "agents")
	ctx := context.Background()
	s, _ := p.DB.LoadSettings(ctx)
	s.PublishTargets = []string{agents}
	if err := p.DB.SaveSettings(ctx, s); err != nil {
		t.Fatal(err)
	}
	if _, err := p.DB.EnsureRepository(ctx, "odoo", "odoo"); err != nil {
		t.Fatal(err)
	}
	return p, f, agents
}

func ruleByKey(t *testing.T, p *Pipeline, key string) db.Rule {
	t.Helper()
	rules, err := p.DB.ListRules(context.Background(), db.RuleFilter{Limit: 2000})
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range rules {
		if r.Key == key {
			return r
		}
	}
	t.Fatalf("rule %s not found", key)
	return db.Rule{}
}

func readSkill(t *testing.T, dir string) map[string]string {
	t.Helper()
	out := map[string]string{}
	if resolved, err := filepath.EvalSymlinks(dir); err == nil {
		dir = resolved
	}
	filepath.WalkDir(dir, func(p string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		b, _ := os.ReadFile(p)
		rel, _ := filepath.Rel(dir, p)
		out[rel] = string(b)
		return nil
	})
	return out
}

func TestEndToEndRunPublishesSkill(t *testing.T) {
	p, _, agents := newPipeline(t)
	ctx := context.Background()
	res, err := p.Run(ctx, Options{Trigger: "cli"})
	if err != nil {
		t.Fatal(err)
	}
	if res.Run.Status != "ok" || res.Run.PRsSynced != 4 || res.Run.CommentsUpserted != 18 {
		t.Fatalf("run %+v", res.Run)
	}
	// Expected knowledge from the fixture.
	q := ruleByKey(t, p, "sig:perf-query-in-loop")
	if q.Occurrences != 2 || q.DistinctPRs != 2 || q.Scope != "general" || q.Severity != "high" {
		t.Errorf("query-in-loop stats %+v", q)
	}
	reuse := ruleByKey(t, p, "sig:reuse-existing-helper")
	if reuse.Occurrences != 2 || reuse.Category != "code-reuse" {
		t.Errorf("reuse stats %+v", reuse)
	}
	tests := ruleByKey(t, p, "sig:test-coverage")
	if tests.Occurrences != 3 || tests.DistinctReviewers != 3 {
		t.Errorf("test-coverage stats %+v", tests)
	}
	cand := ruleByKey(t, p, "cand:c"+itoa(t, p, 2001))
	if cand.Origin != "candidate" || cand.Occurrences != 2 {
		t.Errorf("candidate rule %+v", cand)
	}
	// The superseded sudo comment produced no rule.
	if rules, _ := p.DB.ListRules(ctx, db.RuleFilter{Query: "sudo"}); len(rules) != 0 {
		t.Errorf("superseded feedback became a rule: %+v", rules)
	}
	// Nothing is approved yet → skill published with workflow only.
	if res.Build == nil || res.Build.Build.Status != "published" || res.Build.Build.RuleCount != 0 {
		t.Fatalf("build %+v", res.Build)
	}
	skill := readSkill(t, filepath.Join(agents, "odoo-pr-review"))
	if !strings.Contains(skill["SKILL.md"], "No approved rules yet") {
		t.Error("unapproved rules leaked into the skill")
	}
}

// itoa returns the local comment id for a GitHub comment id, as text.
func itoa(t *testing.T, p *Pipeline, githubID int64) string {
	var id int64
	if err := p.DB.QueryRow(`SELECT id FROM review_comments WHERE github_id=?`, githubID).Scan(&id); err != nil {
		t.Fatal(err)
	}
	return strconv.FormatInt(id, 10)
}

func TestApprovalRejectionEditsMergesSurviveReExtraction(t *testing.T) {
	p, _, agents := newPipeline(t)
	ctx := context.Background()
	if _, err := p.Run(ctx, Options{}); err != nil {
		t.Fatal(err)
	}
	q := ruleByKey(t, p, "sig:perf-query-in-loop")
	reuse := ruleByKey(t, p, "sig:reuse-existing-helper")
	tests := ruleByKey(t, p, "sig:test-coverage")
	naming := ruleByKey(t, p, "sig:style-naming")

	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	must(p.DB.SetRuleStatus(ctx, q.ID, db.StatusApproved, ""))
	must(p.DB.SetRuleStatus(ctx, tests.ID, db.StatusApproved, ""))
	must(p.DB.SetRuleStatus(ctx, naming.ID, db.StatusRejected, "too generic"))
	must(p.DB.EditRule(ctx, reuse.ID, db.RuleEdit{Title: "Grep for an existing helper before writing a new one.", Category: "code-reuse", Severity: "medium"}))
	must(p.DB.SetRuleStatus(ctx, reuse.ID, db.StatusApproved, ""))

	// Exclude one evidence link of the test rule.
	ev, _ := p.DB.RuleEvidence(ctx, tests.ID)
	must(p.DB.SetEvidenceLink(ctx, tests.ID, ev[0].ID, "excluded"))

	// Re-run the deterministic extraction twice: user decisions persist.
	for i := 0; i < 2; i++ {
		if _, err := p.Run(ctx, Options{SkipSync: true}); err != nil {
			t.Fatal(err)
		}
	}
	if r := ruleByKey(t, p, "sig:perf-query-in-loop"); r.Status != db.StatusApproved {
		t.Errorf("approval lost: %s", r.Status)
	}
	if r := ruleByKey(t, p, "sig:style-naming"); r.Status != db.StatusRejected || r.Note != "too generic" {
		t.Errorf("rejection lost: %+v", r)
	}
	if r := ruleByKey(t, p, "sig:reuse-existing-helper"); r.Title != "Grep for an existing helper before writing a new one." || !r.Edited {
		t.Errorf("edit overwritten: %+v", r)
	}
	if r := ruleByKey(t, p, "sig:test-coverage"); r.Occurrences != 2 {
		t.Errorf("excluded evidence came back: occurrences %d", r.Occurrences)
	}

	// Merge the candidate rule into the test rule: its evidence counts there.
	cand := ruleByKey(t, p, "cand:c"+itoa(t, p, 2001))
	must(p.DB.MergeRules(ctx, tests.ID, []int64{cand.ID}))
	if _, err := p.Run(ctx, Options{SkipSync: true}); err != nil {
		t.Fatal(err)
	}
	if r := ruleByKey(t, p, "sig:test-coverage"); r.Occurrences != 4 {
		t.Errorf("merged evidence not counted: %d", r.Occurrences)
	}
	if r := ruleByKey(t, p, cand.Key); r.Status != db.StatusMerged {
		t.Errorf("merged rule status %s", r.Status)
	}

	skill := readSkill(t, filepath.Join(agents, "odoo-pr-review"))
	md := skill["SKILL.md"]
	for _, want := range []string{q.Code, reuse.Code, tests.Code, "Grep for an existing helper"} {
		if !strings.Contains(md, want) {
			t.Errorf("approved rule %s missing from SKILL.md", want)
		}
	}
	for _, unwanted := range []string{naming.Code, cand.Code} {
		if strings.Contains(md, "**"+unwanted) {
			t.Errorf("rejected/merged rule %s published", unwanted)
		}
	}
	if !strings.Contains(skill[filepath.Join("references", "performance.md")], "https://github.com/odoo/odoo/pull/101#discussion_r1001") {
		t.Error("evidence link missing")
	}
	// Superseded rules are excluded.
	must(p.DB.SetRuleStatus(ctx, q.ID, db.StatusSuperseded, "obsolete since 18.0"))
	if _, err := p.Build(ctx, true); err != nil {
		t.Fatal(err)
	}
	if md := readSkill(t, filepath.Join(agents, "odoo-pr-review"))["SKILL.md"]; strings.Contains(md, "**"+q.Code) {
		t.Error("superseded rule still published")
	}
}

// failingRunner fails (and counts) every GitHub call.
type failingRunner struct{ calls atomic.Int64 }

func (f *failingRunner) API(context.Context, ...string) ([]byte, error) {
	f.calls.Add(1)
	return nil, errors.New("network disabled by test")
}

// TestZeroTokenSkillGeneration is the hard acceptance criterion: with every
// LLM/API/network integration disabled or trapped, extraction and skill
// generation still succeed, make zero calls, and are byte-identical.
func TestZeroTokenSkillGeneration(t *testing.T) {
	p, _, agents := newPipeline(t)
	ctx := context.Background()
	// Populate the database once (this is the only networked stage).
	if _, err := p.Run(ctx, Options{NoBuild: true}); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"sig:perf-query-in-loop", "sig:reuse-existing-helper", "sig:test-coverage"} {
		if err := p.DB.SetRuleStatus(ctx, ruleByKey(t, p, key).ID, db.StatusApproved, ""); err != nil {
			t.Fatal(err)
		}
	}

	// --- Disable everything an LLM/AI integration could use. ---
	var trapped atomic.Int64
	trap := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		trapped.Add(1)
		http.Error(w, "AI/network access forbidden during skill generation", http.StatusForbidden)
	}))
	defer trap.Close()
	for _, k := range []string{"HTTP_PROXY", "HTTPS_PROXY", "ALL_PROXY", "http_proxy", "https_proxy",
		"OPENAI_BASE_URL", "OPENAI_API_BASE", "ANTHROPIC_BASE_URL", "GOOGLE_API_BASE", "OLLAMA_HOST"} {
		t.Setenv(k, trap.URL)
	}
	for _, k := range []string{"OPENAI_API_KEY", "ANTHROPIC_API_KEY", "GEMINI_API_KEY", "GOOGLE_API_KEY"} {
		t.Setenv(k, "test-key-must-never-be-used")
	}
	t.Setenv("NO_PROXY", "")
	t.Setenv("PATH", t.TempDir()) // no gh, no curl, no python: any subprocess fails
	runner := &failingRunner{}
	p.Runner = runner

	// Extraction + generation with sync skipped.
	res, err := p.Run(ctx, Options{SkipSync: true, Trigger: "cli"})
	if err != nil {
		t.Fatalf("zero-token run failed: %v", err)
	}
	b := res.Build.Build
	if b.Status != "published" || b.RuleCount != 3 {
		t.Fatalf("skill not generated: %+v", b)
	}
	if b.NetworkCalls != 0 || b.LLMCalls != 0 {
		t.Fatalf("build recorded network=%d llm=%d", b.NetworkCalls, b.LLMCalls)
	}
	if n := runner.calls.Load(); n != 0 {
		t.Fatalf("GitHub/gh invoked %d times during generation", n)
	}
	if n := trapped.Load(); n != 0 {
		t.Fatalf("%d HTTP request(s) reached the AI/network trap", n)
	}
	first := readSkill(t, skillgen.CurrentDir(p.SkillBase()))
	if len(first) < 5 || !strings.Contains(first["SKILL.md"], "R-") {
		t.Fatalf("incomplete skill: %v", keys(first))
	}

	// Same database → byte-identical output, and no new version.
	versionsBefore := countDirs(t, filepath.Join(p.SkillBase(), "versions"))
	br, err := p.Build(ctx, true)
	if err != nil {
		t.Fatal(err)
	}
	if br.Build.Status != "unchanged" || br.Build.ContentHash != b.ContentHash {
		t.Fatalf("rebuild of unchanged knowledge: %+v", br.Build)
	}
	if countDirs(t, filepath.Join(p.SkillBase(), "versions")) != versionsBefore {
		t.Fatal("unchanged rebuild created a new version")
	}
	second := readSkill(t, skillgen.CurrentDir(p.SkillBase()))
	if len(first) != len(second) {
		t.Fatal("file set differs")
	}
	for k, v := range first {
		if second[k] != v {
			t.Fatalf("%s not byte-identical", k)
		}
	}
	// Preview from the same DB matches the published files exactly.
	pv, err := p.Preview(ctx)
	if err != nil || pv.Changed || pv.Diff != "" {
		t.Fatalf("preview differs from published skill: changed=%v err=%v\n%s", pv.Changed, err, pv.Diff)
	}
	// The agent-facing link resolves to the same content.
	if readSkill(t, filepath.Join(agents, "odoo-pr-review"))["SKILL.md"] != first["SKILL.md"] {
		t.Fatal("agent link does not serve the current skill")
	}
	if runner.calls.Load() != 0 || trapped.Load() != 0 {
		t.Fatal("network used during rebuild/preview")
	}

	rep := CheckZeroToken(br.Build)
	if len(rep.ForbiddenLinked) != 0 {
		t.Fatalf("AI SDK linked into the binary: %v", rep.ForbiddenLinked)
	}
}

func TestNetworkGuardBlocksHTTP(t *testing.T) {
	var resp *http.Response
	var err error
	n := WithNetworkGuard(func() {
		resp, err = http.Get("http://127.0.0.1:1/") // would be a real call without the guard
	})
	if n != 1 || err == nil || resp != nil {
		t.Fatalf("guard did not block/count: n=%d err=%v", n, err)
	}
}

func TestConcurrentRunsAreRejected(t *testing.T) {
	p, f, _ := newPipeline(t)
	ctx := context.Background()

	// Another process holds the lock.
	other := flock.New(p.LockPath())
	if ok, err := other.TryLock(); !ok || err != nil {
		t.Fatal("could not take lock", err)
	}
	if _, err := p.Run(ctx, Options{}); !errors.Is(err, ErrBusy) {
		t.Fatalf("expected ErrBusy, got %v", err)
	}
	if _, err := p.Build(ctx, true); !errors.Is(err, ErrBusy) {
		t.Fatalf("build: expected ErrBusy, got %v", err)
	}
	other.Unlock()

	// Two goroutines in the same process: exactly one runs.
	block := make(chan struct{})
	started := make(chan struct{})
	var once sync.Once
	p.Runner = runnerFunc(func(ctx context.Context, args ...string) ([]byte, error) {
		once.Do(func() { close(started) })
		<-block
		return f.API(ctx, args...)
	})
	var wg sync.WaitGroup
	var firstErr error
	wg.Add(1)
	go func() { defer wg.Done(); _, firstErr = p.Run(ctx, Options{}) }()
	<-started
	if _, err := p.Run(ctx, Options{}); !errors.Is(err, ErrBusy) {
		t.Fatalf("second concurrent run: %v", err)
	}
	close(block)
	wg.Wait()
	if firstErr != nil {
		t.Fatal(firstErr)
	}
}

type runnerFunc func(ctx context.Context, args ...string) ([]byte, error)

func (r runnerFunc) API(ctx context.Context, args ...string) ([]byte, error) { return r(ctx, args...) }

func TestRecoveryAfterFailureAndInterruption(t *testing.T) {
	p, f, _ := newPipeline(t)
	ctx := context.Background()

	f.FailAlways["pulls/103/comments"] = "HTTP 404: Not Found"
	res, err := p.Run(ctx, Options{})
	if err != nil {
		t.Fatal(err)
	}
	if res.Run.Status != "error" || res.Run.ReposFailed != 1 || !strings.Contains(res.Run.Error, "PR #103") {
		t.Fatalf("failed run not reported: %+v", res.Run)
	}
	repo, _ := p.DB.GetRepositoryByName(ctx, "odoo/odoo")
	if repo.SyncStatus != "error" || repo.ConsecutiveFailures != 1 || repo.CheckpointUpdatedAt != "2026-02-03T10:00:00Z" {
		t.Fatalf("repo state after failure %+v", repo)
	}
	// Knowledge from the PRs that did sync is still extracted and a valid skill is built.
	if res.Build == nil || res.Build.Build.Status == "failed" {
		t.Fatalf("build after partial sync: %+v", res.Build)
	}

	// Simulate a crash: a run left 'running'.
	if _, err := p.DB.StartRun(ctx, "schedule"); err != nil {
		t.Fatal(err)
	}

	delete(f.FailAlways, "pulls/103/comments")
	res, err = p.Run(ctx, Options{Trigger: "retry"})
	if err != nil {
		t.Fatal(err)
	}
	if res.Run.Status != "ok" || res.Run.PRsSynced != 2 {
		t.Fatalf("retry run %+v", res.Run)
	}
	runs, _ := p.DB.ListRuns(ctx, 10)
	statuses := []string{}
	for _, r := range runs {
		statuses = append(statuses, r.Status)
	}
	if strings.Join(statuses, ",") != "ok,interrupted,error" {
		t.Fatalf("run history = %v", statuses)
	}
	repo, _ = p.DB.GetRepositoryByName(ctx, "odoo/odoo")
	if repo.SyncStatus != "ok" || repo.ConsecutiveFailures != 0 || !repo.FullImportDone {
		t.Fatalf("repo after recovery %+v", repo)
	}
}

func TestRateLimitedRunMarksRemainingRepos(t *testing.T) {
	p, f, _ := newPipeline(t)
	ctx := context.Background()
	p.DB.EnsureRepository(ctx, "odoo", "enterprise")
	f.CoreRemaining = 1
	res, err := p.Run(ctx, Options{NoBuild: true})
	if err != nil {
		t.Fatal(err)
	}
	if res.Run.Status != "error" || res.Run.ReposFailed != 2 {
		t.Fatalf("run %+v", res.Run)
	}
	for _, name := range []string{"odoo/odoo", "odoo/enterprise"} {
		r, _ := p.DB.GetRepositoryByName(ctx, name)
		if r.SyncStatus != "rate_limited" {
			t.Errorf("%s status %s", name, r.SyncStatus)
		}
	}
}

func TestBuildFailureKeepsPreviousSkill(t *testing.T) {
	p, _, _ := newPipeline(t)
	ctx := context.Background()
	if _, err := p.Run(ctx, Options{}); err != nil {
		t.Fatal(err)
	}
	prev := skillgen.CurrentVersion(p.SkillBase())
	// An invalid skill name makes validation fail.
	s, _ := p.DB.LoadSettings(ctx)
	s.SkillName = "Invalid Name!"
	p.DB.SaveSettings(ctx, s)
	if _, err := p.Build(ctx, true); err == nil {
		t.Fatal("invalid skill built")
	}
	if skillgen.CurrentVersion(p.SkillBase()) != prev {
		t.Fatal("previous valid skill replaced")
	}
	last, _ := p.DB.LastBuild(ctx, "")
	if last.Status != "failed" || last.Error == "" {
		t.Fatalf("failure not recorded: %+v", last)
	}
}

func TestSchedulerDue(t *testing.T) {
	p, _, _ := newPipeline(t)
	ctx := context.Background()
	now := time.Now().UTC()
	if due, _ := p.Due(ctx, now); !due {
		t.Fatal("first run should be due")
	}
	p.DB.Now = func() time.Time { return now }
	if _, err := p.Run(ctx, Options{Trigger: "schedule", NoBuild: true}); err != nil {
		t.Fatal(err)
	}
	if due, _ := p.Due(ctx, now.Add(time.Minute)); due {
		t.Fatal("due right after a run")
	}
	if due, _ := p.Due(ctx, now.Add(361*time.Minute)); !due {
		t.Fatal("not due after the interval")
	}
	s, _ := p.DB.LoadSettings(ctx)
	s.ScheduleEnabled = false
	p.DB.SaveSettings(ctx, s)
	if due, _ := p.Due(ctx, now.Add(24*time.Hour)); due {
		t.Fatal("due while disabled")
	}
}

func TestUnifiedDiff(t *testing.T) {
	if UnifiedDiff("a", "x\n", "x\n") != "" {
		t.Fatal("equal inputs produced a diff")
	}
	d := UnifiedDiff("f.md", "a\nb\nc\n", "a\nB\nc\nd\n")
	for _, want := range []string{"--- a/f.md", "-b", "+B", "+d"} {
		if !strings.Contains(d, want) {
			t.Errorf("diff missing %q:\n%s", want, d)
		}
	}
}

func TestRedactedErrorsInRunRecords(t *testing.T) {
	p, f, _ := newPipeline(t)
	f.FailAlways["search/issues"] = "HTTP 401: Bad credentials ghp_abcdefghijklmnopqrstuvwxyz0123456789"
	res, _ := p.Run(context.Background(), Options{NoBuild: true})
	if strings.Contains(res.Run.Error, "ghp_") {
		t.Fatalf("token stored in run record: %s", res.Run.Error)
	}
	repo, _ := p.DB.GetRepositoryByName(context.Background(), "odoo/odoo")
	if strings.Contains(repo.LastError, "ghp_") || !strings.Contains(repo.LastError, "[REDACTED]") {
		t.Fatalf("repo error: %s", repo.LastError)
	}
	_ = ghsync.Redact
}

func keys(m map[string]string) []string {
	var out []string
	for k := range m {
		out = append(out, k)
	}
	return out
}

func countDirs(t *testing.T, dir string) int {
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	n := 0
	for _, e := range entries {
		if e.IsDir() && !strings.HasPrefix(e.Name(), ".") {
			n++
		}
	}
	return n
}
