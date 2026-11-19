# review-dock — Technical Audit (2026-11-19)

## 1. Scope note

The brief names two projects, *review-dock* and *ReviewDock*, but both point to
`/home/odoo/Files/review-dock`. They are the same codebase. This audit
therefore treats "integration" as an **internal boundary** between:

* **ReviewDock desktop** — the existing Wails app (PR list, rebase queue,
  diff/prompt helpers, dashboard). It keeps all of its current responsibilities.
* **Knowledge subsystem** (new, `backend/knowledge/...`) — review-comment
  collection, deterministic rule extraction, and skill generation and publishing.
  It runs inside the desktop app *and* headless from a CLI (`cmd/review-dock-kb`)
  for system cron.

They share one SQLite database and one lock file. They never duplicate business
logic.

## 2. Existing architecture

| Layer | Tech | Notes |
|---|---|---|
| Shell | Wails v2.12 (Go 1.22 module, CI builds with Go 1.26) | `main.go`, `app.go` hold all bindings (≈800 lines) |
| Frontend | React 18 + Zustand + Tailwind, Vite | One store (`appStore.ts`), bindings called via `window.go.main.App.*` |
| GitHub | `gh` CLI subprocesses (`backend/github`, `backend/stats`) | Auth is fully delegated to `gh auth`; no tokens are stored by the app |
| Git | `git` subprocesses (`backend/git`) with per-repo mutexes | |
| Queue | Custom worker pool with branch-group coordination (`backend/queue`) | Has the only test suite in the repo |
| Persistence | JSON files in `~/.config/review-dock/` (`repos.json`, `settings.json`, `review_template.txt`, `stats_cache.json`) | flock + RWMutex |
| Scheduling | In-app 55 s ticker (`startCronScheduler`) that matches wall-clock times from settings, then rebases and force-pushes | Runs only while the app is open |
| Logging | logrus + lumberjack → `~/.config/review-dock/logs/review-dock.log` | Plain text, not structured fields |
| "Review" features | "Copy AI Review Prompt" builds a prompt from a user template + diff | No knowledge of past reviews |
| Stats | `gh pr view --json comments,reviews…` per PR, counts only, JSON cache | Comment **bodies are fetched and thrown away** |

## 3. Problems found

### Correctness and safety
1. **Access token leaked to the WebView.** `GetSession` copies `gh auth token`
   into `Session.AccessToken`, which is serialised to the frontend. The frontend
   never uses it. → *Fixed: the field is no longer populated or sent.*
2. **Non-atomic JSON writes.** `storage` and `stats/cache` truncate the file and
   then encode. A crash mid-write corrupts `repos.json`/`settings.json`. The stats
   cache comment says "atomically" but uses `os.WriteFile`. → *Fixed: write to a
   temp file, fsync, then rename.*
3. **Nil-pointer at startup.** If `storage.NewService` fails, `store.DataDir()`
   panics. → *Fixed: the app logs the error and continues degraded.*
4. **Stats cache is O(n²).** Every `Get`/`Set` re-reads and re-writes the whole
   JSON file. The dashboard loop calls it once per PR. → *Fixed: the cache is
   loaded once and kept in memory.*

### Architecture
5. **No review-knowledge store at all.** There's no database, no comment
   history, and no extraction. The stats engine already downloads every comment
   body and discards it.
6. **JSON files can't support the requirements** (dedup by GitHub ID,
   incremental checkpoints, transactions, indexes, search). → SQLite is added
   for the knowledge data. The existing small JSON settings stay as they are,
   because migrating them brings no benefit.
7. **The scheduler only exists while the GUI is open**, and it's hard-wired to
   rebasing. → The knowledge sync gets its own interval scheduler in-app, plus a
   headless CLI for system cron/systemd. Both share a cross-process lock.
8. **`architecture.md` is an outdated blueprint.** It describes an OAuth device
   flow, `sessions.json`, and go-github, none of which exist. The actual
   implementation uses `gh` CLI. → A note was added; new docs live in `docs/`.
