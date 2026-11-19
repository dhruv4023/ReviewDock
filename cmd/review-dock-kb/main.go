// review-dock-kb is the headless entry point of the review-dock knowledge
// system: GitHub review-comment sync, deterministic rule extraction and
// zero-token skill generation. Intended for system cron / systemd timers;
// shares the database and lock with the desktop app.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"text/tabwriter"

	"review-dock/backend/knowledge/db"
	"review-dock/backend/knowledge/pipeline"
	"review-dock/backend/knowledge/skillgen"
	"review-dock/backend/storage"
)

const usage = `review-dock-kb — review knowledge sync and zero-token skill generation

Usage: review-dock-kb [--data-dir DIR] [--json] <command> [args]

Pipeline
  run [--repo owner/name]... [--max-prs N] [--no-build]
                         sync GitHub → extract rules → build & publish skill
  sync [--repo owner/name]... [--max-prs N]
                         sync + extract only (no skill build)
  extract                re-run deterministic extraction on stored comments
  build [--dry-run]      regenerate the skill from the DB (no network, no AI)
  preview                show the diff between the DB state and the published skill
  status                 overview, last run and last build

Repositories
  repos list
  repos add owner/name [--author LOGIN|@me]
  repos import           register repositories tracked by the desktop app
  repos enable|disable|reset|remove owner/name

Rules
  rules list [--status S] [--category C] [--min N]
  rules approve|reject|supersede|reopen ID...
  rules merge TARGET_ID SOURCE_ID...

Skill & verification
  skill path | version | verify
  zero-token             report proving skill generation uses no LLM / network

Exit codes: 0 ok, 1 error, 3 another sync holds the lock.
`

func main() {
	os.Exit(run(os.Args[1:]))
}

