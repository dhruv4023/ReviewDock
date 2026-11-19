// Package skillgen renders the AI-agent review skill from a knowledge
// snapshot using text/template. It is a pure function of its input: no
// clock, no randomness, no network, no subprocesses, no LLM. Identical
// snapshots produce byte-identical files (enforced by tests, and the import
// boundary is enforced by backend/knowledge/zerotoken_test.go).
package skillgen

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"text/template"
)

// Version identifies the generator templates/logic.
const Version = "skillgen/1"

const versionPlaceholder = "@@SKILL_VERSION@@"

// Evidence is one original reviewer comment supporting a rule.
type Evidence struct {
	Repo      string
	PRNumber  int
	PRURL     string
	URL       string // comment URL (falls back to PR URL)
	GitHubID  int64
	Source    string
	Reviewer  string
	Path      string
	Date      string // YYYY-MM-DD
	Quote     string // original wording, markup stripped, truncated
	Addressed bool   // feedback led to a code change
}

// Rule is a publishable rule.
type Rule struct {
	ID                int64
	Code              string
	AlsoCodes         []string // exact duplicates collapsed into this rule
	Category          string
	Subcategory       string
	Title             string
	Guidance          string
	Scope             string
	Severity          string
	Status            string
	Occurrences       int
	DistinctPRs       int
	DistinctReviewers int
	Confidence        float64
	Evidence          []Evidence
}

// Category is a rule group rendered to its own reference file.
type Category struct {
	ID    string
	Title string
	File  string
}

// Snapshot is everything the generator reads.
type Snapshot struct {
	SkillName        string
	Repositories     []string
	IncludePending   bool
	Categories       []Category // rendering order; the last one catches unknown ids
	ExtractorVersion string
	Rules            []Rule
}

// File is one generated file.
type File struct {
	Path    string
	Content []byte
}

// Output is the rendered skill.
type Output struct {
	Files       []File // sorted by path
	ContentHash string // sha256 over all files (version placeholder unresolved)
	Version     string // first 12 hex chars of ContentHash
	RuleCount   int
	TotalBytes  int
}

// MaxEvidencePerRule limits quoted comments per rule in reference files.
const MaxEvidencePerRule = 5

var reNonAlnum = regexp.MustCompile(`[^a-z0-9]+`)

func normTitle(s string) string {
	return strings.Trim(reNonAlnum.ReplaceAllString(strings.ToLower(s), " "), " ")
}

type catView struct {
	ID, Title, File string
	Rules           []Rule
}

type tmplData struct {
	Name           string
	Version        string
	Description    string
	Repositories   []string
	RuleCount      int
	IncludePending bool
	Categories     []catView
	Checklist      []Rule
	Common         []Rule
	Generator      string
	Extractor      string
}

