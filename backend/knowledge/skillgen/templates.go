package skillgen

import (
	"fmt"
	"strings"
	"text/template"
)

var funcs = template.FuncMap{
	"join": strings.Join,
	"conf": func(f float64) string { return fmt.Sprintf("%.2f", f) },
	"codes": func(r Rule) string {
		if len(r.AlsoCodes) == 0 {
			return r.Code
		}
		return r.Code + " (also " + strings.Join(r.AlsoCodes, ", ") + ")"
	},
	"shown": func(ev []Evidence) []Evidence {
		if len(ev) > MaxEvidencePerRule {
			return ev[:MaxEvidencePerRule]
		}
		return ev
	},
	"more": func(ev []Evidence) int {
		if len(ev) > MaxEvidencePerRule {
			return len(ev) - MaxEvidencePerRule
		}
		return 0
	},
	"pending": func(r Rule) string {
		if r.Status != "approved" {
			return " _(unapproved)_"
		}
		return ""
	},
	"cell": func(s string) string { return strings.ReplaceAll(s, "|", "\\|") },
	"plural": func(n int, one, many string) string {
		if n == 1 {
			return one
		}
		return many
	},
}

var skillTmpl = template.Must(template.New("skill").Funcs(funcs).Parse(`---
name: {{.Name}}
description: {{.Description}}
---

# Odoo PR Review — learned from historical reviewer feedback

Generated deterministically by **review-dock** from its review-knowledge database (version ` + "`{{.Version}}`" + `, {{.RuleCount}} {{plural .RuleCount "rule" "rules"}}, {{.Generator}}). Do not edit by hand: the next rebuild overwrites it. ` + "`metadata.json`" + ` lists every rule ID with its statistics.

Every rule below comes from real review comments on {{if .Repositories}}{{join .Repositories ", "}}{{else}}the configured repositories{{end}}. Each rule links to the original comments in its reference file. Treat a rule as **evidence of what reviewers expect**, not as an absolute law. When the surrounding code or the current project conventions clearly differ, follow the code and mention the rule you deliberately did not apply.

{{- if .IncludePending}}

> Unapproved rules are included in this build and are marked _(unapproved)_. Give them less weight.
{{- end}}

## How to use this skill

| Task | Go to |
|---|---|
| Reviewing a PR or diff | A, then B, then the category reference files |
| Preparing or updating your own PR | D |
| Addressing a reviewer's comment | E |
| Checking whether a mistake was flagged before | ` + "`references/common-review-mistakes.md`" + ` and a search of ` + "`references/`" + ` |

## A. Review workflow

1. **Understand the PR first.** Read the title, description, linked task/ticket and every commit message. State in one sentence what the change is supposed to do, and for which version or branch.
2. **Read the surrounding code, not just the diff.** Open each touched method in full, along with its callers and its overrides in other modules. Check how sibling code solves the same problem.
3. **Check the project's conventions** in the touched module: naming, helpers, mixins, patterns already in use. Prefer consistency with existing code.
4. **Apply the historical rules (section B).** For each touched area, open the matching reference file and check every rule against the diff.
5. **Look for regressions.** Consider other callers, multi-record and multi-company cases, empty values, access rights, upgrades of existing databases, and the frontend/backend contract.
6. **Avoid false positives.** Report a finding only when you can point to the exact line and explain the concrete failure or cost. If a rule does not apply here, skip it. Do not repeat generic advice.
7. **Report** findings ordered by severity. For each one give the file and line, the problem, the suggested fix, and the rule ID it relates to (for example ` + "`R-xxxx`" + `).

## B. Historical reviewer rules

Each entry shows the rule ID, the severity, how many times it was raised (×N) and in how many distinct PRs, followed by the rule. Open the reference file for how to check it and for the original reviewer comments.
{{if not .Categories}}
_No approved rules yet. Approve extracted rules in review-dock (Knowledge → Rules) and rebuild the skill. Until then, apply sections A, D and E._
{{- end}}
{{- range .Categories}}

### {{.Title}} — ` + "`references/{{.File}}`" + `
{{range .Rules}}
- **{{codes .}}** [{{.Severity}} · ×{{.Occurrences}} · {{.DistinctPRs}} {{plural .DistinctPRs "PR" "PRs"}}]{{pending .}} {{.Title}}
{{- end}}
{{- end}}

## C. Areas covered by reviewer evidence

{{if .Categories -}}
| Area | Rules | Reference |
|---|---|---|
{{- range .Categories}}
| {{.Title}} | {{len .Rules}} | ` + "`references/{{.File}}`" + ` |
{{- end}}

The skill includes only the areas listed above, because reviewers have actually given feedback in them. Do not infer additional Odoo rules from this list.
{{- else -}}
No area has approved reviewer evidence yet.
{{- end}}

## D. Before submitting or updating a PR

Go through every item and fix what applies before you push:

- [ ] The diff contains only what the PR's purpose requires: no unrelated refactors, formatting changes or debug leftovers.
- [ ] Commit messages explain the problem and the solution, and follow the repository's guidelines.
- [ ] Relevant tests were added or updated, and they pass locally.
- [ ] The change was checked against every touched area's reference file.
{{- range .Checklist}}
- [ ] **{{.Code}}** {{.Title}}
{{- end}}

## E. When implementing reviewer feedback

1. **Read the review comment completely**, including the whole thread and any suggested change.
2. **Understand the actual problem** the reviewer points at, not only the line they commented on. If the comment is ambiguous, say so instead of guessing.
3. **Search for the related historical rule.** Grep ` + "`references/`" + ` for keywords, function names or the file path, and note the rule ID. A rule with many occurrences means that reviewers care about it consistently.
4. **Read the surrounding implementation** before you change anything (callers, overrides, sibling modules).
5. **Apply the smallest correct fix** that addresses the root cause. Do not refactor unrelated code while doing it.
6. **Look for the same problem elsewhere** in the PR and in the touched files, and fix every occurrence.
7. **Run the relevant tests** (the module's tests and any tour involved), and add a regression test when it is feasible.
8. **Explain what changed**: in your reply to the reviewer, summarise the fix, mention the rule ID if one applies, and list the other places you fixed.
`))

