package skillgen

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func snapshot() Snapshot {
	return Snapshot{
		SkillName:        "odoo-pr-review",
		Repositories:     []string{"odoo/odoo", "odoo/enterprise"},
		ExtractorVersion: "extract/test",
		Categories: []Category{
			{"performance", "Performance", "performance.md"},
			{"code-reuse", "Code Reuse", "code-reuse.md"},
			{"general", "General", "general.md"},
		},
		Rules: []Rule{
			{ID: 3, Code: "R-0003", Category: "code-reuse", Title: "Reuse existing helpers | utilities.", Scope: "general", Severity: "medium",
				Status: "approved", Occurrences: 2, DistinctPRs: 2, DistinctReviewers: 1, Confidence: 0.55,
				Evidence: []Evidence{
					{Repo: "odoo/odoo", PRNumber: 102, URL: "https://github.com/odoo/odoo/pull/102#discussion_r1101", Reviewer: "jpp", Date: "2026-01-21", Quote: "Please use the existing helper | here."},
					{Repo: "odoo/odoo", PRNumber: 103, URL: "https://github.com/odoo/odoo/pull/103#discussion_r1201", Reviewer: "jpp", Date: "2026-02-11", Quote: "We already have a helper."},
				}},
			{ID: 1, Code: "R-0001", Category: "performance", Subcategory: "Database queries", Title: "Avoid queries in loops.",
				Guidance: "Batch them.", Scope: "general", Severity: "high", Status: "approved", Occurrences: 3, DistinctPRs: 2,
				DistinctReviewers: 1, Confidence: 0.6, Evidence: []Evidence{
					{Repo: "odoo/odoo", PRNumber: 101, URL: "https://github.com/odoo/odoo/pull/101#discussion_r1001", Reviewer: "rco", Date: "2026-01-03", Path: "addons/sale/models/sale_order.py", Quote: "Don't use this method here.", Addressed: true},
				}},
			{ID: 9, Code: "R-0009", Category: "unknown-cat", Title: "Something else.", Scope: "context", Severity: "low", Status: "approved",
				Occurrences: 1, DistinctPRs: 1, Evidence: []Evidence{{Repo: "odoo/odoo", PRNumber: 104, URL: "https://github.com/odoo/odoo/pull/104", Reviewer: "x", Date: "2026-03-06", Quote: "q"}}},
		},
	}
}

func TestGenerateIsByteIdentical(t *testing.T) {
	a, err := Generate(snapshot())
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 5; i++ {
		s := snapshot()
		// Input order must not matter.
		s.Rules[0], s.Rules[2] = s.Rules[2], s.Rules[0]
		s.Repositories[0], s.Repositories[1] = s.Repositories[1], s.Repositories[0]
		b, err := Generate(s)
		if err != nil {
			t.Fatal(err)
		}
		if a.ContentHash != b.ContentHash || len(a.Files) != len(b.Files) {
			t.Fatalf("hash differs: %s vs %s", a.ContentHash, b.ContentHash)
		}
		for j := range a.Files {
			if a.Files[j].Path != b.Files[j].Path || !bytes.Equal(a.Files[j].Content, b.Files[j].Content) {
				t.Fatalf("file %s differs", a.Files[j].Path)
			}
		}
	}
	if errs := Validate(a); len(errs) > 0 {
		t.Fatalf("valid skill rejected: %v", errs)
	}
}

func TestGeneratedContent(t *testing.T) {
	out, _ := Generate(snapshot())
	files := map[string]string{}
	for _, f := range out.Files {
		files[f.Path] = string(f.Content)
	}
	for _, p := range []string{"SKILL.md", "metadata.json", "references/performance.md", "references/code-reuse.md",
		"references/general.md", "references/common-review-mistakes.md"} {
		if _, ok := files[p]; !ok {
			t.Errorf("missing %s", p)
		}
	}
	skill := files["SKILL.md"]
	for _, want := range []string{"name: odoo-pr-review", "## A. Review workflow", "## B. Historical reviewer rules",
		"## C. Areas covered", "## D. Before submitting", "## E. When implementing reviewer feedback",
		"**R-0001** [high · ×3 · 2 PRs]", out.Version} {
		if !strings.Contains(skill, want) {
			t.Errorf("SKILL.md missing %q", want)
		}
	}
	if strings.Contains(skill, "2026-") {
		t.Error("SKILL.md must not contain dates/timestamps that change between builds")
	}
	// Performance (category order 0) is listed before code reuse.
	if strings.Index(skill, "R-0001") > strings.Index(skill, "R-0003") {
		t.Error("rules not grouped in category order")
	}
	perf := files["references/performance.md"]
	if !strings.Contains(perf, "https://github.com/odoo/odoo/pull/101#discussion_r1001") || !strings.Contains(perf, "led to a code change") {
		t.Errorf("evidence/provenance missing:\n%s", perf)
	}
	// Unknown categories fall into the last (general) bucket.
	if !strings.Contains(files["references/general.md"], "R-0009") {
		t.Error("unknown category not routed to general")
	}
	if !strings.Contains(files["references/common-review-mistakes.md"], `\|`) {
		t.Error("pipe in table cell not escaped")
	}
}

func TestEmptyKnowledgeStillProducesValidSkill(t *testing.T) {
	out, err := Generate(Snapshot{SkillName: "odoo-pr-review"})
	if err != nil {
		t.Fatal(err)
	}
	if errs := Validate(out); len(errs) > 0 {
		t.Fatalf("empty skill invalid: %v", errs)
	}
	if out.RuleCount != 0 {
		t.Fatal("rules invented from nothing")
	}
}