func run(args []string) int {
	fs := flag.NewFlagSet("review-dock-kb", flag.ContinueOnError)
	dataDir := fs.String("data-dir", "", "data directory (default: the desktop app's config dir)")
	asJSON := fs.Bool("json", false, "machine-readable output")
	fs.Usage = func() { fmt.Fprint(os.Stderr, usage) }
	if err := fs.Parse(args); err != nil {
		return 1
	}
	if fs.NArg() == 0 {
		fs.Usage()
		return 1
	}
	if *dataDir == "" {
		store, err := storage.NewService("review-dock")
		if err != nil {
			return fail(err)
		}
		*dataDir = store.DataDir()
	}

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	log := pipeline.NewLogger(*dataDir, os.Stderr)
	p, err := pipeline.Open(*dataDir, log)
	if err != nil {
		return fail(err)
	}
	defer p.DB.Close()

	out := func(v any, text func()) int {
		if *asJSON {
			enc := json.NewEncoder(os.Stdout)
			enc.SetIndent("", "  ")
			enc.Encode(v)
		} else {
			text()
		}
		return 0
	}

	cmd, rest := fs.Arg(0), fs.Args()[1:]
	switch cmd {
	case "run", "sync":
		sub := flag.NewFlagSet(cmd, flag.ContinueOnError)
		var repos multi
		sub.Var(&repos, "repo", "limit to owner/name (repeatable)")
		maxPRs := sub.Int("max-prs", 0, "max PRs per repository in this run (0 = unlimited)")
		noBuild := sub.Bool("no-build", cmd == "sync", "skip skill build")
		if err := sub.Parse(rest); err != nil {
			return 1
		}
		registerTracked(ctx, p)
		opts := pipeline.Options{Trigger: "cli", MaxPRs: *maxPRs, NoBuild: *noBuild}
		for _, name := range repos {
			r, err := p.DB.GetRepositoryByName(ctx, name)
			if err != nil {
				return fail(fmt.Errorf("unknown repository %s (add it with: repos add %s)", name, name))
			}
			opts.RepoIDs = append(opts.RepoIDs, r.ID)
		}
		res, err := p.Run(ctx, opts)
		if errors.Is(err, pipeline.ErrBusy) {
			fmt.Fprintln(os.Stderr, err)
			return 3
		}
		if err != nil {
			return fail(err)
		}
		code := out(res, func() {
			fmt.Printf("run #%d: %s — %d repos (%d failed), %d PRs, %d comments changed, %d deleted, %d rules\n",
				res.Run.ID, res.Run.Status, res.Run.ReposTotal, res.Run.ReposFailed, res.Run.PRsSynced,
				res.Run.CommentsUpserted, res.Run.CommentsDeleted, res.Run.RulesTotal)
			if res.Build != nil {
				fmt.Printf("skill: %s version %s (%d rules)\n", res.Build.Build.Status, short(res.Build.Build.ContentHash), res.Build.Build.RuleCount)
			}
			if res.Run.Error != "" {
				fmt.Println("errors:", res.Run.Error)
			}
		})
		if res.Run.Status == "error" {
			return 1
		}
		return code

	case "extract":
		sum, err := p.Extract(ctx)
		if errors.Is(err, pipeline.ErrBusy) {
			fmt.Fprintln(os.Stderr, err)
			return 3
		}
		if err != nil {
			return fail(err)
		}
		return out(sum, func() {
			fmt.Printf("analysed %d comments → %d rules (+%d, -%d, auto-approved %d)\n", sum.CommentsAnalyzed, sum.RulesTotal,
				sum.RulesAdded, sum.RulesRemoved, sum.AutoApproved)
			fmt.Println("by kind:", sortedMap(sum.ByKind))
		})

	case "build":
		sub := flag.NewFlagSet("build", flag.ContinueOnError)
		dry := sub.Bool("dry-run", false, "render and validate without publishing")
		if err := sub.Parse(rest); err != nil {
			return 1
		}
		br, err := p.Build(ctx, !*dry)
		if errors.Is(err, pipeline.ErrBusy) {
			fmt.Fprintln(os.Stderr, err)
			return 3
		}
		if err != nil {
			return fail(err)
		}
		return out(br, func() {
			b := br.Build
			fmt.Printf("%s: version %s, %d rules, %d files, %d bytes, network calls %d, LLM calls %d\n",
				b.Status, short(b.ContentHash), b.RuleCount, b.FileCount, b.TotalBytes, b.NetworkCalls, b.LLMCalls)
			if br.Publish != nil {
				fmt.Println("current:", br.Publish.Current, "→", br.Publish.VersionDir)
				for _, t := range br.Publish.Targets {
					fmt.Println("agent link:", t)
				}
				for _, w := range br.Publish.Warnings {
					fmt.Println("warning:", w)
				}
			}
		})

	case "preview":
		pv, err := p.Preview(ctx)
		if err != nil {
			return fail(err)
		}
		return out(pv, func() {
			fmt.Printf("current %s → new %s (changed: %v, %d rules)\n", orNone(pv.CurrentVersion), pv.Version, pv.Changed, pv.RuleCount)
			for _, e := range pv.Errors {
				fmt.Println("validation:", e)
			}
			fmt.Print(pv.Diff)
		})

	case "status":
		ov, err := p.DB.GetOverview(ctx)
		if err != nil {
			return fail(err)
		}
		repos, _ := p.DB.ListRepositories(ctx)
		return out(map[string]any{"overview": ov, "repositories": repos, "skill_version": skillgen.CurrentVersion(p.SkillBase())}, func() {
			fmt.Printf("database: %s (schema v%d)\n", ov.DBPath, ov.SchemaVersion)
			fmt.Printf("PRs %d · comments %d (reviewer %d) · rules %d %v\n", ov.PRs, ov.Comments, ov.ReviewerComms, ov.Rules, counts(ov.RulesByStatus))
			fmt.Printf("skill: %s at %s\n", orNone(skillgen.CurrentVersion(p.SkillBase())), filepath.Join(p.SkillBase(), "current"))
			if ov.LastRun != nil {
				fmt.Printf("last run #%d %s (%s, started %s)\n", ov.LastRun.ID, ov.LastRun.Status, ov.LastRun.Trigger, ov.LastRun.StartedAt)
			}
			printRepos(repos)
		})

	case "repos":
		return reposCmd(ctx, p, rest, out)

	case "rules":
		return rulesCmd(ctx, p, rest, out)

	case "skill":
		if len(rest) == 0 {
			return fail(errors.New("skill: expected path | version | verify"))
		}
		switch rest[0] {
		case "path":
			fmt.Println(filepath.Join(p.SkillBase(), "current"))
		case "version":
			fmt.Println(orNone(skillgen.CurrentVersion(p.SkillBase())))
		case "verify":
			pv, err := p.Preview(ctx)
			if err != nil {
				return fail(err)
			}
			if len(pv.Errors) > 0 {
				for _, e := range pv.Errors {
					fmt.Println("invalid:", e)
				}
				return 1
			}
			if pv.Changed {
				fmt.Printf("published %s is stale: database renders %s (run `review-dock-kb build`)\n", orNone(pv.CurrentVersion), pv.Version)
				return 1
			}
			fmt.Printf("ok: published skill %s matches the database byte-for-byte\n", pv.Version)
		default:
			return fail(fmt.Errorf("skill: unknown subcommand %q", rest[0]))
		}
		return 0

	case "zero-token":
		last, _ := p.DB.LastBuild(ctx, "")
		r := pipeline.CheckZeroToken(last)
		code := out(r, func() {
			fmt.Println(r.Statement)
			fmt.Printf("linked Go modules: %d, forbidden AI/LLM modules linked: %d %v\n", r.LinkedModules, len(r.ForbiddenLinked), r.ForbiddenLinked)
			fmt.Printf("last build: network calls %d, LLM calls %d\n", r.LastBuildNetwork, r.LastBuildLLM)
		})
		if len(r.ForbiddenLinked) > 0 || r.LastBuildNetwork > 0 || r.LastBuildLLM > 0 {
			return 1
		}
		return code
	}
	fs.Usage()
	return 1
}

