package extract

import (
	"math/rand"
	"reflect"
	"strings"
	"testing"
)

func TestNormalizeStripsQuotesCodeAndLinks(t *testing.T) {
	n := Normalize("> quoted text from author\nPlease use `search_count` here, see [docs](https://x.y/z).\n```python\nx = 1\n```\n```suggestion\ncount = len(x)\n```")
	if strings.Contains(n.Text, "quoted") {
		t.Errorf("quote not removed: %q", n.Text)
	}
	if !strings.Contains(n.Text, "search_count") || !strings.Contains(n.Text, "docs") {
		t.Errorf("inline code / link text lost: %q", n.Text)
	}
	if strings.Contains(n.Text, "https://") || strings.Contains(n.Text, "x = 1") {
		t.Errorf("url or code block kept: %q", n.Text)
	}
	if !n.HasSuggestion || !n.HasCode {
		t.Errorf("flags: suggestion=%v code=%v", n.HasSuggestion, n.HasCode)
	}
}

func TestClassifyKind(t *testing.T) {
	cases := []struct {
		body, state, want string
	}{
		{"Don't use this method here because it triggers an unnecessary database query for every record.", "", KindRequirement},
		{"Please use the existing helper instead of duplicating this implementation.", "", KindRequirement},
		{"nit: maybe rename this variable", "", KindSuggestion},
		{"Why is this needed?", "", KindQuestion},
		{"LGTM", "", KindNoise},
		{"Thanks!", "", KindNoise},
		{"@robodoo r+", "", KindNoise},
		{"Nevermind, I misread the diff", "", KindSuperseded},
		{"```suggestion\nfoo()\n```", "", KindRequirement},
		{"", "APPROVED", KindNoise},
		{"The ordering of these fields matters here for the report", "", KindUnclear},
		{"This changes the ordering of the report lines", "CHANGES_REQUESTED", KindRequirement},
		{"Why do you use read here?", "", KindQuestion},
		{"Could you add a regression test? Thanks", "", KindRequirement},
		{"This does a search for each line in the loop, it will be slow with many records.", "", KindRequirement},
		{"Is this called for every record? I wonder", "", KindQuestion},
	}
	for _, c := range cases {
		if got := ClassifyKind(Normalize(c.body), c.state); got != c.want {
			t.Errorf("ClassifyKind(%q) = %s, want %s", c.body, got, c.want)
		}
	}
}

func TestBriefExamplesAreCategorised(t *testing.T) {
	res := Analyze([]Comment{
		{ID: 1, GitHubID: 1, Source: "inline", PRID: 1, Author: "rev", ThreadRootID: 1, CreatedAt: "2026-01-01T00:00:00Z",
			Path: "addons/sale/models/sale_order.py",
			Body: "Don't use this method here because it triggers an unnecessary database query for every record."},
		{ID: 2, GitHubID: 2, Source: "inline", PRID: 2, Author: "rev", ThreadRootID: 2, CreatedAt: "2026-01-02T00:00:00Z",
			Path: "addons/sale/models/sale_order.py",
			Body: "Please use the existing helper instead of duplicating this implementation."},
	}, Options{})

	if res.Analyses[0].Category != "performance" || res.Analyses[0].Subcategory != "Database queries" {
		t.Errorf("example 1: got %s/%s, want performance/Database queries", res.Analyses[0].Category, res.Analyses[0].Subcategory)
	}
	if res.Analyses[1].Category != "code-reuse" {
		t.Errorf("example 2: got %s, want code-reuse", res.Analyses[1].Category)
	}
	keys := map[string]ProposedRule{}
	for _, r := range res.Rules {
		keys[r.Key] = r
	}
	q, ok := keys["sig:perf-query-in-loop"]
	if !ok || len(q.Evidence) != 1 || q.Evidence[0].CommentID != 1 {
		t.Fatalf("query-in-loop rule missing or wrong evidence: %+v", q)
	}
	if _, ok := keys["sig:reuse-existing-helper"]; !ok {
		t.Fatalf("reuse rule missing; got %v", keys)
	}
}

