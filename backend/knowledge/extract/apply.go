package extract

import (
	"context"
	"database/sql"
	"errors"
	"sort"
	"strconv"
	"strings"

	"review-dock/backend/knowledge/db"
)

// Summary reports what an extraction run changed.
type Summary struct {
	CommentsAnalyzed int            `json:"comments_analyzed"`
	ByKind           map[string]int `json:"by_kind"`
	RulesTotal       int            `json:"rules_total"`
	RulesAdded       int            `json:"rules_added"`
	RulesRemoved     int            `json:"rules_removed"`
	AutoApproved     int            `json:"auto_approved"`
}

// Run re-derives all analyses, rules, evidence and statistics from the raw
// comments in the database. It is idempotent and deterministic. User decisions
// (status, edits, manual/excluded evidence, merges) are preserved.
func Run(ctx context.Context, d *db.DB, settings db.Settings) (Summary, error) {
	comments, err := loadComments(ctx, d)
	if err != nil {
		return Summary{}, err
	}
	anchors, err := loadAnchors(ctx, d)
	if err != nil {
		return Summary{}, err
	}
	res := Analyze(comments, Options{IgnoredAuthors: settings.IgnoredAuthors, Anchors: anchors})

	sum := Summary{CommentsAnalyzed: len(res.Analyses), ByKind: map[string]int{}}
	for _, a := range res.Analyses {
		sum.ByKind[a.Kind]++
	}

	err = d.InTx(ctx, func(tx *sql.Tx) error {
		now := d.NowTS()
		if _, err := tx.ExecContext(ctx, `DELETE FROM comment_analysis`); err != nil {
			return err
		}
		ins, err := tx.PrepareContext(ctx, `INSERT INTO comment_analysis(comment_id, kind, category, subcategory, signals, normalized, extractor_version) VALUES (?,?,?,?,?,?,?)`)
		if err != nil {
			return err
		}
		for _, a := range res.Analyses {
			if _, err := ins.ExecContext(ctx, a.CommentID, a.Kind, a.Category, a.Subcategory, strings.Join(a.Signals, ","), a.Normalized, Version); err != nil {
				ins.Close()
				return err
			}
		}
		ins.Close()

		if _, err := tx.ExecContext(ctx, `DELETE FROM rule_evidence WHERE link_source = 'auto'`); err != nil {
			return err
		}
		for _, r := range res.Rules {
			var id int64
			var edited bool
			err := tx.QueryRowContext(ctx, `SELECT id, edited FROM rules WHERE rule_key = ?`, r.Key).Scan(&id, &edited)
			switch {
			case errors.Is(err, sql.ErrNoRows):
				out, err := tx.ExecContext(ctx, `INSERT INTO rules(rule_key, origin, category, subcategory, title, guidance, severity, status, created_at, updated_at)
					VALUES (?,?,?,?,?,?,?, 'pending', ?, ?)`, r.Key, r.Origin, r.Category, r.Subcategory, r.Title, r.Guidance, r.Severity, now, now)
				if err != nil {
					return err
				}
				id, _ = out.LastInsertId()
				sum.RulesAdded++
			case err != nil:
				return err
			case !edited:
				// Template text may evolve with the extractor version.
				if _, err := tx.ExecContext(ctx, `UPDATE rules SET category=?, subcategory=?, title=?, guidance=?,
					updated_at = CASE WHEN category<>? OR subcategory<>? OR title<>? OR guidance<>? THEN ? ELSE updated_at END
					WHERE id=?`, r.Category, r.Subcategory, r.Title, r.Guidance,
					r.Category, r.Subcategory, r.Title, r.Guidance, now, id); err != nil {
					return err
				}
			}
			for _, ev := range r.Evidence {
				if _, err := tx.ExecContext(ctx, `INSERT OR IGNORE INTO rule_evidence(rule_id, comment_id, link_source, feedback_kind, score)
					VALUES (?,?, 'auto', ?, ?)`, id, ev.CommentID, ev.Kind, ev.Score); err != nil {
					return err
				}
			}
		}

		// Drop untouched auto-proposed rules that lost all evidence.
		out, err := tx.ExecContext(ctx, `DELETE FROM rules WHERE origin <> 'manual' AND status = 'pending' AND edited = 0
			AND NOT EXISTS (SELECT 1 FROM rule_evidence e WHERE e.rule_id = rules.id AND e.link_source IN ('auto','manual'))
			AND NOT EXISTS (SELECT 1 FROM rules child WHERE child.merged_into_id = rules.id)`)
		if err != nil {
			return err
		}
		n, _ := out.RowsAffected()
		sum.RulesRemoved = int(n)

		if err := recomputeStats(ctx, tx); err != nil {
			return err
		}
		if settings.AutoApproveMinPRs > 0 {
			out, err := tx.ExecContext(ctx, `UPDATE rules SET status='approved', status_changed_at=?,
				note = CASE WHEN note = '' THEN 'auto-approved: seen in ' || distinct_prs || ' PRs' ELSE note END
				WHERE status='pending' AND origin='signal' AND distinct_prs >= ?`, now, settings.AutoApproveMinPRs)
			if err != nil {
				return err
			}
			n, _ := out.RowsAffected()
			sum.AutoApproved = int(n)
		}
		return tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM rules`).Scan(&sum.RulesTotal)
	})
	return sum, err
}

func loadComments(ctx context.Context, d *db.DB) ([]Comment, error) {
	rows, err := d.QueryContext(ctx, `SELECT c.id, c.github_id, c.source, c.pr_id, c.author, c.is_bot, c.is_pr_author, c.body,
		COALESCE(c.path,''),
		COALESCE(c.review_state, (SELECT r.review_state FROM review_comments r WHERE r.source='review' AND r.github_id=c.review_id), ''),
		COALESCE(c.in_reply_to_id,0), COALESCE(c.thread_root_id,0), COALESCE(c.created_at,'')
		FROM review_comments c WHERE c.deleted_at IS NULL ORDER BY c.id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Comment
	for rows.Next() {
		var c Comment
		if err := rows.Scan(&c.ID, &c.GitHubID, &c.Source, &c.PRID, &c.Author, &c.IsBot, &c.IsPRAuthor, &c.Body, &c.Path,
			&c.ReviewState, &c.InReplyToID, &c.ThreadRootID, &c.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

func loadAnchors(ctx context.Context, d *db.DB) ([]int64, error) {
	rows, err := d.QueryContext(ctx, `SELECT rule_key FROM rules WHERE origin = 'candidate' ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []int64
	for rows.Next() {
		var k string
		if err := rows.Scan(&k); err != nil {
			return nil, err
		}
		if id, err := strconv.ParseInt(strings.TrimPrefix(k, "cand:c"), 10, 64); err == nil {
			out = append(out, id)
		}
	}
	return out, rows.Err()
}

type evRow struct {
	ruleID, commentID, prID int64
	kind, author, created   string
	codeChanged, reviewSt   string
}

type ruleRow struct {
	id, mergedInto int64
	key, origin    string
	severity       string
	edited         bool
}

// recomputeStats derives occurrence counts, scope, confidence and severity
// for every rule. Evidence of rules merged into another counts for the target.
func recomputeStats(ctx context.Context, tx *sql.Tx) error {
	var rules []ruleRow
	rows, err := tx.QueryContext(ctx, `SELECT id, COALESCE(merged_into_id,0), rule_key, origin, severity, edited FROM rules ORDER BY id`)
	if err != nil {
		return err
	}
	for rows.Next() {
		var r ruleRow
		if err := rows.Scan(&r.id, &r.mergedInto, &r.key, &r.origin, &r.severity, &r.edited); err != nil {
			rows.Close()
			return err
		}
		rules = append(rules, r)
	}
	rows.Close()

	byID := map[int64]ruleRow{}
	for _, r := range rules {
		byID[r.id] = r
	}
	target := func(id int64) int64 {
		for depth := 0; depth < 8; depth++ {
			r, ok := byID[id]
			if !ok || r.mergedInto == 0 || r.mergedInto == id {
				return id
			}
			id = r.mergedInto
		}
		return id
	}

	evRows, err := tx.QueryContext(ctx, `SELECT e.rule_id, e.comment_id, c.pr_id, e.feedback_kind, c.author, COALESCE(c.created_at,''),
		c.code_changed, COALESCE(c.review_state, (SELECT r.review_state FROM review_comments r WHERE r.source='review' AND r.github_id=c.review_id), '')
		FROM rule_evidence e JOIN review_comments c ON c.id = e.comment_id
		WHERE e.link_source IN ('auto','manual') AND c.deleted_at IS NULL ORDER BY e.rule_id, e.comment_id`)
	if err != nil {
		return err
	}
	agg := map[int64][]evRow{}
	for evRows.Next() {
		var e evRow
		if err := evRows.Scan(&e.ruleID, &e.commentID, &e.prID, &e.kind, &e.author, &e.created, &e.codeChanged, &e.reviewSt); err != nil {
			evRows.Close()
			return err
		}
		agg[e.ruleID] = append(agg[e.ruleID], e)
		if t := target(e.ruleID); t != e.ruleID {
			agg[t] = append(agg[t], e)
		}
	}
	evRows.Close()

	upd, err := tx.PrepareContext(ctx, `UPDATE rules SET occurrences=?, distinct_prs=?, distinct_reviewers=?, requirement_ratio=?,
		addressed_ratio=?, confidence=?, scope=?, severity=?, first_seen_at=?, last_seen_at=? WHERE id=?`)
	if err != nil {
		return err
	}
	defer upd.Close()

	for _, r := range rules {
		evs := agg[r.id]
		comments := map[int64]bool{}
		prs := map[int64]bool{}
		reviewers := map[string]bool{}
		req, addressed, changesReq := 0, 0, false
		var times []string
		for _, e := range evs {
			if comments[e.commentID] {
				continue
			}
			comments[e.commentID] = true
			prs[e.prID] = true
			reviewers[strings.ToLower(e.author)] = true
			if e.kind == KindRequirement || e.kind == "" {
				req++
			}
			if e.codeChanged == "yes" {
				addressed++
			}
			if e.reviewSt == "CHANGES_REQUESTED" {
				changesReq = true
			}
			if e.created != "" {
				times = append(times, e.created)
			}
		}
		n := len(comments)
		var reqRatio, addrRatio float64
		if n > 0 {
			reqRatio = round2(float64(req) / float64(n))
			addrRatio = round2(float64(addressed) / float64(n))
		}
		scope := "context"
		if len(prs) >= 2 {
			scope = "general"
		}
		severity := r.severity
		if !r.edited && r.origin == "signal" {
			if base, ok := SignalByID(strings.TrimPrefix(r.key, "sig:")); ok {
				severity = base.Severity
			}
			if changesReq && reqRatio >= 0.5 {
				severity = "high"
			}
		}
		sort.Strings(times)
		first, last := "", ""
		if len(times) > 0 {
			first, last = times[0], times[len(times)-1]
		}
		if _, err := upd.ExecContext(ctx, n, len(prs), len(reviewers), reqRatio, addrRatio,
			Confidence(len(prs), len(reviewers), reqRatio, addrRatio), scope, severity,
			db.NullStr(first), db.NullStr(last), r.id); err != nil {
			return err
		}
	}
	return nil
}

// Confidence is a deterministic 0..1 score: how strongly the history supports
// a rule. It grows with the number of distinct PRs and reviewers, the share of
// explicit requirements, and the share of feedback that led to a code change.
//
//	0.30 base
//	+0.15 per additional distinct PR (max +0.45)
//	+0.10 if two or more reviewers raised it
//	+0.10 × requirement ratio
//	+0.05 × addressed ratio
func Confidence(prs, reviewers int, reqRatio, addrRatio float64) float64 {
	if prs == 0 {
		return 0
	}
	c := 0.30
	extra := prs - 1
	if extra > 3 {
		extra = 3
	}
	c += 0.15 * float64(extra)
	if reviewers >= 2 {
		c += 0.10
	}
	c += 0.10*reqRatio + 0.05*addrRatio
	if c > 1 {
		c = 1
	}
	return round2(c)
}