// Generate renders the skill. It never fails on empty knowledge: a skill
// with zero rules still contains the review workflow.
func Generate(s Snapshot) (Output, error) {
	if s.SkillName == "" {
		s.SkillName = "odoo-pr-review"
	}
	if len(s.Categories) == 0 {
		s.Categories = []Category{{ID: "general", Title: "General", File: "general.md"}}
	}
	catOrder := func(id string) int {
		for i, c := range s.Categories {
			if c.ID == id {
				return i
			}
		}
		return len(s.Categories) - 1
	}
	rules := dedupe(s.Rules)
	for i := range rules {
		sortEvidence(rules[i].Evidence)
	}
	sort.SliceStable(rules, func(i, j int) bool {
		a, b := rules[i], rules[j]
		if oa, ob := catOrder(a.Category), catOrder(b.Category); oa != ob {
			return oa < ob
		}
		if a.Occurrences != b.Occurrences {
			return a.Occurrences > b.Occurrences
		}
		if a.DistinctPRs != b.DistinctPRs {
			return a.DistinctPRs > b.DistinctPRs
		}
		return a.ID < b.ID
	})

	var cats []catView
	for i, c := range s.Categories {
		cv := catView{ID: c.ID, Title: c.Title, File: c.File}
		for _, r := range rules {
			if catOrder(r.Category) == i {
				cv.Rules = append(cv.Rules, r)
			}
		}
		if len(cv.Rules) > 0 {
			cats = append(cats, cv)
		}
	}

	ranked := make([]Rule, len(rules))
	copy(ranked, rules)
	sort.SliceStable(ranked, func(i, j int) bool {
		a, b := ranked[i], ranked[j]
		if a.Occurrences != b.Occurrences {
			return a.Occurrences > b.Occurrences
		}
		if a.DistinctPRs != b.DistinctPRs {
			return a.DistinctPRs > b.DistinctPRs
		}
		return a.ID < b.ID
	})
	common := ranked
	if len(common) > 15 {
		common = common[:15]
	}
	var checklist []Rule
	for _, r := range ranked {
		if r.Scope == "general" || r.Severity == "high" {
			checklist = append(checklist, r)
		}
		if len(checklist) == 20 {
			break
		}
	}

	repos := append([]string(nil), s.Repositories...)
	sort.Strings(repos)
	data := tmplData{
		Name: s.SkillName, Version: versionPlaceholder, Repositories: repos, RuleCount: len(rules),
		IncludePending: s.IncludePending, Categories: cats, Checklist: checklist, Common: common,
		Generator: Version, Extractor: s.ExtractorVersion,
	}
	data.Description = description(data)

	var files []File
	render := func(path string, t *template.Template, d any) error {
		var buf bytes.Buffer
		if err := t.Execute(&buf, d); err != nil {
			return fmt.Errorf("render %s: %w", path, err)
		}
		files = append(files, File{Path: path, Content: tidy(buf.Bytes())})
		return nil
	}
	if err := render("SKILL.md", skillTmpl, data); err != nil {
		return Output{}, err
	}
	for _, c := range cats {
		if err := render("references/"+c.File, categoryTmpl, struct {
			catView
			Version string
		}{c, versionPlaceholder}); err != nil {
			return Output{}, err
		}
	}
	if err := render("references/common-review-mistakes.md", commonTmpl, data); err != nil {
		return Output{}, err
	}
	sort.Slice(files, func(i, j int) bool { return files[i].Path < files[j].Path })

	h := sha256.New()
	for _, f := range files {
		h.Write([]byte(f.Path))
		h.Write([]byte{0})
		h.Write(f.Content)
		h.Write([]byte{0})
	}
	hash := hex.EncodeToString(h.Sum(nil))
	version := hash[:12]

	meta, err := metadata(s.SkillName, s.ExtractorVersion, hash, version, rules, cats, repos)
	if err != nil {
		return Output{}, err
	}
	out := Output{ContentHash: hash, Version: version, RuleCount: len(rules)}
	for i := range files {
		files[i].Content = bytes.ReplaceAll(files[i].Content, []byte(versionPlaceholder), []byte(version))
	}
	files = append(files, File{Path: "metadata.json", Content: meta})
	sort.Slice(files, func(i, j int) bool { return files[i].Path < files[j].Path })
	for _, f := range files {
		out.TotalBytes += len(f.Content)
	}
	out.Files = files
	return out, nil
}

// dedupe collapses rules with identical normalized titles (keeps lowest id).
func dedupe(in []Rule) []Rule {
	sorted := append([]Rule(nil), in...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].ID < sorted[j].ID })
	idx := map[string]int{}
	var out []Rule
	for _, r := range sorted {
		k := normTitle(r.Title)
		if i, ok := idx[k]; ok {
			keep := &out[i]
			keep.AlsoCodes = append(keep.AlsoCodes, r.Code)
			keep.Occurrences += r.Occurrences
			if r.DistinctPRs > keep.DistinctPRs {
				keep.DistinctPRs = r.DistinctPRs
			}
			if r.Confidence > keep.Confidence {
				keep.Confidence = r.Confidence
			}
			seen := map[string]bool{}
			for _, e := range keep.Evidence {
				seen[e.URL] = true
			}
			for _, e := range r.Evidence {
				if !seen[e.URL] {
					keep.Evidence = append(keep.Evidence, e)
				}
			}
			continue
		}
		idx[k] = len(out)
		out = append(out, r)
	}
	return out
}

