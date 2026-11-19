# Review Knowledge System

review-dock collects the review feedback on your pull requests, turns it into
structured engineering rules with **deterministic code**, and publishes an
AI-agent skill built from those rules. **No step of collection, extraction,
generation or publishing calls an LLM, an embedding API, or any AI service.**

* The **database** is the source of truth.
* The **skill** is a deterministic build artifact of the database.
* The **AI agent** only consumes the skill.

See `docs/AUDIT.md` for the audit of the existing app and the reasons behind this design.

---

## 1. Architecture

```
GitHub ──gh api──► ghsync ──► knowledge.db (SQLite) ──► extract ──► rules
                                                          │
                                         skillgen (templates) ◄─┘
                                                          │ validate
                         ~/.config/review-dock/knowledge/skill/versions/<hash>/odoo-pr-review
                                                          ▲ current (atomic symlink)
                         ~/.claude/skills/odoo-pr-review ─┘ (symlink → current)
```

| Package | Responsibility | Network |
|---|---|---|
| `backend/knowledge/db` | SQLite connection, embedded migrations, typed queries | none |
| `backend/knowledge/ghsync` | `gh api` client: pagination, retries, rate limits, redaction; per-repository incremental sync | **yes, via `gh` only** |
| `backend/knowledge/extract` | normalisation, feedback classification, taxonomy, clustering, statistics | none |
| `backend/knowledge/skillgen` | `text/template` rendering, validation, atomic versioned publishing | none (stdlib only) |
| `backend/knowledge/pipeline` | lock → recover → sync → extract → build → publish; schedule matching; HTTP guard | sync stage only |
| `app_knowledge.go` | Wails bindings for the desktop UI (**Review Knowledge** tab) | sync stage only |

Everything runs **inside the desktop app**. There is no separate CLI and no system cron or systemd job:
when review-dock is closed, nothing syncs. The database is `~/.config/review-dock/knowledge.db`, and the
lock `~/.config/review-dock/knowledge.lock` prevents two runs at once, even across two app windows.

### Integration boundary

The existing PR and rebase features are unchanged. They share only two things with the knowledge system:

1. **Repositories.** Every repository added in the sidebar is registered in the knowledge DB automatically. You can also add repositories without a local clone, on the Sync tab.
2. **The published skill.** The PR details panel's *Copy AI Review Prompt* supports a `{{REVIEW_RULES}}` placeholder, which is filled from the published skill's `metadata.json`. The prompt therefore uses the same artifact the agents read, not the database directly.

---

## 2. Database schema (migration `0001_init.sql`)

| Table | Purpose / key |
|---|---|
| `repositories` | `full_name` unique; `author_filter` (default `@me`); `checkpoint_updated_at` (incremental high-water mark); `full_import_done`; status, last success, last error, consecutive failures |
| `pull_requests` | **GitHub PR database id** as primary key; number, title, URL, author, state, merged, head SHA, timestamps |
| `review_comments` | unique **(`source`, `github_id`)**, where `source` is one of `inline` (diff comments and thread replies), `issue` (PR conversation) or `review` (submitted reviews). Stores the verbatim body, path, line, original line, commit SHAs, diff hunk (code context), thread root, `in_reply_to`, review state, resolved/outdated flags, `code_changed` (`yes`/`no`/`unknown`), `deleted_at` (soft delete), raw JSON, and first/last seen times |
| `comment_revisions` | previous bodies of edited comments. The original reviewer text is never lost |
| `comment_analysis` | per-comment kind, category, matched signals and normalised text (recomputed on each extraction) |
| `rules` | `rule_key` unique (`sig:<signal>`, `cand:c<comment>`, or `manual:…`); title, guidance, category, severity, scope, status, `merged_into_id`, `edited`, statistics |
| `rule_evidence` | (rule, comment) with `link_source`: `auto`, `manual`, or `excluded` (an exclusion persists across re-extraction) |
| `sync_runs`, `sync_run_repos` | run history with per-repository outcome and errors |
| `skill_builds` | every build or publish: content hash, rule count, network and LLM call counters, rule changes, errors |
| `kb_settings`, `schema_migrations` | configuration (JSON) and applied migrations |