func TestOneCommentYieldsAtMostOneRulePerCategory(t *testing.T) {
	// Matches both perf-query-in-loop and perf-redundant-work: only the best stays.
	m := MatchSignals(Normalize("Don't use this method here because it triggers an unnecessary database query for every record.").Text, "")
	if len(m) != 1 || m[0].ID != "perf-query-in-loop" {
		t.Fatalf("matches = %+v", m)
	}
	// A weak catch-all still applies on its own.
	m = MatchSignals(Normalize("This is unnecessary, please simplify").Text, "")
	if len(m) != 1 || m[0].ID != "style-simplify" {
		t.Fatalf("weak-only matches = %+v", m)
	}
	// Different areas may both apply.
	m = MatchSignals(Normalize("Please avoid this sudo and add a test for it").Text, "")
	if len(m) != 2 {
		t.Fatalf("cross-category matches = %+v", m)
	}
}

func TestBotsAuthorsAndNoiseNeverBecomeEvidence(t *testing.T) {
	res := Analyze([]Comment{
		{ID: 1, Source: "issue", PRID: 1, Author: "robodoo", Body: "Please avoid sudo here"},
		{ID: 2, Source: "issue", PRID: 1, Author: "me", IsPRAuthor: true, Body: "Please avoid sudo here"},
		{ID: 3, Source: "issue", PRID: 1, Author: "github-actions[bot]", Body: "Please avoid sudo here"},
		{ID: 4, Source: "issue", PRID: 1, Author: "rev", Body: "LGTM"},
	}, Options{IgnoredAuthors: []string{"robodoo"}})
	if len(res.Rules) != 0 {
		t.Fatalf("expected no rules, got %+v", res.Rules)
	}
	kinds := []string{res.Analyses[0].Kind, res.Analyses[1].Kind, res.Analyses[2].Kind, res.Analyses[3].Kind}
	want := []string{KindBot, KindSelf, KindBot, KindNoise}
	if !reflect.DeepEqual(kinds, want) {
		t.Errorf("kinds = %v, want %v", kinds, want)
	}
}

func TestThreadSupersessionWhenReviewerConcedes(t *testing.T) {
	res := Analyze([]Comment{
		{ID: 1, GitHubID: 10, Source: "inline", PRID: 1, Author: "rev", ThreadRootID: 10, CreatedAt: "1", Body: "Please remove this sudo()."},
		{ID: 2, GitHubID: 11, Source: "inline", PRID: 1, Author: "me", IsPRAuthor: true, ThreadRootID: 10, InReplyToID: 10, CreatedAt: "2",
			Body: "It is needed because the portal user has no access."},
		{ID: 3, GitHubID: 12, Source: "inline", PRID: 1, Author: "rev", ThreadRootID: 10, InReplyToID: 10, CreatedAt: "3", Body: "Ok, fair enough."},
	}, Options{})
	if res.Analyses[0].Kind != KindSuperseded {
		t.Fatalf("root kind = %s, want superseded", res.Analyses[0].Kind)
	}
	if len(res.Rules) != 0 {
		t.Fatalf("superseded feedback must not create rules: %+v", res.Rules)
	}

	// Same thread, but the author fixed it: the requirement stands.
	res = Analyze([]Comment{
		{ID: 1, GitHubID: 10, Source: "inline", PRID: 1, Author: "rev", ThreadRootID: 10, CreatedAt: "1", Body: "Please remove this sudo()."},
		{ID: 2, GitHubID: 11, Source: "inline", PRID: 1, Author: "me", IsPRAuthor: true, ThreadRootID: 10, InReplyToID: 10, CreatedAt: "2", Body: "Done"},
		{ID: 3, GitHubID: 12, Source: "inline", PRID: 1, Author: "rev", ThreadRootID: 10, InReplyToID: 10, CreatedAt: "3", Body: "Ok thanks"},
	}, Options{})
	if res.Analyses[0].Kind != KindRequirement {
		t.Fatalf("root kind = %s, want requirement", res.Analyses[0].Kind)
	}
}

func TestCandidateClusteringNeedsTwoPRs(t *testing.T) {
	a := Comment{ID: 1, Source: "issue", PRID: 1, Author: "r1", CreatedAt: "1", Body: "Please target the 17.0 branch, saas versions are frozen for this kind of change."}
	b := Comment{ID: 2, Source: "issue", PRID: 2, Author: "r2", CreatedAt: "2", Body: "Please target the 17.0 branch instead, saas versions are frozen."}
	res := Analyze([]Comment{a}, Options{})
	if len(res.Rules) != 0 {
		t.Fatalf("single-PR feedback outside the taxonomy must not become a rule: %+v", res.Rules)
	}
	res = Analyze([]Comment{a, b}, Options{})
	if len(res.Rules) != 1 || res.Rules[0].Key != "cand:c1" || len(res.Rules[0].Evidence) != 2 {
		t.Fatalf("expected one candidate rule anchored on comment 1 with 2 evidence: %+v", res.Rules)
	}
	if !strings.HasPrefix(res.Rules[0].Title, "Please target the 17.0 branch") {
		t.Errorf("candidate title must quote the reviewer: %q", res.Rules[0].Title)
	}
	// An anchor keeps its key even when an older similar comment appears.
	older := Comment{ID: 3, Source: "issue", PRID: 3, Author: "r3", CreatedAt: "0", Body: "Please target the 17.0 branch, saas versions are frozen now."}
	res = Analyze([]Comment{a, b, older}, Options{Anchors: []int64{1}})
	if len(res.Rules) != 1 || res.Rules[0].Key != "cand:c1" || len(res.Rules[0].Evidence) != 3 {
		t.Fatalf("anchor not stable: %+v", res.Rules)
	}
}

