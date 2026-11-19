package extract

import (
	"fmt"
	"sort"
	"strings"
)

// Comment is the extractor's view of a stored review comment.
type Comment struct {
	ID           int64 // local id (review_comments.id)
	GitHubID     int64
	Source       string // inline|issue|review
	PRID         int64
	Author       string
	IsBot        bool
	IsPRAuthor   bool
	Body         string
	Path         string
	ReviewState  string
	InReplyToID  int64
	ThreadRootID int64
	CreatedAt    string
}

// Analysis is the per-comment classification.
type Analysis struct {
	CommentID   int64
	Kind        string
	Category    string
	Subcategory string
	Signals     []string
	Normalized  string
}

// EvidenceLink ties a comment to a proposed rule.
type EvidenceLink struct {
	CommentID int64
	Kind      string
	Score     float64
}

// ProposedRule is a rule backed by at least one evidence link.
type ProposedRule struct {
	Key         string // sig:<id> | cand:c<anchor comment id>
	Origin      string // signal|candidate
	Category    string
	Subcategory string
	Title       string
	Guidance    string
	Severity    string
	Evidence    []EvidenceLink
}

// Options tunes extraction. Zero values select defaults.
type Options struct {
	IgnoredAuthors []string
	// Candidate (non-taxonomy) clusters need feedback from at least this many
	// distinct PRs before they are proposed as rules. Default 2.
	CandidateMinPRs int
	// Jaccard similarity threshold for clustering candidates. Default 0.5.
	CandidateSimilarity float64
	// Anchors are comment ids of existing candidate rules; they seed clusters
	// first so rule keys stay stable across runs.
	Anchors []int64
}

// Result is the complete, deterministic output of Analyze.
type Result struct {
	Analyses []Analysis
	Rules    []ProposedRule
}