func sortEvidence(ev []Evidence) {
	sort.SliceStable(ev, func(i, j int) bool {
		if ev[i].Date != ev[j].Date {
			return ev[i].Date > ev[j].Date // most recent first
		}
		if ev[i].Repo != ev[j].Repo {
			return ev[i].Repo < ev[j].Repo
		}
		return ev[i].GitHubID < ev[j].GitHubID
	})
}

func description(d tmplData) string {
	repos := "the configured repositories"
	if len(d.Repositories) > 0 {
		repos = strings.Join(d.Repositories, ", ")
	}
	var catNames []string
	for _, c := range d.Categories {
		catNames = append(catNames, strings.ToLower(c.Title))
	}
	desc := fmt.Sprintf("Review Odoo pull requests and implement reviewer-requested changes using %d rules learned from real historical reviewer feedback on %s.", d.RuleCount, repos)
	if len(catNames) > 0 {
		desc += " Covers " + strings.Join(catNames, ", ") + "."
	}
	desc += " Use when reviewing a PR or diff, before opening or updating a PR, when addressing review comments, or when fixing bugs raised in review."
	if len(desc) > 1000 {
		desc = desc[:997] + "..."
	}
	return desc
}

// tidy normalises whitespace: no trailing spaces, at most one blank line,
// exactly one trailing newline.
func tidy(b []byte) []byte {
	lines := strings.Split(strings.ReplaceAll(string(b), "\r\n", "\n"), "\n")
	var out []string
	blank := 0
	for _, l := range lines {
		l = strings.TrimRight(l, " \t")
		if l == "" {
			blank++
			if blank > 1 {
				continue
			}
		} else {
			blank = 0
		}
		out = append(out, l)
	}
	return []byte(strings.TrimSpace(strings.Join(out, "\n")) + "\n")
}

type metaRule struct {
	ID          string   `json:"id"`
	Also        []string `json:"also,omitempty"`
	Category    string   `json:"category"`
	Status      string   `json:"status"`
	Severity    string   `json:"severity"`
	Scope       string   `json:"scope"`
	Occurrences int      `json:"occurrences"`
	DistinctPRs int      `json:"distinct_prs"`
	Confidence  float64  `json:"confidence"`
	Title       string   `json:"title"`
	File        string   `json:"file"`
}

type metaDoc struct {
	Name         string         `json:"name"`
	Version      string         `json:"version"`
	ContentHash  string         `json:"content_hash"`
	Generator    string         `json:"generator"`
	Extractor    string         `json:"extractor"`
	Source       string         `json:"source"`
	LLMCalls     int            `json:"llm_calls"`
	NetworkCalls int            `json:"network_calls"`
	Repositories []string       `json:"repositories"`
	RuleCount    int            `json:"rule_count"`
	Categories   map[string]int `json:"categories"`
	Rules        []metaRule     `json:"rules"`
}

func metadata(name, extractor, hash, version string, rules []Rule, cats []catView, repos []string) ([]byte, error) {
	m := metaDoc{
		Name: name, Version: version, ContentHash: hash, Generator: Version, Extractor: extractor,
		Source:       "review-dock knowledge database (deterministic templates; no AI involved)",
		Repositories: repos, RuleCount: len(rules), Categories: map[string]int{}, Rules: []metaRule{},
	}
	if m.Repositories == nil {
		m.Repositories = []string{}
	}
	for _, c := range cats {
		m.Categories[c.ID] = len(c.Rules)
		for _, r := range c.Rules {
			m.Rules = append(m.Rules, metaRule{ID: r.Code, Also: r.AlsoCodes, Category: c.ID, Status: r.Status, Severity: r.Severity,
				Scope: r.Scope, Occurrences: r.Occurrences, DistinctPRs: r.DistinctPRs, Confidence: r.Confidence,
				Title: r.Title, File: "references/" + c.File})
		}
	}
	b, err := json.MarshalIndent(m, "", "  ") // map keys are sorted by encoding/json
	if err != nil {
		return nil, err
	}
	return append(b, '\n'), nil
}
