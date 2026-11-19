package skillgen

import (
	"bytes"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
)

var (
	reSkillName = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,63}$`)
	reRefLink   = regexp.MustCompile("`(references/[a-z0-9.-]+\\.md)`")
	reRuleCode  = regexp.MustCompile(`\bR-\d{4,}\b`)
)

// MaxSkillMDBytes keeps SKILL.md small enough to load into an agent context.
const MaxSkillMDBytes = 200_000

// Validate statically checks a rendered skill. It returns every problem
// found; an empty slice means the skill is publishable.
func Validate(out Output) []string {
	var errs []string
	files := map[string][]byte{}
	for _, f := range out.Files {
		if _, dup := files[f.Path]; dup {
			errs = append(errs, "duplicate file "+f.Path)
		}
		files[f.Path] = f.Content
		if bytes.Contains(f.Content, []byte("<no value>")) || bytes.Contains(f.Content, []byte("{{")) ||
			bytes.Contains(f.Content, []byte(versionPlaceholder)) {
			errs = append(errs, f.Path+": unresolved template marker")
		}
		if !strings.HasSuffix(string(f.Content), "\n") {
			errs = append(errs, f.Path+": missing trailing newline")
		}
	}

	skill, ok := files["SKILL.md"]
	if !ok {
		return append(errs, "SKILL.md missing")
	}
	if len(skill) > MaxSkillMDBytes {
		errs = append(errs, fmt.Sprintf("SKILL.md too large (%d bytes)", len(skill)))
	}

	// Frontmatter.
	text := string(skill)
	if !strings.HasPrefix(text, "---\n") {
		errs = append(errs, "SKILL.md: missing YAML frontmatter")
	} else {
		end := strings.Index(text[4:], "\n---\n")
		if end < 0 {
			errs = append(errs, "SKILL.md: unterminated frontmatter")
		} else {
			fm := map[string]string{}
			for _, line := range strings.Split(text[4:4+end], "\n") {
				if k, v, ok := strings.Cut(line, ":"); ok {
					fm[strings.TrimSpace(k)] = strings.TrimSpace(v)
				}
			}
			if !reSkillName.MatchString(fm["name"]) {
				errs = append(errs, fmt.Sprintf("SKILL.md: invalid name %q", fm["name"]))
			}
			if d := fm["description"]; d == "" || len(d) > 1024 {
				errs = append(errs, "SKILL.md: description missing or longer than 1024 characters")
			} else if strings.ContainsAny(d, "<>") {
				errs = append(errs, "SKILL.md: description must not contain angle brackets")
			}
		}
	}

	// Every referenced file exists.
	for _, m := range reRefLink.FindAllStringSubmatch(text, -1) {
		if _, ok := files[m[1]]; !ok {
			errs = append(errs, "SKILL.md references missing file "+m[1])
		}
	}

	// metadata.json is consistent with the Markdown.
	var meta metaDoc
	if raw, ok := files["metadata.json"]; !ok {
		errs = append(errs, "metadata.json missing")
	} else if err := json.Unmarshal(raw, &meta); err != nil {
		errs = append(errs, "metadata.json invalid: "+err.Error())
	} else {
		if meta.Version != out.Version || meta.ContentHash != out.ContentHash {
			errs = append(errs, "metadata.json version does not match content hash")
		}
		if meta.LLMCalls != 0 || meta.NetworkCalls != 0 {
			errs = append(errs, "metadata.json reports non-zero LLM/network calls")
		}
		if meta.RuleCount != len(meta.Rules) || meta.RuleCount != out.RuleCount {
			errs = append(errs, "metadata.json rule_count mismatch")
		}
		seen := map[string]bool{}
		for _, r := range meta.Rules {
			if seen[r.ID] {
				errs = append(errs, "duplicate rule id "+r.ID)
			}
			seen[r.ID] = true
			if !strings.Contains(text, "**"+r.ID) {
				errs = append(errs, r.ID+": not listed in SKILL.md")
			}
			ref, ok := files[r.File]
			if !ok {
				errs = append(errs, r.ID+": reference file missing "+r.File)
				continue
			}
			section := sectionFor(string(ref), r.ID)
			if section == "" {
				errs = append(errs, r.ID+": no section in "+r.File)
			} else if !strings.Contains(section, "](http") {
				errs = append(errs, r.ID+": no evidence link (provenance) in "+r.File)
			}
		}
		// No rule code appears in SKILL.md that metadata does not know.
		for _, code := range reRuleCode.FindAllString(text, -1) {
			if !seen[code] {
				known := false
				for _, r := range meta.Rules {
					for _, a := range r.Also {
						if a == code {
							known = true
						}
					}
				}
				if !known {
					errs = append(errs, "SKILL.md mentions unknown rule "+code)
				}
			}
		}
	}
	return errs
}

func sectionFor(doc, code string) string {
	start := strings.Index(doc, "## "+code)
	if start < 0 {
		return ""
	}
	rest := doc[start+3:]
	if end := strings.Index(rest, "\n## "); end >= 0 {
		return rest[:end]
	}
	return rest
}
