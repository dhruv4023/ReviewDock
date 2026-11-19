package extract

import (
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// Feedback kinds.
const (
	KindRequirement = "requirement" // the reviewer asks for a change
	KindSuggestion  = "suggestion"  // optional / nit / "consider"
	KindQuestion    = "question"    // a question without an explicit request
	KindSuperseded  = "superseded"  // withdrawn by the reviewer or rejected in discussion
	KindNoise       = "noise"       // approvals, thanks, bot commands
	KindUnclear     = "unclear"     // could not be classified
	KindSelf        = "self"        // written by the PR author (context only)
	KindBot         = "bot"         // automated account
)

// UsableKind reports whether a comment of this kind can be rule evidence.
func UsableKind(k string) bool { return k == KindRequirement || k == KindSuggestion }

var (
	reNoise  = regexp.MustCompile(`^(lgtm|lgtm!|thanks?( you)?!?|thx|ty|ok|okay|done|nice|great|good|perfect|cool|\+1|approved?|merci|:\+1:|👍|ack|r\+|robodoo .*|@?robodoo .*|fw-bot .*|ping|up|retry|rebase.*)[.! ]*$`)
	reBotCmd = regexp.MustCompile(`^(@?robodoo|@?fw-bot|@?mergebot)\b`)

	reSupersede = regexp.MustCompile(`\b(never ?mind|ignore (this|that|my (previous|last) comment|me)|my bad|forget (it|this|that|what i said)|scratch that|disregard|i was wrong|not relevant anymore|no longer relevant|you'?re right|fair enough)\b`)

	reSuggest = regexp.MustCompile(`(^|\W)(nit|nitpick|optional|optionally|maybe|perhaps|might|could you consider|consider|what about|how about|why not|i'?d rather|i would|not blocking|non-blocking|up to you|fyi|feel free|if you want|personally|minor:|could be|would be (nice|better|cleaner))\b`)

	reRequireStrong = regexp.MustCompile(`(^|\W)(please|pls|plz|could you|can you|would you|must|should|shouldn'?t|need to|needs to|have to|has to|don'?t|do not|avoid|never|remove|revert|replace|rename|missing|forgot|wrong|incorrect|broken|this (will )?breaks?|will break|not correct|not needed|no need|useless|unnecessary|instead)\b`)
	reRequireWeak   = regexp.MustCompile(`(^|\W)(use|move|drop|required|mandatory|can'?t|cannot|isn'?t|is not)\b`)
	// A reviewer reporting a concrete defect is an implied requirement.
	reProblem = regexp.MustCompile(`(^|\W)(will be slow|too slow|slow|inefficient|expensive|costly|crash(es)?|traceback|will fail|fails|bug|regression|leaks?|race condition)\b`)

	// Author pushes back / reviewer concedes, for thread-level supersession.
	reAuthorPushback = regexp.MustCompile(`\b(not needed|no because|because|i don'?t think|it is needed|it'?s needed|on purpose|intended|intentional(ly)?|won'?t|can'?t|not possible|actually|on the contrary|it is required|it'?s required|we need it|that'?s expected|expected behaviou?r)\b`)
	reAuthorFixed    = regexp.MustCompile(`\b(done|fixed|changed|updated|good catch|indeed|applied|addressed|adapted|removed|modified)\b`)
	reConcede        = regexp.MustCompile(`\b(ok|okay|fair enough|right|makes sense|i see|got it|you'?re right|indeed|fine( for me)?|alright|understood|never ?mind|my bad|agreed?)\b`)
)

// ClassifyKind assigns a feedback kind from the normalized text alone.
// Thread context (supersession by discussion) is applied separately.
func ClassifyKind(n Normalized, reviewState string) string {
	t := strings.TrimSpace(n.Text)
	if t == "" {
		if n.HasSuggestion {
			return KindRequirement // a bare suggestion block is a concrete change request
		}
		if reviewState == "APPROVED" {
			return KindNoise
		}
		return KindUnclear
	}
	if reBotCmd.MatchString(t) || reNoise.MatchString(t) {
		return KindNoise
	}
	if reSupersede.MatchString(t) {
		return KindSuperseded
	}
	words := len(strings.Fields(t))
	isSuggest := reSuggest.MatchString(t)
	isStrong := reRequireStrong.MatchString(t) || n.HasSuggestion
	switch {
	case isSuggest:
		return KindSuggestion
	case isStrong:
		return KindRequirement
	case strings.Contains(t, "?"):
		return KindQuestion
	case reRequireWeak.MatchString(t) || reProblem.MatchString(t):
		return KindRequirement
	case reviewState == "CHANGES_REQUESTED" && words >= 3:
		return KindRequirement
	case words <= 2:
		return KindNoise
	}
	return KindUnclear
}

// signalMatch is one signal hit with its score.
type signalMatch struct {
	ID    string
	Score float64
}

// MatchSignals returns the signals matching text/path, best first: the top
// match and, optionally, the best match from another category.
func MatchSignals(text, path string) []signalMatch {
	var out []signalMatch
	for idx, s := range Signals {
		hits := 0
		for _, r := range s.Any {
			if r.MatchString(text) {
				hits++
			}
		}
		if hits == 0 {
			continue
		}
		ok := true
		for _, r := range s.All {
			if !r.MatchString(text) {
				ok = false
				break
			}
		}
		if !ok {
			continue
		}
		if len(s.PathAny) > 0 {
			pathOK := false
			for _, r := range s.PathAny {
				if r.MatchString(path) {
					pathOK = true
					break
				}
			}
			if !pathOK {
				continue
			}
		}
		// Deterministic score: number of distinct patterns hit, with a tiny
		// order-based tiebreak so that sorting is total.
		score := float64(hits) + 0.001*float64(len(Signals)-idx)
		out = append(out, signalMatch{ID: s.ID, Score: score})
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Score > out[j].Score })
	// Drop weak catch-alls when a specific signal matched.
	var specific []signalMatch
	for _, m := range out {
		if s, _ := SignalByID(m.ID); !s.Weak {
			specific = append(specific, m)
		}
	}
	if len(specific) > 0 {
		out = specific
	}
	if len(out) == 0 {
		return nil
	}
	// Keep the best match, plus at most one more from a *different* category:
	// one comment should not spawn two near-duplicate rules in the same area.
	best := out[0]
	bestSig, _ := SignalByID(best.ID)
	for _, m := range out[1:] {
		if s, _ := SignalByID(m.ID); s.Category != bestSig.Category {
			return []signalMatch{best, m}
		}
	}
	return []signalMatch{best}
}

// SignalByID returns the signal definition.
func SignalByID(id string) (Signal, bool) {
	for _, s := range Signals {
		if s.ID == id {
			return s, true
		}
	}
	return Signal{}, false
}

func contains(s string, subs ...string) bool {
	for _, x := range subs {
		if strings.Contains(s, x) {
			return true
		}
	}
	return false
}

func hasSuffix(s string, sufs ...string) bool {
	for _, x := range sufs {
		if strings.HasSuffix(s, x) {
			return true
		}
	}
	return false
}

func hasPrefixBase(path, prefix string) bool {
	return strings.HasPrefix(filepath.Base(path), prefix)
}
