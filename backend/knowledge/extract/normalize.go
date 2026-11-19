// Package extract turns raw reviewer comments into structured review rules
// using only deterministic text processing: regular expressions, keyword
// taxonomies, token-set similarity and counting. No LLMs, no embeddings and
// no network access (enforced by backend/knowledge/zerotoken_test.go).
package extract

import (
	"regexp"
	"sort"
	"strings"
	"unicode"
)

// Version identifies the extraction algorithm. Bump it whenever taxonomy,
// classification or clustering logic changes, so stored analyses are
// recognisably stale.
const Version = "extract/1"

var (
	reSuggestionBlock = regexp.MustCompile("(?s)```suggestion[^\\n]*\\n.*?```")
	reCodeBlock       = regexp.MustCompile("(?s)```.*?```")
	reHTMLComment     = regexp.MustCompile(`(?s)<!--.*?-->`)
	reImage           = regexp.MustCompile(`!\[[^\]]*\]\([^)]*\)`)
	reLink            = regexp.MustCompile(`\[([^\]]+)\]\([^)]*\)`)
	reURL             = regexp.MustCompile(`https?://\S+`)
	reHTMLTag         = regexp.MustCompile(`</?[a-zA-Z][^>]*>`)
	reMention         = regexp.MustCompile(`(^|\s)@[A-Za-z0-9_-]+`)
	reSpaces          = regexp.MustCompile(`\s+`)
	reSentenceSplit   = regexp.MustCompile(`([.!?])\s+|\n+`)
	reWord            = regexp.MustCompile(`[a-z0-9_]+`)
)

// Normalized is the cleaned form of a comment body.
type Normalized struct {
	Text          string // lower-cased, whitespace-collapsed, for matching
	Display       string // original casing, markup stripped, single line
	Sentences     []string
	HasSuggestion bool // contained a GitHub ```suggestion block
	HasCode       bool
}

// Normalize strips quotes, markup, links and code blocks from a comment.
// Inline `code` content is kept because identifiers are strong signals.
func Normalize(body string) Normalized {
	n := Normalized{}
	if reSuggestionBlock.MatchString(body) {
		n.HasSuggestion = true
		body = reSuggestionBlock.ReplaceAllString(body, " ")
	}
	if reCodeBlock.MatchString(body) {
		n.HasCode = true
		body = reCodeBlock.ReplaceAllString(body, " ")
	}
	body = reHTMLComment.ReplaceAllString(body, " ")

	// Drop quoted lines ("> ..."): they repeat someone else's words.
	var kept []string
	for _, line := range strings.Split(body, "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), ">") {
			continue
		}
		kept = append(kept, line)
	}
	body = strings.Join(kept, "\n")

	body = reImage.ReplaceAllString(body, " ")
	body = reLink.ReplaceAllString(body, "$1")
	body = reURL.ReplaceAllString(body, " ")
	body = reHTMLTag.ReplaceAllString(body, " ")
	body = reMention.ReplaceAllString(body, "$1")
	body = strings.NewReplacer("**", "", "__", "", "`", "", "~~", "").Replace(body)

	// Sentences are split before collapsing newlines.
	for _, s := range reSentenceSplit.Split(body, -1) {
		s = strings.TrimSpace(reSpaces.ReplaceAllString(s, " "))
		s = strings.TrimLeft(s, "-*#0123456789. ")
		if s != "" {
			n.Sentences = append(n.Sentences, s)
		}
	}
	n.Display = strings.TrimSpace(reSpaces.ReplaceAllString(body, " "))
	n.Text = strings.ToLower(n.Display)
	return n
}

var stopwords = map[string]bool{}

func init() {
	for _, w := range strings.Fields(`a an the this that these those it its is are was were be been being to of in on at by
		for with as from or and but if then else so not no do does did done can could would should will shall may might
		i you we they he she me my your our their them us here there what which who whom when where why how all any
		some just also very really please maybe perhaps think thing things use used using one ones like more less than
		have has had get got make made need needs needed`) {
		stopwords[w] = true
	}
}

// stem applies a tiny, deterministic suffix stripper (enough for similarity,
// not linguistics).
func stem(w string) string {
	for _, suf := range []string{"ings", "ing", "edly", "ed", "ies", "es", "s"} {
		if len(w) > len(suf)+3 && strings.HasSuffix(w, suf) {
			if suf == "ies" {
				return w[:len(w)-3] + "y"
			}
			return w[:len(w)-len(suf)]
		}
	}
	return w
}

// Tokens returns the sorted, de-duplicated content-word stems of text.
func Tokens(text string) []string {
	set := map[string]bool{}
	for _, w := range reWord.FindAllString(strings.ToLower(text), -1) {
		if len(w) < 3 || stopwords[w] || isNumber(w) {
			continue
		}
		set[stem(w)] = true
	}
	out := make([]string, 0, len(set))
	for w := range set {
		out = append(out, w)
	}
	sort.Strings(out)
	return out
}

func isNumber(s string) bool {
	for _, r := range s {
		if !unicode.IsDigit(r) {
			return false
		}
	}
	return true
}

// Jaccard similarity between two sorted token sets.
func Jaccard(a, b []string) float64 {
	if len(a) == 0 && len(b) == 0 {
		return 1
	}
	i, j, inter := 0, 0, 0
	for i < len(a) && j < len(b) {
		switch {
		case a[i] == b[j]:
			inter++
			i++
			j++
		case a[i] < b[j]:
			i++
		default:
			j++
		}
	}
	union := len(a) + len(b) - inter
	if union == 0 {
		return 0
	}
	return float64(inter) / float64(union)
}

// Truncate shortens s to at most n runes on a word boundary.
func Truncate(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	cut := string(r[:n])
	if i := strings.LastIndex(cut, " "); i > n/2 {
		cut = cut[:i]
	}
	return strings.TrimRight(cut, " ,;:") + "…"
}