// Analyze classifies every comment and proposes evidence-backed rules.
// Output depends only on the inputs: same comments + options => same result.
func Analyze(comments []Comment, opts Options) Result {
	if opts.CandidateMinPRs <= 0 {
		opts.CandidateMinPRs = 2
	}
	if opts.CandidateSimilarity <= 0 {
		opts.CandidateSimilarity = 0.5
	}
	ignored := map[string]bool{}
	for _, a := range opts.IgnoredAuthors {
		ignored[strings.ToLower(strings.TrimSpace(a))] = true
	}

	cs := make([]Comment, len(comments))
	copy(cs, comments)
	sort.SliceStable(cs, func(i, j int) bool {
		if cs[i].CreatedAt != cs[j].CreatedAt {
			return cs[i].CreatedAt < cs[j].CreatedAt
		}
		return cs[i].ID < cs[j].ID
	})

	type state struct {
		c        Comment
		n        Normalized
		kind     string
		cat, sub string
		signals  []signalMatch
	}
	states := make([]*state, len(cs))
	byGitHub := map[string]*state{} // source:github_id

	for i, c := range cs {
		n := Normalize(c.Body)
		st := &state{c: c, n: n}
		login := strings.ToLower(c.Author)
		switch {
		case c.IsBot || ignored[login] || strings.HasSuffix(login, "[bot]"):
			st.kind = KindBot
		case c.IsPRAuthor:
			st.kind = KindSelf
		default:
			st.kind = ClassifyKind(n, c.ReviewState)
		}
		st.signals = MatchSignals(n.Text, c.Path)
		if len(st.signals) > 0 {
			sig, _ := SignalByID(st.signals[0].ID)
			st.cat, st.sub = sig.Category, sig.Subcategory
		} else if cat, sub := pathCategory(c.Path); cat != "" {
			st.cat, st.sub = cat, sub
		} else {
			st.cat = "general"
		}
		states[i] = st
		byGitHub[fmt.Sprintf("%s:%d", c.Source, c.GitHubID)] = st
	}

	// --- Thread-level supersession (inline review threads) ---------------
	threads := map[int64][]*state{}
	var threadKeys []int64
	for _, st := range states {
		if st.c.Source != "inline" || st.c.ThreadRootID == 0 {
			continue
		}
		if _, ok := threads[st.c.ThreadRootID]; !ok {
			threadKeys = append(threadKeys, st.c.ThreadRootID)
		}
		threads[st.c.ThreadRootID] = append(threads[st.c.ThreadRootID], st)
	}
	sort.Slice(threadKeys, func(i, j int) bool { return threadKeys[i] < threadKeys[j] })
	for _, key := range threadKeys {
		root := byGitHub[fmt.Sprintf("inline:%d", key)]
		if root == nil || !UsableKind(root.kind) {
			continue
		}
		pushedBack := false
		for _, r := range threads[key] {
			if r == root {
				continue
			}
			t := r.n.Text
			switch {
			case r.c.IsPRAuthor:
				if reAuthorPushback.MatchString(t) && !reAuthorFixed.MatchString(t) {
					pushedBack = true
				} else if reAuthorFixed.MatchString(t) {
					pushedBack = false
				}
			case strings.EqualFold(r.c.Author, root.c.Author):
				if r.kind == KindSuperseded ||
					(pushedBack && reConcede.MatchString(t) && !reRequireStrong.MatchString(t)) {
					root.kind = KindSuperseded
				}
			}
		}
	}

	// --- Evidence for taxonomy signals -----------------------------------
	type evKey struct {
		rule   string
		thread string
	}
	seen := map[evKey]bool{}
	ruleEv := map[string][]EvidenceLink{}
	threadOf := func(st *state) string {
		if st.c.Source == "inline" && st.c.ThreadRootID != 0 {
			return fmt.Sprintf("t%d", st.c.ThreadRootID)
		}
		return fmt.Sprintf("c%d", st.c.ID)
	}
	var candidates []*state
	for _, st := range states {
		if !UsableKind(st.kind) {
			continue
		}
		if len(st.signals) == 0 {
			candidates = append(candidates, st)
			continue
		}
		for _, m := range st.signals {
			key := "sig:" + m.ID
			k := evKey{key, threadOf(st)}
			if seen[k] {
				continue // count a thread once per rule
			}
			seen[k] = true
			ruleEv[key] = append(ruleEv[key], EvidenceLink{CommentID: st.c.ID, Kind: st.kind, Score: round2(m.Score)})
		}
	}

	// --- Candidate clustering (feedback outside the taxonomy) -------------
	type cluster struct {
		anchor  *state
		tokens  [][]string
		members []*state
		prs     map[int64]bool
	}
	var clusters []*cluster
	anchorSet := map[int64]bool{}
	for _, id := range opts.Anchors {
		anchorSet[id] = true
	}
	tokensOf := map[*state][]string{}
	keySentence := map[*state]string{}
	var pool []*state
	for _, st := range candidates {
		ks := pickKeySentence(st.n)
		toks := Tokens(ks)
		if len(toks) < 3 {
			toks = Tokens(st.n.Text)
		}
		if len(toks) < 3 {
			continue // too little content to generalise
		}
		tokensOf[st] = toks
		keySentence[st] = ks
		pool = append(pool, st)
	}
	// Anchors first (stable keys), then everything else in chronological order.
	sort.SliceStable(pool, func(i, j int) bool {
		ai, aj := anchorSet[pool[i].c.ID], anchorSet[pool[j].c.ID]
		return ai && !aj
	})
	for _, st := range pool {
		var best *cluster
		bestSim := 0.0
		for _, cl := range clusters {
			if cl.anchor.cat != st.cat {
				continue
			}
			for _, t := range cl.tokens {
				if s := Jaccard(t, tokensOf[st]); s > bestSim {
					bestSim, best = s, cl
				}
			}
		}
		if best != nil && bestSim >= opts.CandidateSimilarity {
			best.members = append(best.members, st)
			best.tokens = append(best.tokens, tokensOf[st])
			best.prs[st.c.PRID] = true
			continue
		}
		clusters = append(clusters, &cluster{anchor: st, tokens: [][]string{tokensOf[st]}, members: []*state{st},
			prs: map[int64]bool{st.c.PRID: true}})
	}

	var rules []ProposedRule
	for _, cl := range clusters {
		if len(cl.prs) < opts.CandidateMinPRs && !anchorSet[cl.anchor.c.ID] {
			continue
		}
		pr := ProposedRule{
			Key:         fmt.Sprintf("cand:c%d", cl.anchor.c.ID),
			Origin:      "candidate",
			Category:    cl.anchor.cat,
			Subcategory: cl.anchor.sub,
			Title:       Truncate(keySentence[cl.anchor], 220),
			Guidance:    "",
			Severity:    "context-dependent",
		}
		threadSeen := map[string]bool{}
		for _, m := range cl.members {
			th := threadOf(m)
			if threadSeen[th] {
				continue
			}
			threadSeen[th] = true
			sim := Jaccard(tokensOf[cl.anchor], tokensOf[m])
			pr.Evidence = append(pr.Evidence, EvidenceLink{CommentID: m.c.ID, Kind: m.kind, Score: round2(sim)})
		}
		rules = append(rules, pr)
	}

	for key, ev := range ruleEv {
		sig, _ := SignalByID(strings.TrimPrefix(key, "sig:"))
		rules = append(rules, ProposedRule{
			Key: key, Origin: "signal", Category: sig.Category, Subcategory: sig.Subcategory,
			Title: sig.Title, Guidance: sig.Guidance, Severity: sig.Severity, Evidence: ev,
		})
	}
	sort.Slice(rules, func(i, j int) bool { return rules[i].Key < rules[j].Key })
	for i := range rules {
		sort.Slice(rules[i].Evidence, func(a, b int) bool { return rules[i].Evidence[a].CommentID < rules[i].Evidence[b].CommentID })
	}

	analyses := make([]Analysis, 0, len(states))
	for _, st := range states {
		ids := make([]string, len(st.signals))
		for i, m := range st.signals {
			ids[i] = m.ID
		}
		analyses = append(analyses, Analysis{
			CommentID: st.c.ID, Kind: st.kind, Category: st.cat, Subcategory: st.sub,
			Signals: ids, Normalized: Truncate(st.n.Text, 2000),
		})
	}
	sort.Slice(analyses, func(i, j int) bool { return analyses[i].CommentID < analyses[j].CommentID })
	return Result{Analyses: analyses, Rules: rules}
}

// pickKeySentence returns the sentence that carries the request.
func pickKeySentence(n Normalized) string {
	for _, s := range n.Sentences {
		l := strings.ToLower(s)
		if reRequireStrong.MatchString(l) || reSuggest.MatchString(l) {
			return s
		}
	}
	if len(n.Sentences) > 0 {
		return n.Sentences[0]
	}
	return n.Display
}

func round2(f float64) float64 {
	return float64(int64(f*100+0.5)) / 100
}