var categoryTmpl = template.Must(template.New("category").Funcs(funcs).Parse(`# {{.Title}} — reviewer rules

Part of the review skill version ` + "`{{.Version}}`" + `. These rules come from historical review comments. Evidence quotes are the reviewers' own words, shortened, with markup removed.
{{range .Rules}}

## {{codes .}} — {{.Title}}{{pending .}}

- **Severity:** {{.Severity}} · **Scope:** {{if eq .Scope "general"}}general (seen in several PRs){{else}}context-specific (seen in one PR){{end}}
- **History:** raised {{.Occurrences}} {{plural .Occurrences "time" "times"}} in {{.DistinctPRs}} {{plural .DistinctPRs "PR" "PRs"}} by {{.DistinctReviewers}} {{plural .DistinctReviewers "reviewer" "reviewers"}} · **Confidence:** {{conf .Confidence}}
{{- if .Subcategory}}
- **Topic:** {{.Subcategory}}
{{- end}}
{{- if .Guidance}}

**How to check:** {{.Guidance}}
{{- end}}

**Evidence:**
{{range shown .Evidence}}
- [{{.Repo}}#{{.PRNumber}}]({{.URL}}) · @{{.Reviewer}} · {{.Date}}{{if .Path}} · ` + "`{{.Path}}`" + `{{end}}{{if .Addressed}} · led to a code change{{end}}
  > {{.Quote}}
{{- end}}
{{- if gt (more .Evidence) 0}}
- … and {{more .Evidence}} more in review-dock (Knowledge → Rules → {{.Code}}).
{{- end}}
{{- end}}
`))

var commonTmpl = template.Must(template.New("common").Funcs(funcs).Parse(`# Most common review mistakes

The rules reviewers raised most often across all areas, part of skill version ` + "`{{.Version}}`" + `. Check these first on every PR.
{{if not .Common}}
_No approved rules yet._
{{- else}}

| Rule | Times raised | PRs | Severity | Area | Rule text |
|---|---|---|---|---|---|
{{- range .Common}}
| {{.Code}} | {{.Occurrences}} | {{.DistinctPRs}} | {{.Severity}} | {{.Category}} | {{cell .Title}}{{pending .}} |
{{- end}}
{{- end}}
`))