Migrations are embedded in the binary and applied forward-only, each in its own transaction, the
first time the database is opened. Nothing in the existing JSON files is migrated or modified.

---

## 3. GitHub synchronization

* **Auth.** All calls go through `gh api`, so credentials stay inside `gh`. The app never reads or stores a token. (It used to send `gh auth token` to the WebView; this was fixed.) Error text is passed through `ghsync.Redact`, which masks anything that looks like a GitHub credential, before it is logged or stored.
* **Which PRs.** GitHub search `repo:<owner/name> is:pr author:<author_filter> updated:>=<checkpoint>`, sorted by `updated` ascending.
* **First run = full historical import.** A repository without a checkpoint is imported from the beginning. The search API caps results at 1000, so the import pages through in windows by moving the checkpoint forward.
* **Incremental.** Later runs only fetch PRs updated since the checkpoint. A PR whose stored `updated_at` equals the search result is skipped.
* **Per PR:** the PR itself, every page (100 per page) of inline comments, issue comments and reviews, plus one GraphQL call for review threads (`isResolved`, `isOutdated`). If the GraphQL call fails, the sync continues and keeps the previously known resolution state.
* **Transactions and resume.** Each PR is written in **one transaction** together with its checkpoint advance. An interrupted run loses at most the PR that was in progress, and the next run resumes from the last committed PR.
* **Edits and deletions.** A changed body keeps the old text in `comment_revisions`. Comments missing from a complete fetch of their PR get `deleted_at`, and are restored if they reappear.
* **Did the feedback change code?** If the thread is outdated (the commented lines changed afterwards), `code_changed` is `yes`. If the PR was merged and the line is still current, it is `no`. Otherwise it is `unknown`.
* **Rate limits.** Before each search page and each PR the client checks `/rate_limit`, which costs no quota. When the core quota drops below 50 the run stops with `rate_limited`, keeps its checkpoint, and marks the remaining repositories as `rate_limited`. Transient failures (5xx, 429, secondary limits, timeouts, connection errors) are retried with exponential backoff (2s, 4s, 8s, 16s, 32s, capped at 60s). Permanent errors such as 404 or bad credentials fail fast.
* **Concurrency.** An in-process mutex plus a `flock` on `knowledge.lock` let only one run happen at a time. A second attempt returns `ErrBusy` and is skipped.
* **Crash recovery.** Runs left in `running` by a dead process are marked `interrupted` the next time the lock is acquired.

---

## 4. Deterministic rule extraction (`backend/knowledge/extract`)

Extraction is fully re-derived from the stored comments on every run. Running it again on the same
data gives identical results, and the result does not depend on input order (this is tested).