func TestOneThreadCountsOncePerRule(t *testing.T) {
	res := Analyze([]Comment{
		{ID: 1, GitHubID: 10, Source: "inline", PRID: 1, Author: "rev", ThreadRootID: 10, CreatedAt: "1", Body: "Please don't use sudo here"},
		{ID: 2, GitHubID: 11, Source: "inline", PRID: 1, Author: "rev", ThreadRootID: 10, InReplyToID: 10, CreatedAt: "2", Body: "Also, avoid sudo in the other method"},
	}, Options{})
	for _, r := range res.Rules {
		if r.Key == "sig:sec-sudo" && len(r.Evidence) != 1 {
			t.Fatalf("thread counted %d times", len(r.Evidence))
		}
	}
}

func TestAnalyzeIsDeterministicAndOrderIndependent(t *testing.T) {
	var cs []Comment
	bodies := []string{
		"Please avoid a query in the loop, use read_group instead",
		"You should call super() in this write override",
		"Please add a test for this fix",
		"nit: maybe rename this variable to something clearer",
		"Please target the 17.0 branch, saas versions are frozen.",
		"Please target the 17.0 branch instead, saas versions are frozen for fixes.",
		"Remove this sudo please",
		"Why?",
	}
	for i, b := range bodies {
		cs = append(cs, Comment{ID: int64(i + 1), GitHubID: int64(100 + i), Source: "issue", PRID: int64(i%3 + 1), Author: "rev",
			CreatedAt: "2026-01-0" + string(rune('1'+i)) + "T00:00:00Z", Body: b})
	}
	want := Analyze(cs, Options{})
	r := rand.New(rand.NewSource(42))
	for i := 0; i < 20; i++ {
		shuffled := append([]Comment(nil), cs...)
		r.Shuffle(len(shuffled), func(a, b int) { shuffled[a], shuffled[b] = shuffled[b], shuffled[a] })
		if got := Analyze(shuffled, Options{}); !reflect.DeepEqual(got, want) {
			t.Fatalf("non-deterministic result on shuffle %d", i)
		}
	}
}

func TestPathCategories(t *testing.T) {
	cases := map[string]string{
		"addons/point_of_sale/static/src/app/x.js": "pos",
		"addons/web/static/src/core/l10n.js":       "frontend-owl",
		"addons/sale/tests/test_sale.py":           "testing",
		"addons/sale/security/ir.model.access.csv": "security",
		"addons/sale/views/sale_views.xml":         "views-xml",
		"addons/sale/models/sale.py":               "backend-python",
	}
	for p, want := range cases {
		if got, _ := pathCategory(p); got != want {
			t.Errorf("pathCategory(%s) = %s, want %s", p, got, want)
		}
	}
}

func TestConfidence(t *testing.T) {
	if Confidence(0, 0, 0, 0) != 0 {
		t.Error("no evidence must be 0")
	}
	if c := Confidence(1, 1, 1, 0); c != 0.4 {
		t.Errorf("single PR requirement = %v, want 0.4", c)
	}
	if c := Confidence(10, 5, 1, 1); c != 1 {
		t.Errorf("strong evidence = %v, want 1", c)
	}
}

func TestEverySignalIsWellFormed(t *testing.T) {
	ids := map[string]bool{}
	for _, s := range Signals {
		if ids[s.ID] {
			t.Errorf("duplicate signal id %s", s.ID)
		}
		ids[s.ID] = true
		if CategoryByID(s.Category).ID != s.Category {
			t.Errorf("%s: unknown category %s", s.ID, s.Category)
		}
		if s.Title == "" || len(s.Any) == 0 {
			t.Errorf("%s: missing title or patterns", s.ID)
		}
	}
}
