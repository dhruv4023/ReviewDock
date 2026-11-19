# review-dock

A Linux desktop app for managing **your own GitHub pull requests** across
several local Odoo checkouts. It:

* lists your PRs with CI, mergeability and ahead/behind state;
* rebases them in parallel and force-pushes, by hand or on a schedule;
* builds AI-review prompts from a diff and a template;
* shows review statistics;
* learns from the review comments you received. It extracts rules from them
  **deterministically** and publishes those rules as an agent skill
  (`~/.claude/skills/odoo-pr-review`) that coding agents read before
  reviewing your next PR.

All GitHub access goes through the `gh` CLI. The app never stores or handles a
GitHub token.

---

## Contents

1. [Quick start](#1-quick-start)
2. [System overview](#2-system-overview)
3. [Repository layout](#3-repository-layout)
4. [Backend components](#4-backend-components)
5. [Frontend](#5-frontend)
6. [Main flows](#6-main-flows)
7. [Persistence](#7-persistence)
8. [Concurrency and scheduling](#8-concurrency-and-scheduling)
9. [Security model](#9-security-model)
10. [Testing](#10-testing)
11. [Further documentation](#11-further-documentation)

---

## 1. Quick start

**Prerequisites:** Go ≥ 1.25, Node ≥ 18, the Wails CLI v2
(`go install github.com/wailsapp/wails/v2/cmd/wails@latest`), `git`, `gh`, and
the `webkit2gtk-4.1` dev package.

```bash
gh auth login                           # the only credential store the app uses

wails dev -tags webkit2_41              # dev mode, live reload
./build.sh                              # release build → ~/reviewdock
```

Everything is in this single app. In **Review Knowledge → Sync**, click
**Sync now** for the first (full) import. Optionally add scheduled sync times
there, the same way you would for the scheduled rebase. Like the rebase
schedule, it runs only while the app is open; there is no background service
or system cron.

---

## 2. System overview

The program has two subsystems. They run in the same binary but don't share
state.

| Subsystem | Purpose | State |
|---|---|---|
| **PR / rebase** (original) | List PRs, rebase/push queue, diff and review prompt, stats dashboard, scheduled rebase | JSON files in `~/.config/review-dock/` |
| **Review knowledge** (`backend/knowledge`) | Collect review comments, extract rules, generate and publish the agent skill | SQLite `~/.config/review-dock/knowledge.db` |

The only place they meet is the `{{REVIEW_RULES}}` placeholder in the review
prompt template. It is filled from the published skill.

```
┌──────────────────────────── review-dock (Wails v2 binary) ─────────────────────────────┐
│                                                                                        │
│  React UI (WebView)                                                                    │
│   PRs │ Dashboard │ Review Knowledge          ◄── events: terminal:log, rebase:status, │
│      │ window.go.main.App.*  (IPC bindings)        kb:progress, kb:done                │
│      ▼                                                                                 │
│  app.go  (PR/rebase bindings)            app_knowledge.go  (KB* bindings)              │
│   │  │  │  │                                     │                                     │
│   │  │  │  └─ stats ─────┐                       ▼                                     │
│   │  │  └──── queue ─► git ─► git CLI      knowledge/pipeline ◄── 55 s tick (in-app)   │
│   │  └─────── github ──┴─────► gh CLI ◄──── ghsync  │ extract │ skillgen               │
│   └────────── storage (JSON)                    └────┴────┬────┘                       │
│                                                       knowledge/db (SQLite)            │
└────────────────────────────────────────────────────────────────────────────────────────┘
                                                    │
                       ~/.config/review-dock/knowledge/skill/current ◄── ~/.claude/skills/odoo-pr-review
```

**Design principles**

* **Delegate to the tools.** `git` and `gh` do the real work. The app adds
  orchestration, concurrency control and UI.
* **Deterministic knowledge.** Building the skill is a pure function of the
  database, so the same data gives byte-identical files. It uses no LLM, no
  embeddings and no network, and tests enforce this (see §10).
* **Resumable and idempotent.** Every write is atomic, either a transaction
  or a temp-file-then-rename. An interrupted sync or publish leaves the data
  consistent.

---

## 3. Repository layout

```
main.go                     Wails bootstrap (window, embedded frontend/dist, bindings)
app.go                      App struct, lifecycle, PR/rebase/stats bindings, scheduler tick
app_knowledge.go            Review-knowledge bindings (KB*), knowledge lifecycle + schedule check
logger/                     logrus + lumberjack → ~/.config/review-dock/logs/review-dock.log

backend/
  models/                   Shared DTOs (Repository, PullRequest, Settings, Session…)
  storage/                  JSON persistence with flock + atomic writes
  github/                   gh CLI wrapper: auth/session, PR listing, CI status
  git/                      git CLI executor: per-repo mutexes, streamed output
  queue/                    Parallel rebase/push worker pool with branch groups (+ tests)
  stats/                    Per-PR review statistics, dashboard aggregation, cache
  knowledge/
    db/                     SQLite schema, migrations, stores (sync, rules, skill builds, settings)
    ghsync/                 GitHub collector: search, pagination, GraphQL threads, retry/rate limit
    extract/                Deterministic classifier, taxonomy, clustering, rule upsert
    skillgen/               Pure skill generator, validator, atomic versioned publisher (stdlib only)
    pipeline/               Orchestration: lock → sync → extract → build, schedule match, network guard
    kbtest/                 Fake `gh` and Odoo-flavoured fixtures for tests
    zerotoken_test.go       Import-graph audit (no network/LLM in the build path)

frontend/
  src/App.tsx               Shell: sidebar, main tabs, details panel, terminal
  src/stores/               appStore (PRs, queue, settings), knowledgeStore (KB)
  src/components/           PRTable, Dashboard, DetailsPanel, SettingsDialog, TerminalPanel…
  src/components/knowledge/ Overview, Comments, Rules, Sync, Skill tabs
  wailsjs/                  Generated binding stubs (`wails generate module`)

docs/                       AUDIT.md, KNOWLEDGE_SYSTEM.md
build.sh                    Release build of the app
```

---

## 4. Backend components

### 4.1 `App` (app.go, app_knowledge.go)

This is the Wails-bound object. Every exported method becomes a frontend call
(`window.go.main.App.<Method>`).

On `startup` it:

1. opens the storage service;
2. opens the stats cache;
3. starts the knowledge subsystem;
4. creates the queue manager with the configured concurrency;
5. starts the scheduler tick (scheduled rebase and scheduled knowledge sync);
6. checks the `git` version.

If storage fails, the error is logged and the app keeps running with reduced
functionality.

| Area | Bindings |
|---|---|
| Repositories | `GetRepositories`, `AddRepository`, `RemoveRepository`, `GetRemotes`, `SetBranchTracking` |
| Auth | `GetSession`, `LoginGitHub`, `Logout` (all via `gh auth`) |
| PRs | `GetPullRequests(remoteUpdate)`, `GetPRCIStatus`, `GetPRDiff` |
| Rebase | `RebasePRs`, `CancelRebase` |
| Review prompt | `GetReviewTemplate`, `SaveReviewTemplate`, `KBReviewRules` |
| Stats | `GetPRStats`, `GetDashboardStats` |
| Settings | `GetSettings`, `SaveSettings` |
| Knowledge | `KBOverview`, `KBRepositories`, `KBRunSync`, `KBRules`, `KBSetRuleStatus`, `KBEditRule`, `KBMergeRules`, `KBSearchComments`, `KBPreviewSkill`, `KBRebuildSkill`, … (see `app_knowledge.go`) |

Long-running work is sent back as **events**, not return values:

| Event | Payload | Emitted by |
|---|---|---|
| `terminal:log` | ANSI-coloured line for the xterm panel | queue, git, scheduler, knowledge |
| `rebase:status` | `{job_id, status, error, active_count, queued_count}` | queue |
| `oauth:device_code` / `oauth:success` / `oauth:error` | login progress | `LoginGitHub` |
| `kb:progress` | pipeline stage / repo / counters | knowledge pipeline |
| `kb:done` | run result | `KBRunSync` |

### 4.2 `github`: GitHub through `gh`

`Client.FetchPRs` runs `gh pr list --author @me --state all --json …` for each
repository. It combines the result with local ahead/behind counts from `git`.
`FetchCombinedCIStatus` reads the commit status and checks. Login runs
`gh auth login` and passes the device code to the UI through events.
`GetSession` returns only the user profile.

### 4.3 `git`: executor

The executor wraps `git` subprocesses. Each one streams its stdout and stderr
to a `LogWriter` and respects context cancellation.

It keeps **one mutex per repository path**, so two jobs never change the same
working tree at once. It provides fetch, checkout, rebase and abort,
timestamp amend, force-push, remote detection (by priority), diff, and
ahead/behind counts.

### 4.4 `queue`: rebase/push worker pool

Each job is one PR rebase:

```
pending ─► rebasing ─► rebased ─► pushing ─► success
                 └───────────┴──────────┴──► failed
```

* **Concurrency.** A semaphore sized by `Settings.ConcurrencyLimit` limits how
  many rebases run at once. Jobs in the same repository are also serialised by
  the git repo lock.
* **Rebase phase.**
  1. Require a clean working tree.
  2. Fetch.
  3. Pick a remote.
  4. Check out the branch.
  5. `git rebase <base>`. If it fails, run `git rebase --abort` with a fresh
     context so the tree is never left mid-rebase.
  6. Optionally amend the commit timestamp.
* **Branch groups.** PRs that share a head-branch name (the same feature
  forward-ported to several repos) form a group. The group is
  **all-or-nothing**: every member must rebase before any member pushes, and
  one failure cancels and fails the whole group.
* **Cancel.** Before the rebase starts, the job is marked to skip its push. If
  it is mid-rebase, its context is cancelled, which triggers the abort.
* **Dedup.** A job ID that is already queued is ignored.

### 4.5 Scheduled rebase (app.go)

A ticker fires every 55 s and compares the time against `Settings.CronTimes`,
at most once per minute. On a match it:

1. refreshes the PRs;
2. selects the **branches** where any PR has `AheadCount > 100`;
3. skips conflicting PRs and dirty working trees (the whole group);
4. includes drafts only if `CronIncludeDrafts` is set;
5. submits a batch with force-push enabled.

The same tick also calls `checkKnowledgeCron` (§4.8). Both schedules run only
while the app is open, and a missed time is not caught up later.

### 4.6 `stats`: review statistics

`ComputePRStats` runs `gh pr view --json comments,reviews,…` and computes
comment and reviewer counts and timings. `CacheService` keeps the results in
memory and persists them to `stats_cache.json` with atomic writes. A cache
entry is invalid once the PR's `updatedAt` changes. The dashboard aggregates
the cached per-PR stats.

### 4.7 `storage`: JSON persistence

Settings, repositories, the session profile and the review template are
stored in `~/.config/review-dock/`. A cross-process `flock` and an in-process
`RWMutex` protect access. Writes go to a temp file, which is fsynced and then
renamed (`WriteFileAtomic`).

### 4.8 `knowledge`: the review knowledge system

The full design is in [`docs/KNOWLEDGE_SYSTEM.md`](docs/KNOWLEDGE_SYSTEM.md).
In short:

```
gh api ─► ghsync ─► db ─► extract ─► db(rules) ─► skillgen ─► validate ─► publish ─► ~/.claude/skills
 (network)                     (no network from here on; skillgen is stdlib-only)
```

| Package | Responsibility |
|---|---|
| `db` | SQLite (`modernc.org/sqlite`, no CGO) in WAL mode. Embedded forward-only migrations. Stores repositories, PRs, comments with their revisions, analyses, rules, rule evidence, sync runs, skill builds and settings. |
| `ghsync` | Searches your PRs with `updated >= checkpoint` in ascending order, windowed past GitHub's 1000-result search cap. Fetches inline comments, issue comments and reviews (paginated), plus review threads over GraphQL (resolved/outdated). Each PR is saved in **one transaction together with its checkpoint**, so syncs resume. Edits become revisions; disappeared comments are soft-deleted. Also handles exponential backoff, rate-limit preflight and token redaction. |
| `extract` | Pure `Analyze()`. Classifies each comment (requirement, suggestion, question, superseded, noise, unclear, self or bot) and matches a 49-signal taxonomy over 14 categories. Clusters uncategorised feedback that recurs across ≥2 PRs and detects threads where the reviewer conceded. `apply.go` upserts rules without overwriting manual edits, approvals, rejections, merges or evidence exclusions. |
| `skillgen` | `Generate(snapshot)` produces `SKILL.md`, `references/*.md`, `common-mistakes.md` and `metadata.json` with `text/template`. Sorting is deterministic, and the content hash is computed before the version is stamped in. `Validate()` runs before every publish. `Publish()` writes to a temp dir, renames it to `versions/<hash>`, atomically swaps the `current` symlink and prunes old versions. Identical output creates no new version. |
| `pipeline` | `Run()` takes the lock, recovers interrupted runs, then syncs, extracts and builds, recording every run. It also provides `Preview()`/diff, `ScheduledAt()` (schedule-time matching) and `WithNetworkGuard` (any HTTP during a build fails and is counted). |

Only rules you have **approved** that have at least one evidence link go into
the skill. Nothing is approved automatically unless you turn on
`auto_approve_min_prs`.

**Scheduled knowledge sync.** This follows the same model as the scheduled
rebase (§4.5). The knowledge settings hold `schedule_enabled` and
`schedule_times` (for example `["09:00 AM", "02:00 PM"]`); the schedule is off
by default.

On every 55 s tick, `checkKnowledgeCron` checks `pipeline.ScheduledAt(settings,
now)`. On a match, at most once per minute, it starts the same run as
**Sync now** (`KBRunSync(nil, "schedule")`). That run requires `gh` to be logged
in and is skipped if a run is already in progress.

Nothing runs while the app is closed. There is no separate binary, cron or
systemd unit.

---

## 5. Frontend

The frontend is React 18 + TypeScript, built with Vite and styled with
Tailwind. It uses Zustand for state and xterm.js for the log terminal.

* **`App.tsx`:** the layout is a sidebar (repositories and filters), a central
  tab area and a PR details panel, with the terminal panel at the bottom. The
  main tabs are **PRs** (`PRTable`), **Dashboard** and **Review Knowledge**
  (`KnowledgeView`).
* **`stores/appStore.ts`:** PRs, repositories, selection, queue status and
  settings. It subscribes to `terminal:log` and `rebase:status`.
* **`stores/knowledgeStore.ts`:** knowledge overview and run state. It
  subscribes to `kb:progress` and `kb:done`. The tabs re-fetch when its
  `version` counter changes.
* **`components/knowledge/`:**

  | Tab | What it shows and lets you do |
  |---|---|
  | Overview | Counts and the last run |
  | Comments | Search, view threads and revisions, turn a comment into a rule |
  | Rules | Filter; approve, reject, edit or merge rules; edit evidence |
  | Sync | Repositories, checkpoints, settings, run history, retry failed repos |
  | Skill | Current version, preview and diff, rebuild |

* **`reviewTemplate.ts`:** the default AI-review prompt and its placeholders,
  including `{{REVIEW_RULES}}`.

Calls go through the generated stubs in `frontend/wailsjs/go/main/App`.
Regenerate them with `wails generate module` after changing a binding
signature.

---

## 6. Main flows

**List PRs.** `GetPullRequests(remoteUpdate)` runs the following for each
repository: optional `git remote update`, then `gh pr list`, then local
ahead/behind counts. The CI status is fetched lazily for each row.

**Rebase.** The UI calls `RebasePRs([...])`. The app builds the jobs, and
`queue.SubmitBatch` registers the branch groups and starts one goroutine per
job. Progress arrives as `rebase:status` and `terminal:log` events, and
`CancelRebase(jobID)` stops a job.

**Review prompt.**
1. The details panel calls `GetPRDiff`, `GetReviewTemplate` and
   `KBReviewRules`.
2. It fills the placeholders.
3. It copies the result to the clipboard.

**Knowledge run.** A run is started by **Sync now** (`KBRunSync`) or at a
scheduled time while the app is open.
1. Take the lock.
2. Sync the GitHub data for each enabled repository.
3. Extract rules from the comments.
4. Generate the skill from a snapshot of the approved rules.
5. Validate it and publish it, or report it unchanged.
6. Record the run in `sync_runs` and the build in `skill_builds`.

**Agent consumption.** `~/.claude/skills/odoo-pr-review` points to
`knowledge/skill/current`, which points to `versions/<hash>`. Agents read the
skill files and never call back into the app.

---

## 7. Persistence

Everything lives under `~/.config/review-dock/`:

| Path | Owner | Format |
|---|---|---|
| `repos.json`, `settings.json`, `sessions.json`, `review_template.txt` | storage | JSON/text, atomic writes |
| `stats_cache.json` | stats | JSON, atomic writes |
| `knowledge.db` (+ `-wal`, `-shm`) | knowledge/db | SQLite, migrations in `backend/knowledge/db/migrations/` |
| `knowledge.lock` | pipeline | flock, one knowledge run at a time |
| `knowledge/skill/versions/<hash>/odoo-pr-review/`, `knowledge/skill/current` | skillgen | Generated skill, `current` symlink |
| `logs/review-dock.log` | logger | Text (logrus) |
| `logs/knowledge.log` | pipeline | JSON lines (slog), secrets redacted |

Schema changes are added as a new `NNNN_*.sql` file in `migrations/`. These
files are applied once each, in order, inside a transaction. Never edit a
migration that has already shipped.

---

## 8. Concurrency and scheduling

| Mechanism | Protects |
|---|---|
| Per-repo mutex (`git.Executor`) | One git operation per working tree |
| Rebase semaphore (`queue`) | Global rebase parallelism |
| Branch-group coordination (`queue`) | All-or-nothing push for PRs on the same branch |
| `flock` + RWMutex (`storage`) | JSON files across processes |
| `flock` on `knowledge.lock` + mutex (`pipeline`) | One knowledge run at a time, even with two app instances |
| SQLite WAL + per-PR transactions | Concurrent readers during sync, crash-safe checkpoints |

One in-app ticker (55 s) drives two independent schedules, and both work the
same way:

* each runs at configured wall-clock times, at most once per minute;
* each runs only while the app is open;
* neither has a background service or system cron.

| Schedule | Settings | Action |
|---|---|---|
| **Scheduled rebase** | `settings.json` → `cron_enabled`, `cron_times` | Rebase and force-push branches with a PR more than 100 commits ahead |
| **Scheduled knowledge sync** | knowledge settings → `schedule_enabled`, `schedule_times` | Sync → extract → build → publish skill. Skipped if a run is in progress |

---

## 9. Security model

* **Credentials.** `gh` owns the GitHub credential. The app never reads the
  token, persists it or sends it to the WebView; `Session.AccessToken` is
  tagged `json:"-"`.
* **Logs.** Knowledge logs and stored errors go through `ghsync.Redact`
  (`ghp_`, `gho_`, `github_pat_`, `Authorization`, `token=`).
* **Network boundary.** Only `github`, `stats` and `ghsync` start `gh`. `db`
  and `extract` have no direct network or exec imports. `skillgen` imports only
  the standard library, and no HTTP, TLS or RPC client is reachable from it.
  `backend/knowledge/zerotoken_test.go` enforces this boundary, and the Skill
  tab checks the running binary for AI SDK modules.
* **Filesystem safety.** Publishing never overwrites a real directory at the
  agent link. It warns instead.

---

## 10. Testing

```bash
cd frontend && npm ci && npm run build && cd ..   # main package embeds frontend/dist
go vet  -tags webkit2_41 ./...
go test -race -count=1 ./...
```

| Suite | Covers |
|---|---|
| `backend/queue` | Worker pool, branch groups, cancellation |
| `knowledge/db` | Migrations, upsert/dedup, revisions, soft delete, checkpoints, rule status/merge |
| `knowledge/ghsync` | Pagination, incremental/resume, retries, rate limit, redaction (fake `gh`) |
| `knowledge/extract` | Classifier, taxonomy, supersession, clustering, stability of manual edits |
| `knowledge/skillgen` | Determinism, validation failures, atomic publish, no churn, pruning |
| `knowledge/pipeline` | End-to-end run, locking, schedule-time matching, **`TestZeroTokenSkillGeneration`** |
| `knowledge/zerotoken_test.go` | Static import-graph audit |

The tests never touch the real GitHub, `~/.config` or `~/.claude`. They use
`kbtest.FakeGH` and temporary directories.

---

## 11. Further documentation

* [`docs/KNOWLEDGE_SYSTEM.md`](docs/KNOWLEDGE_SYSTEM.md): knowledge system
  schema, sync, extraction rules, skill format, agent integration, operation
  and settings.
* [`docs/AUDIT.md`](docs/AUDIT.md): technical audit (2026-11), problems found
  and what changed.
* [`architecture.md`](architecture.md): the original design blueprint, kept
  for history. It describes an OAuth/go-github design that was never built.
* [`walkthrough.md`](walkthrough.md): the original scaffolding notes.