func reposCmd(ctx context.Context, p *pipeline.Pipeline, args []string, out func(any, func()) int) int {
	if len(args) == 0 || args[0] == "list" {
		repos, err := p.DB.ListRepositories(ctx)
		if err != nil {
			return fail(err)
		}
		return out(repos, func() { printRepos(repos) })
	}
	switch args[0] {
	case "import":
		n := registerTracked(ctx, p)
		fmt.Printf("registered %d new repositories from the desktop app\n", n)
		return 0
	case "add":
		sub := flag.NewFlagSet("add", flag.ContinueOnError)
		author := sub.String("author", "@me", "PR author to collect reviews for")
		if len(args) < 2 {
			return fail(errors.New("repos add owner/name"))
		}
		if err := sub.Parse(args[2:]); err != nil {
			return 1
		}
		owner, name, ok := strings.Cut(args[1], "/")
		if !ok {
			return fail(errors.New("expected owner/name"))
		}
		r, err := p.DB.EnsureRepository(ctx, owner, name)
		if err != nil {
			return fail(err)
		}
		if err := p.DB.UpdateRepositoryConfig(ctx, r.ID, true, *author); err != nil {
			return fail(err)
		}
		fmt.Printf("tracking %s (author %s)\n", r.FullName, *author)
		return 0
	case "enable", "disable", "reset", "remove":
		if len(args) < 2 {
			return fail(fmt.Errorf("repos %s owner/name", args[0]))
		}
		r, err := p.DB.GetRepositoryByName(ctx, args[1])
		if err != nil {
			return fail(fmt.Errorf("unknown repository %s", args[1]))
		}
		switch args[0] {
		case "enable", "disable":
			err = p.DB.UpdateRepositoryConfig(ctx, r.ID, args[0] == "enable", r.AuthorFilter)
		case "reset":
			err = p.DB.ResetRepositoryCheckpoint(ctx, r.ID)
		case "remove":
			err = p.DB.DeleteRepository(ctx, r.ID)
		}
		if err != nil {
			return fail(err)
		}
		fmt.Printf("%s: %s\n", r.FullName, args[0])
		return 0
	}
	return fail(fmt.Errorf("repos: unknown subcommand %q", args[0]))
}