1. **Normalise.** Drop quoted lines (`> …`), code blocks, URLs, HTML and mentions. Keep inline-code identifiers. Detect ```` ```suggestion ```` blocks.
2. **Classify the kind of feedback:**
   * `bot` / `self`: ignored authors, `[bot]` accounts, or the PR author.
   * `noise`: LGTM, thanks, `@robodoo r+`, empty approvals.
   * `superseded`: withdrawn by the reviewer ("never mind", "my bad", …), **or rejected in discussion**. That means the PR author pushed back in the thread ("it is needed because…") without saying it was fixed, and the same reviewer then conceded ("ok, fair enough").
   * `suggestion`: nit, maybe, consider, optional, why not…
   * `requirement`: please, should, don't, avoid, instead, a suggestion block, "could you…", or a concrete defect report (slow, crash, regression…).
   * `question` / `unclear`: everything else.

   **Only `requirement` and `suggestion` comments become rule evidence.**
3. **Categorise and match signals.** About 50 signals in `taxonomy.go`, grouped into 14 areas (performance, database & ORM, backend Python, frontend/OWL, POS, security, code reuse, testing, stable policy, views/XML, i18n, style, commits, general). Each signal has keyword/regex patterns, an optional file-path condition, a rule template and a check procedure. A comment yields its best signal plus at most one more from a *different* area. Generic catch-alls ("unnecessary", "readability") apply only when nothing specific matched. If no signal matches, the file path decides the area (`/static/src/` means frontend, `point_of_sale/` means POS, `tests/` means testing, and so on).
4. **Feedback outside the taxonomy** is clustered by token-set Jaccard similarity (≥ 0.5, same area). A cluster becomes a *candidate* rule only when it spans **at least 2 PRs**. Its title is the reviewer's own key sentence, so nothing is invented. Clusters are anchored on existing rules so that rule IDs stay stable.
5. **Count each thread once** per rule (a reviewer repeating themselves in one thread counts once).
6. **Statistics:** occurrences, distinct PRs, distinct reviewers, requirement ratio, addressed ratio, first and last seen. Scope is `general` when the rule was seen in two or more PRs, otherwise `context`. Severity is escalated to `high` when most evidence is explicit requirements from a *Changes requested* review. Confidence is computed by this formula:
   `0.30 + 0.15·min(PRs−1, 3) + 0.10·[≥2 reviewers] + 0.10·requirement_ratio + 0.05·addressed_ratio`.
7. **Your decisions win.** Re-extraction never changes a rule's status, your edits (`edited=1`), manual or excluded evidence, or merges. Evidence of merged rules counts toward the target. A pending, unedited rule that loses all its evidence is removed.

Templates can only fire on real comments, so a rule never exists without evidence, and every rule
keeps links to the comments that support it.

---

## 5. The generated skill

```
odoo-pr-review/
  SKILL.md                         frontmatter + A–E sections + rule index
  references/<area>.md             each rule: severity, scope, history, how to check, evidence quotes + links
  references/common-review-mistakes.md
  metadata.json                    version, content hash, every rule id/stat, llm_calls: 0, network_calls: 0