9. **Repo lookups are duplicated** (`for _, r := range repos { if r.ID == … }`
   appears 6 times in `app.go`). Left mostly as is; new code uses a helper.

### Missing
10. No tests outside `queue`. No structured logs. No sync status or history. No
    rate-limit handling (every `gh` call either succeeds or fails).

## 4. Changes required → implemented

| Need | Implementation |
|---|---|
| Durable review DB | `backend/knowledge/db`: SQLite (pure-Go `modernc.org/sqlite`, no CGO, so the CLI cross-compiles), embedded versioned migrations, WAL, FKs |
| GitHub collection | `backend/knowledge/ghsync`: `gh api` wrapper with manual pagination, rate-limit preflight, exponential backoff, and token redaction. Covers inline comments, issue comments, reviews, and review threads (GraphQL: resolved/outdated) |
| Incremental + resumable | Per-repo `checkpoint_updated_at`. PRs are processed in ascending `updated_at`, one transaction per PR, and the checkpoint advances after each commit |
| Deterministic extraction | `backend/knowledge/extract`: normalisation, feedback-kind classifier, path+keyword categoriser, signal taxonomy with rule templates, Jaccard clustering for uncategorised feedback, thread-based supersession, deterministic statistics |
| Skill generator | `backend/knowledge/skillgen`: pure function from snapshot to files (`text/template`), validator, content hash, atomic versioned publish, and a `current` symlink |
| Orchestration | `backend/knowledge/pipeline`: lock → sync → extract → build → publish, with every run recorded |
| Headless/cron | `cmd/review-dock-kb` (`sync`, `extract`, `build`, `run`, `status`, `repos`, `rules`, `skill`) |
| UI | New **Knowledge** tab (Overview, Comments, Rules, Sync, Skill) in the existing style |
| Integration | The rebase app's "Copy AI Review Prompt" gains a `{{REVIEW_RULES}}` placeholder fed from the published skill. Agents read `~/.claude/skills/odoo-pr-review` (symlink to the current version) |

## 5. Proposed architecture

```
                    ┌──────────────── review-dock binary (Wails) ───────────────┐
 gh CLI ◄──────────┤ app.go (rebase/PR, unchanged)   app_knowledge.go (bindings)│
   ▲               │                                   │                         │
   │               │                     backend/knowledge/pipeline ◄── ticker   │
   │               └───────────────────────────────────┼─────────────────────────┘
   │                                                   │
   │   cmd/review-dock-kb (cron) ──────────────────────┤  (same packages, flock)
   │                                                   ▼
   └──── ghsync ──► db (SQLite: knowledge.db) ◄── extract   (NO network)
                                   │
                                   ▼
                               skillgen (NO network, NO exec, pure)
                                   │
              ~/.config/review-dock/knowledge/skill/versions/<hash>/odoo-pr-review
                                   ▲ current (symlink, atomic swap)
              ~/.claude/skills/odoo-pr-review  ──symlink──┘   ◄── AI agent
```

The **network boundary** is enforced by package imports. Only `ghsync` may run
`gh`. A test fails the build if `extract`, `skillgen`, or `db` ever import
`net`, `net/http`, `os/exec`, or any LLM/embedding SDK.

## 6. Migration requirements

* **Non-destructive.** Existing JSON files are untouched apart from atomic writes,
  which keep the same format. The new DB `~/.config/review-dock/knowledge.db` is
  created on first use, and migrations are applied forward-only inside a
  transaction (`schema_migrations`).
* **New Go dependency:** `modernc.org/sqlite` v1.34.5. It needs no CGO and is
  compatible with the existing `go 1.22` directive, which is unchanged.
* **First sync = full historical import** per repository (resumable). Afterwards
  each sync is incremental.
* **Nothing is auto-approved.** The skill includes only approved rules, so the
  first published skill has instructions and checklists but no historical rules
  until you approve some. The opt-in setting `auto_approve_min_prs` changes this.
