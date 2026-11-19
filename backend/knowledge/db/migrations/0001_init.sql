-- review-dock knowledge database, initial schema.
-- All timestamps are RFC3339 UTC strings. GitHub identifiers are the
-- deduplication keys; local surrogate keys are never exposed to GitHub.

CREATE TABLE kb_settings (
    key   TEXT PRIMARY KEY,
    value TEXT NOT NULL
);

CREATE TABLE repositories (
    id                    INTEGER PRIMARY KEY AUTOINCREMENT,
    full_name             TEXT NOT NULL UNIQUE,          -- owner/name
    owner                 TEXT NOT NULL,
    name                  TEXT NOT NULL,
    enabled               INTEGER NOT NULL DEFAULT 1,
    author_filter         TEXT NOT NULL DEFAULT '@me',   -- GitHub search author qualifier
    sync_status           TEXT NOT NULL DEFAULT 'never', -- never|running|ok|error|rate_limited|interrupted
    checkpoint_updated_at TEXT,                          -- high-water mark of PR updated_at fully processed
    full_import_done      INTEGER NOT NULL DEFAULT 0,
    last_attempt_at       TEXT,
    last_success_at       TEXT,
    last_error            TEXT,
    consecutive_failures  INTEGER NOT NULL DEFAULT 0,
    created_at            TEXT NOT NULL
);

CREATE TABLE pull_requests (
    id           INTEGER PRIMARY KEY,                   -- GitHub PR database id
    repo_id      INTEGER NOT NULL REFERENCES repositories(id) ON DELETE CASCADE,
    node_id      TEXT,
    number       INTEGER NOT NULL,
    title        TEXT NOT NULL DEFAULT '',
    url          TEXT NOT NULL DEFAULT '',
    author       TEXT NOT NULL DEFAULT '',
    state        TEXT NOT NULL DEFAULT '',              -- open|closed
    merged       INTEGER NOT NULL DEFAULT 0,
    base_ref     TEXT,
    head_sha     TEXT,
    created_at   TEXT,
    updated_at   TEXT,
    closed_at    TEXT,
    merged_at    TEXT,
    synced_at    TEXT NOT NULL,
    UNIQUE (repo_id, number)
);
CREATE INDEX idx_pr_repo_updated ON pull_requests(repo_id, updated_at);

CREATE TABLE review_comments (
    id                  INTEGER PRIMARY KEY AUTOINCREMENT,
    source              TEXT NOT NULL,                  -- inline|issue|review
    github_id           INTEGER NOT NULL,
    node_id             TEXT,
    pr_id               INTEGER NOT NULL REFERENCES pull_requests(id) ON DELETE CASCADE,
    author              TEXT NOT NULL DEFAULT '',
    author_association  TEXT,
    is_bot              INTEGER NOT NULL DEFAULT 0,
    is_pr_author        INTEGER NOT NULL DEFAULT 0,
    body                TEXT NOT NULL DEFAULT '',       -- current body, verbatim
    review_state        TEXT,                           -- APPROVED|CHANGES_REQUESTED|COMMENTED|DISMISSED (reviews, and inline via review_id)
    review_id           INTEGER,                        -- pull_request_review_id for inline comments
    in_reply_to_id      INTEGER,                        -- github_id of the parent inline comment
    thread_root_id      INTEGER,                        -- github_id of the first comment in the thread
    thread_node_id      TEXT,
    path                TEXT,
    line                INTEGER,
    original_line       INTEGER,
    side                TEXT,
    commit_sha          TEXT,
    original_commit_sha TEXT,
    diff_hunk           TEXT,                           -- code context captured by GitHub
    html_url            TEXT,
    is_resolved         INTEGER,                        -- NULL = unknown
    is_outdated         INTEGER,                        -- NULL = unknown
    code_changed        TEXT NOT NULL DEFAULT 'unknown',-- yes|no|unknown (deterministic heuristic)
    created_at          TEXT,
    updated_at          TEXT,
    deleted_at          TEXT,                           -- set when no longer returned by GitHub
    body_hash           TEXT NOT NULL DEFAULT '',
    raw_json            TEXT,
    first_seen_at       TEXT NOT NULL,
    last_seen_at        TEXT NOT NULL,
    UNIQUE (source, github_id)
);
CREATE INDEX idx_rc_pr ON review_comments(pr_id);
CREATE INDEX idx_rc_author ON review_comments(author);
CREATE INDEX idx_rc_created ON review_comments(created_at);
CREATE INDEX idx_rc_thread ON review_comments(thread_root_id);
CREATE INDEX idx_rc_path ON review_comments(path);

-- Previous bodies of edited comments: the original reviewer text is never lost.
CREATE TABLE comment_revisions (
    id           INTEGER PRIMARY KEY AUTOINCREMENT,
    comment_id   INTEGER NOT NULL REFERENCES review_comments(id) ON DELETE CASCADE,
    body         TEXT NOT NULL,
    body_hash    TEXT NOT NULL,
    updated_at   TEXT,
    recorded_at  TEXT NOT NULL
);
CREATE INDEX idx_rev_comment ON comment_revisions(comment_id);