func TestDuplicateTitlesAreCollapsed(t *testing.T) {
	s := snapshot()
	dup := s.Rules[1]
	dup.ID, dup.Code = 7, "R-0007"
	dup.Title = "Avoid queries in loops"
	dup.Evidence = []Evidence{{Repo: "odoo/odoo", PRNumber: 200, URL: "https://github.com/odoo/odoo/pull/200", Reviewer: "z", Date: "2026-05-01", Quote: "loop query"}}
	s.Rules = append(s.Rules, dup)
	out, _ := Generate(s)
	if out.RuleCount != 3 {
		t.Fatalf("duplicate not collapsed: %d rules", out.RuleCount)
	}
	if errs := Validate(out); len(errs) > 0 {
		t.Fatalf("invalid: %v", errs)
	}
	var skill string
	for _, f := range out.Files {
		if f.Path == "SKILL.md" {
			skill = string(f.Content)
		}
	}
	if !strings.Contains(skill, "R-0001 (also R-0007)") {
		t.Error("collapsed rule id not referenced")
	}
}

func TestValidateCatchesProblems(t *testing.T) {
	out, _ := Generate(snapshot())
	broken := out
	broken.Files = nil
	for _, f := range out.Files {
		if f.Path == "references/performance.md" {
			continue // dangling reference
		}
		c := f
		if f.Path == "SKILL.md" {
			c.Content = bytes.Replace(f.Content, []byte("name: odoo-pr-review"), []byte("name: Bad Name"), 1)
		}
		broken.Files = append(broken.Files, c)
	}
	errs := strings.Join(Validate(broken), "\n")
	for _, want := range []string{"invalid name", "references missing file references/performance.md", "R-0001: reference file missing"} {
		if !strings.Contains(errs, want) {
			t.Errorf("validation did not report %q; got:\n%s", want, errs)
		}
	}
}

func TestPublishIsAtomicVersionedAndIdempotent(t *testing.T) {
	base := t.TempDir()
	agents := t.TempDir()
	out1, _ := Generate(snapshot())
	res, err := Publish(base, "odoo-pr-review", out1, []string{agents}, 5)
	if err != nil {
		t.Fatal(err)
	}
	if !res.Changed || CurrentVersion(base) != out1.Version {
		t.Fatalf("first publish: %+v", res)
	}
	link := filepath.Join(agents, "odoo-pr-review")
	b, err := os.ReadFile(filepath.Join(link, "SKILL.md"))
	if err != nil || !strings.Contains(string(b), out1.Version) {
		t.Fatalf("agent link does not resolve to the skill: %v", err)
	}

	// Same content again: nothing changes, no new version directory.
	res, err = Publish(base, "odoo-pr-review", out1, []string{agents}, 5)
	if err != nil || res.Changed {
		t.Fatalf("idempotent publish changed something: %+v %v", res, err)
	}
	if n := countVersions(t, base); n != 1 {
		t.Fatalf("versions = %d", n)
	}

	// New knowledge: new version, link follows automatically, old kept.
	s := snapshot()
	s.Rules[0].Occurrences = 9
	out2, _ := Generate(s)
	if _, err := Publish(base, "odoo-pr-review", out2, []string{agents}, 5); err != nil {
		t.Fatal(err)
	}
	b, _ = os.ReadFile(filepath.Join(link, "metadata.json"))
	if !strings.Contains(string(b), out2.Version) || countVersions(t, base) != 2 {
		t.Fatal("agent link not on latest version or previous version not kept")
	}

	// Invalid output is refused and the previous version stays current.
	bad := out2
	bad.Files = bad.Files[:1]
	bad.Version = "deadbeef0000"
	if _, err := Publish(base, "odoo-pr-review", bad, nil, 5); err == nil {
		t.Fatal("invalid skill published")
	}
	if CurrentVersion(base) != out2.Version {
		t.Fatal("failed publish replaced the current version")
	}
}

func TestPublishNeverOverwritesForeignDirectory(t *testing.T) {
	base, agents := t.TempDir(), t.TempDir()
	mine := filepath.Join(agents, "odoo-pr-review")
	os.MkdirAll(mine, 0o755)
	os.WriteFile(filepath.Join(mine, "SKILL.md"), []byte("hand written"), 0o644)
	out, _ := Generate(snapshot())
	res, err := Publish(base, "odoo-pr-review", out, []string{agents}, 5)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Warnings) != 1 || len(res.Targets) != 0 {
		t.Fatalf("expected a warning and no target: %+v", res)
	}
	b, _ := os.ReadFile(filepath.Join(mine, "SKILL.md"))
	if string(b) != "hand written" {
		t.Fatal("user directory overwritten")
	}
}

func TestPruneKeepsCurrent(t *testing.T) {
	base := t.TempDir()
	var last Output
	for i := 0; i < 5; i++ {
		s := snapshot()
		s.Rules[0].Occurrences = 10 + i
		last, _ = Generate(s)
		if _, err := Publish(base, "odoo-pr-review", last, nil, 2); err != nil {
			t.Fatal(err)
		}
	}
	if n := countVersions(t, base); n != 2 {
		t.Fatalf("versions kept = %d, want 2", n)
	}
	if CurrentVersion(base) != last.Version {
		t.Fatal("current pruned")
	}
}

func countVersions(t *testing.T, base string) int {
	entries, err := os.ReadDir(filepath.Join(base, "versions"))
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