```

* **Content.** A: review workflow. B: rules grouped by area, with ID, severity, ×occurrences and PR count. C: the areas that have evidence (no inferred Odoo knowledge). D: pre-submit checklist (static items plus general and high-severity rules). E: the 8-step procedure for implementing reviewer feedback.
* **What is included.** Only `approved` rules with at least `min_occurrences` non-excluded evidence. Pending rules can be included as *(unapproved)* through a setting. Rejected, superseded and merged rules are never included. Rules with identical normalised titles are collapsed into one (`R-0001 (also R-0007)`).
* **Determinism.** No timestamps or randomness. Rules, evidence and files are sorted, whitespace is normalised, and JSON keys are sorted. The version is the first 12 hex characters of a SHA-256 over all files. Same database and same settings give byte-identical files.
* **Validation** (before anything is published): frontmatter name and description, every referenced file exists, every rule ID is listed in SKILL.md and has a section with at least one evidence link, no unresolved template markers, metadata consistent with the content and with zero calls, size limits.
* **Atomic publishing.** Files are written to `versions/.tmp-*`, fsynced, and renamed to `versions/<hash>/`. Then the `current` symlink is swapped atomically with a symlink-and-rename. **If generation, validation or publishing fails, `current` keeps pointing at the previous valid version** and the failure is recorded in `skill_builds`. An unchanged rebuild creates no new version (and a scheduled one creates no history row either). The newest `keep_versions` versions are kept.

---

## 6. Agent integration

Every publish target `<dir>` (default `~/.claude/skills`) gets a symlink
`<dir>/odoo-pr-review → ~/.config/review-dock/knowledge/skill/current`.

* **Claude Code** discovers `~/.claude/skills/odoo-pr-review/SKILL.md` automatically, and always reads the latest valid version because the link follows `current`. Nobody has to copy files.
* **Other agents.** Point them at the same directory. The Skill tab shows the path.
* **Checking the version.** Read `metadata.json` (`version`, `content_hash`), or open the Skill tab. Its preview shows whether the published skill differs from what the database renders now.
* A real directory that already exists at a target path is **never overwritten**. The publish reports a warning instead.

---

## 7. Operation

### Build

```bash
./build.sh            # desktop app → ~/reviewdock (the knowledge system is part of it)
```

### First run

1. `gh auth login` (once, if not logged in).
2. Start review-dock. Repositories from the sidebar are registered automatically. Add more, without a
   local clone, on **Review Knowledge → Sync**.
3. Click **Sync now**. The first sync imports the full history and is resumable. If you close the app
   mid-import, the next sync continues from the last committed PR.
4. Review the proposals in **Review Knowledge → Rules**: approve, reject, edit, merge, mark superseded,
   or inspect evidence and code context.
5. **Skill → Rebuild** publishes the approved rules. The next sync also does this automatically.

### Schedule

The knowledge sync schedule works **exactly like the scheduled rebase**:

* Configure it in **Review Knowledge → Sync → Scheduled sync**. Add one or more wall-clock times
  (for example `09:00 AM`, `02:00 PM`). It is off by default.
* The app's existing 55-second scheduler tick checks both schedules, and a run starts at most once per
  matching minute.
* It runs **only while review-dock is open** and `gh` is logged in. A missed time is not caught up
  later. If a run is already in progress, the scheduled one is skipped.
* Each scheduled run does sync → extract → build → publish, and appears in the sync history with
  trigger `schedule`.

### Logs and status

* `~/.config/review-dock/logs/knowledge.log`: structured JSON (slog), rotated. Contains no tokens.
* Desktop: **Review Knowledge → Overview / Sync / Skill**, plus `[KNOWLEDGE]` lines in the terminal panel.

### Settings (`kb_settings`, editable in the UI)

| Setting | Default | Meaning |
|---|---|---|
| `schedule_enabled`, `schedule_times` | false, `[]` | scheduled sync at wall-clock times (`"09:00 PM"`), while the app is open |
| `skill_name` | `odoo-pr-review` | skill directory and frontmatter name |
| `publish_targets` | `["~/.claude/skills"]` | where agent-facing symlinks are maintained |
| `auto_publish` | true | publish after each sync |
| `include_pending` | false | include unapproved rules (marked) |
| `min_occurrences` | 1 | minimum evidence for a rule to be published |
| `keep_versions` | 10 | skill versions kept on disk |
| `ignored_authors` | robodoo, fw-bot, C3POdoo, … | never treated as reviewers |
| `auto_approve_min_prs` | 0 (off) | auto-approve taxonomy rules seen in ≥ N PRs |

---

## 8. Zero-token guarantee: how it is enforced

| Layer | Mechanism | Verified by |
|---|---|---|
| Static imports | `db`, `extract`, `skillgen` never import `net`, `net/http`, `crypto/tls`, `os/exec`. `skillgen` depends **only on the Go standard library**. No HTTP, TLS or RPC client package is reachable from any of them, even transitively. No AI/LLM/embedding module is in `go.mod`. | `backend/knowledge/zerotoken_test.go`. A deliberately injected `net/http` import makes it fail (checked). |
| Runtime | Builds run inside `WithNetworkGuard`, which replaces `http.DefaultTransport` with a transport that refuses and counts requests. A build that attempts a request is aborted. The counts are stored in `skill_builds.network_calls` and `llm_calls`. | `TestNetworkGuardBlocksHTTP` |
| End to end | `TestZeroTokenSkillGeneration` makes the GitHub runner fail and count, empties `PATH` (no `gh`, `curl` or `python`), points `HTTP(S)_PROXY`, `OPENAI_BASE_URL`, `ANTHROPIC_BASE_URL` and others at a trap server, and sets fake AI keys. It then runs extract, build and publish, and asserts: complete skill, 0 GitHub calls, 0 trapped HTTP requests, recorded network=0 and llm=0, byte-identical rebuild, no new version, preview diff empty, and no AI SDK linked. | `go test ./backend/knowledge/pipeline -run ZeroToken` |
| Binary | The Skill tab inspects the running binary's linked modules for AI SDKs and shows the last build's counters. | manual |

Run the complete suite with `go test ./...` (it needs `frontend/dist` to exist for the root package: `cd frontend && npm ci && npm run build`).