-- Deterministic per-comment analysis (fully recomputed by the extractor).
CREATE TABLE comment_analysis (
    comment_id        INTEGER PRIMARY KEY REFERENCES review_comments(id) ON DELETE CASCADE,
    kind              TEXT NOT NULL,      -- requirement|suggestion|question|superseded|noise|unclear|self
    category          TEXT NOT NULL,
    subcategory       TEXT NOT NULL DEFAULT '',
    signals           TEXT NOT NULL DEFAULT '',  -- comma separated signal ids
    normalized        TEXT NOT NULL DEFAULT '',
    extractor_version TEXT NOT NULL
);
CREATE INDEX idx_ca_category ON comment_analysis(category);
CREATE INDEX idx_ca_kind ON comment_analysis(kind);

CREATE TABLE rules (
    id                 INTEGER PRIMARY KEY AUTOINCREMENT,
    rule_key           TEXT NOT NULL UNIQUE,  -- sig:<signal-id> | cand:<fingerprint> | manual:<n>
    origin             TEXT NOT NULL,         -- signal|candidate|manual
    category           TEXT NOT NULL,
    subcategory        TEXT NOT NULL DEFAULT '',
    title              TEXT NOT NULL,
    guidance           TEXT NOT NULL DEFAULT '',
    scope              TEXT NOT NULL DEFAULT 'context',  -- general|context
    severity           TEXT NOT NULL DEFAULT 'context-dependent',
    status             TEXT NOT NULL DEFAULT 'pending',  -- pending|approved|rejected|superseded|merged
    merged_into_id     INTEGER REFERENCES rules(id) ON DELETE SET NULL,
    edited             INTEGER NOT NULL DEFAULT 0,       -- 1 = user edited text/category; extractor must not overwrite
    occurrences        INTEGER NOT NULL DEFAULT 0,
    distinct_prs       INTEGER NOT NULL DEFAULT 0,
    distinct_reviewers INTEGER NOT NULL DEFAULT 0,
    requirement_ratio  REAL NOT NULL DEFAULT 0,
    addressed_ratio    REAL NOT NULL DEFAULT 0,
    confidence         REAL NOT NULL DEFAULT 0,
    first_seen_at      TEXT,
    last_seen_at       TEXT,
    note               TEXT NOT NULL DEFAULT '',
    created_at         TEXT NOT NULL,
    updated_at         TEXT NOT NULL,
    status_changed_at  TEXT
);
CREATE INDEX idx_rules_status ON rules(status);
CREATE INDEX idx_rules_category ON rules(category);

CREATE TABLE rule_evidence (
    rule_id       INTEGER NOT NULL REFERENCES rules(id) ON DELETE CASCADE,
    comment_id    INTEGER NOT NULL REFERENCES review_comments(id) ON DELETE CASCADE,
    link_source   TEXT NOT NULL,   -- auto|manual|excluded
    feedback_kind TEXT NOT NULL DEFAULT '',
    score         REAL NOT NULL DEFAULT 0,
    PRIMARY KEY (rule_id, comment_id)
);
CREATE INDEX idx_ev_comment ON rule_evidence(comment_id);

CREATE TABLE sync_runs (
    id                 INTEGER PRIMARY KEY AUTOINCREMENT,
    trigger            TEXT NOT NULL,          -- manual|schedule|cli|retry
    started_at         TEXT NOT NULL,
    finished_at        TEXT,
    status             TEXT NOT NULL,          -- running|ok|partial|error|interrupted|skipped
    repos_total        INTEGER NOT NULL DEFAULT 0,
    repos_failed       INTEGER NOT NULL DEFAULT 0,
    prs_synced         INTEGER NOT NULL DEFAULT 0,
    comments_upserted  INTEGER NOT NULL DEFAULT 0,
    comments_deleted   INTEGER NOT NULL DEFAULT 0,
    rules_total        INTEGER NOT NULL DEFAULT 0,
    skill_build_id     INTEGER,
    error              TEXT
);

CREATE TABLE sync_run_repos (
    run_id      INTEGER NOT NULL REFERENCES sync_runs(id) ON DELETE CASCADE,
    repo_id     INTEGER NOT NULL REFERENCES repositories(id) ON DELETE CASCADE,
    status      TEXT NOT NULL,
    prs_synced  INTEGER NOT NULL DEFAULT 0,
    comments    INTEGER NOT NULL DEFAULT 0,
    error       TEXT,
    started_at  TEXT NOT NULL,
    finished_at TEXT,
    PRIMARY KEY (run_id, repo_id)
);

CREATE TABLE skill_builds (
    id                    INTEGER PRIMARY KEY AUTOINCREMENT,
    created_at            TEXT NOT NULL,
    status                TEXT NOT NULL,       -- published|unchanged|failed
    content_hash          TEXT,
    knowledge_fingerprint TEXT,
    generator_version     TEXT NOT NULL,
    rule_count            INTEGER NOT NULL DEFAULT 0,
    file_count            INTEGER NOT NULL DEFAULT 0,
    total_bytes           INTEGER NOT NULL DEFAULT 0,
    version_dir           TEXT,
    network_calls         INTEGER NOT NULL DEFAULT 0,
    llm_calls             INTEGER NOT NULL DEFAULT 0,
    changes               TEXT,                -- JSON summary of added/removed rule ids vs previous version
    error                 TEXT
);