func rulesCmd(ctx context.Context, p *pipeline.Pipeline, args []string, out func(any, func()) int) int {
	if len(args) == 0 || args[0] == "list" {
		sub := flag.NewFlagSet("list", flag.ContinueOnError)
		status := sub.String("status", "", "pending|approved|rejected|superseded|merged")
		cat := sub.String("category", "", "category id")
		min := sub.Int("min", 0, "minimum occurrences")
		if len(args) > 0 {
			if err := sub.Parse(args[1:]); err != nil {
				return 1
			}
		}
		rules, err := p.DB.ListRules(ctx, db.RuleFilter{Status: *status, Category: *cat, MinOcc: *min, Limit: 2000})
		if err != nil {
			return fail(err)
		}
		return out(rules, func() {
			w := tabwriter.NewWriter(os.Stdout, 0, 2, 2, ' ', 0)
			fmt.Fprintln(w, "ID\tSTATUS\tCATEGORY\tOCC\tPRS\tCONF\tTITLE")
			for _, r := range rules {
				fmt.Fprintf(w, "%s\t%s\t%s\t%d\t%d\t%.2f\t%s\n", r.Code, r.Status, r.Category, r.Occurrences, r.DistinctPRs, r.Confidence, trunc(r.Title, 90))
			}
			w.Flush()
		})
	}
	ids, err := parseIDs(args[1:])
	if err != nil {
		return fail(err)
	}
	switch args[0] {
	case "approve", "reject", "supersede", "reopen":
		status := map[string]string{"approve": db.StatusApproved, "reject": db.StatusRejected,
			"supersede": db.StatusSuperseded, "reopen": db.StatusPending}[args[0]]
		for _, id := range ids {
			if err := p.DB.SetRuleStatus(ctx, id, status, ""); err != nil {
				return fail(fmt.Errorf("%s: %w", db.RuleCode(id), err))
			}
			fmt.Printf("%s → %s\n", db.RuleCode(id), status)
		}
		return 0
	case "merge":
		if len(ids) < 2 {
			return fail(errors.New("rules merge TARGET_ID SOURCE_ID..."))
		}
		if err := p.DB.MergeRules(ctx, ids[0], ids[1:]); err != nil {
			return fail(err)
		}
		fmt.Printf("merged %d rule(s) into %s; run `review-dock-kb extract` to refresh statistics\n", len(ids)-1, db.RuleCode(ids[0]))
		return 0
	}
	return fail(fmt.Errorf("rules: unknown subcommand %q", args[0]))
}

func registerTracked(ctx context.Context, p *pipeline.Pipeline) int {
	store, err := storage.NewService("review-dock")
	if err != nil {
		return 0
	}
	repos, err := store.ReadRepos()
	if err != nil {
		return 0
	}
	n, _ := p.RegisterTracked(ctx, repos)
	return n
}

func parseIDs(args []string) ([]int64, error) {
	var ids []int64
	for _, a := range args {
		a = strings.TrimPrefix(strings.ToUpper(a), "R-")
		id, err := strconv.ParseInt(a, 10, 64)
		if err != nil {
			return nil, fmt.Errorf("invalid rule id %q", a)
		}
		ids = append(ids, id)
	}
	if len(ids) == 0 {
		return nil, errors.New("expected rule ids")
	}
	return ids, nil
}

func printRepos(repos []db.Repository) {
	w := tabwriter.NewWriter(os.Stdout, 0, 2, 2, ' ', 0)
	fmt.Fprintln(w, "REPOSITORY\tENABLED\tAUTHOR\tSTATUS\tPRS\tCOMMENTS\tCHECKPOINT\tLAST SUCCESS\tERROR")
	for _, r := range repos {
		fmt.Fprintf(w, "%s\t%v\t%s\t%s\t%d\t%d\t%s\t%s\t%s\n", r.FullName, r.Enabled, r.AuthorFilter, r.SyncStatus, r.PRCount,
			r.CommentCount, orNone(r.CheckpointUpdatedAt), orNone(r.LastSuccessAt), trunc(r.LastError, 60))
	}
	w.Flush()
}

type multi []string

func (m *multi) String() string     { return strings.Join(*m, ",") }
func (m *multi) Set(v string) error { *m = append(*m, v); return nil }

func fail(err error) int {
	fmt.Fprintln(os.Stderr, "error:", err)
	return 1
}

func short(h string) string {
	if len(h) > 12 {
		return h[:12]
	}
	return orNone(h)
}

func orNone(s string) string {
	if s == "" {
		return "-"
	}
	return s
}

func trunc(s string, n int) string {
	if len(s) > n {
		return s[:n-1] + "…"
	}
	return s
}

func counts(items []db.CountItem) string {
	var parts []string
	for _, c := range items {
		parts = append(parts, fmt.Sprintf("%s=%d", c.Key, c.Count))
	}
	return strings.Join(parts, " ")
}

func sortedMap(m map[string]int) string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	for i := 1; i < len(keys); i++ {
		for j := i; j > 0 && keys[j] < keys[j-1]; j-- {
			keys[j], keys[j-1] = keys[j-1], keys[j]
		}
	}
	var parts []string
	for _, k := range keys {
		parts = append(parts, fmt.Sprintf("%s=%d", k, m[k]))
	}
	return strings.Join(parts, " ")
}
