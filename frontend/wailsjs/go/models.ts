export namespace db {
	
	export class CommentFilter {
	    repo: string;
	    reviewer: string;
	    category: string;
	    kind: string;
	    query: string;
	    since: string;
	    until: string;
	    pr_number: number;
	    include_deleted: boolean;
	    limit: number;
	    offset: number;
	
	    static createFrom(source: any = {}) {
	        return new CommentFilter(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.repo = source["repo"];
	        this.reviewer = source["reviewer"];
	        this.category = source["category"];
	        this.kind = source["kind"];
	        this.query = source["query"];
	        this.since = source["since"];
	        this.until = source["until"];
	        this.pr_number = source["pr_number"];
	        this.include_deleted = source["include_deleted"];
	        this.limit = source["limit"];
	        this.offset = source["offset"];
	    }
	}
	export class CommentView {
	    id: number;
	    source: string;
	    github_id: number;
	    repo: string;
	    pr_number: number;
	    pr_title: string;
	    pr_url: string;
	    pr_author: string;
	    author: string;
	    body: string;
	    path: string;
	    line: number;
	    original_line: number;
	    diff_hunk: string;
	    html_url: string;
	    review_state: string;
	    thread_root_id: number;
	    is_resolved?: boolean;
	    is_outdated?: boolean;
	    code_changed: string;
	    created_at: string;
	    updated_at: string;
	    deleted_at: string;
	    kind: string;
	    category: string;
	    subcategory: string;
	    signals: string;
	    rule_ids: number[];
	    link_source?: string;
	    revisions: number;
	
	    static createFrom(source: any = {}) {
	        return new CommentView(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.source = source["source"];
	        this.github_id = source["github_id"];
	        this.repo = source["repo"];
	        this.pr_number = source["pr_number"];
	        this.pr_title = source["pr_title"];
	        this.pr_url = source["pr_url"];
	        this.pr_author = source["pr_author"];
	        this.author = source["author"];
	        this.body = source["body"];
	        this.path = source["path"];
	        this.line = source["line"];
	        this.original_line = source["original_line"];
	        this.diff_hunk = source["diff_hunk"];
	        this.html_url = source["html_url"];
	        this.review_state = source["review_state"];
	        this.thread_root_id = source["thread_root_id"];
	        this.is_resolved = source["is_resolved"];
	        this.is_outdated = source["is_outdated"];
	        this.code_changed = source["code_changed"];
	        this.created_at = source["created_at"];
	        this.updated_at = source["updated_at"];
	        this.deleted_at = source["deleted_at"];
	        this.kind = source["kind"];
	        this.category = source["category"];
	        this.subcategory = source["subcategory"];
	        this.signals = source["signals"];
	        this.rule_ids = source["rule_ids"];
	        this.link_source = source["link_source"];
	        this.revisions = source["revisions"];
	    }
	}
	export class CountItem {
	    key: string;
	    count: number;
	
	    static createFrom(source: any = {}) {
	        return new CountItem(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.key = source["key"];
	        this.count = source["count"];
	    }
	}
	export class SkillBuild {
	    id: number;
	    created_at: string;
	    status: string;
	    content_hash: string;
	    knowledge_fingerprint: string;
	    generator_version: string;
	    rule_count: number;
	    file_count: number;
	    total_bytes: number;
	    version_dir: string;
	    network_calls: number;
	    llm_calls: number;
	    changes: string;
	    error: string;
	
	    static createFrom(source: any = {}) {
	        return new SkillBuild(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.created_at = source["created_at"];
	        this.status = source["status"];
	        this.content_hash = source["content_hash"];
	        this.knowledge_fingerprint = source["knowledge_fingerprint"];
	        this.generator_version = source["generator_version"];
	        this.rule_count = source["rule_count"];
	        this.file_count = source["file_count"];
	        this.total_bytes = source["total_bytes"];
	        this.version_dir = source["version_dir"];
	        this.network_calls = source["network_calls"];
	        this.llm_calls = source["llm_calls"];
	        this.changes = source["changes"];
	        this.error = source["error"];
	    }
	}
	export class SyncRunRepo {
	    repo_id: number;
	    full_name: string;
	    status: string;
	    prs_synced: number;
	    comments: number;
	    error: string;
	    started_at: string;
	    finished_at: string;
	
	    static createFrom(source: any = {}) {
	        return new SyncRunRepo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.repo_id = source["repo_id"];
	        this.full_name = source["full_name"];
	        this.status = source["status"];
	        this.prs_synced = source["prs_synced"];
	        this.comments = source["comments"];
	        this.error = source["error"];
	        this.started_at = source["started_at"];
	        this.finished_at = source["finished_at"];
	    }
	}
	export class SyncRun {
	    id: number;
	    trigger: string;
	    started_at: string;
	    finished_at: string;
	    status: string;
	    repos_total: number;
	    repos_failed: number;
	    prs_synced: number;
	    comments_upserted: number;
	    comments_deleted: number;
	    rules_total: number;
	    skill_build_id: number;
	    error: string;
	    repos?: SyncRunRepo[];
	
	    static createFrom(source: any = {}) {
	        return new SyncRun(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.trigger = source["trigger"];
	        this.started_at = source["started_at"];
	        this.finished_at = source["finished_at"];
	        this.status = source["status"];
	        this.repos_total = source["repos_total"];
	        this.repos_failed = source["repos_failed"];
	        this.prs_synced = source["prs_synced"];
	        this.comments_upserted = source["comments_upserted"];
	        this.comments_deleted = source["comments_deleted"];
	        this.rules_total = source["rules_total"];
	        this.skill_build_id = source["skill_build_id"];
	        this.error = source["error"];
	        this.repos = this.convertValues(source["repos"], SyncRunRepo);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class Rule {
	    id: number;
	    code: string;
	    rule_key: string;
	    origin: string;
	    category: string;
	    subcategory: string;
	    title: string;
	    guidance: string;
	    scope: string;
	    severity: string;
	    status: string;
	    merged_into_id: number;
	    edited: boolean;
	    occurrences: number;
	    distinct_prs: number;
	    distinct_reviewers: number;
	    requirement_ratio: number;
	    addressed_ratio: number;
	    confidence: number;
	    first_seen_at: string;
	    last_seen_at: string;
	    note: string;
	    created_at: string;
	    updated_at: string;
	    status_changed_at: string;
	
	    static createFrom(source: any = {}) {
	        return new Rule(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.code = source["code"];
	        this.rule_key = source["rule_key"];
	        this.origin = source["origin"];
	        this.category = source["category"];
	        this.subcategory = source["subcategory"];
	        this.title = source["title"];
	        this.guidance = source["guidance"];
	        this.scope = source["scope"];
	        this.severity = source["severity"];
	        this.status = source["status"];
	        this.merged_into_id = source["merged_into_id"];
	        this.edited = source["edited"];
	        this.occurrences = source["occurrences"];
	        this.distinct_prs = source["distinct_prs"];
	        this.distinct_reviewers = source["distinct_reviewers"];
	        this.requirement_ratio = source["requirement_ratio"];
	        this.addressed_ratio = source["addressed_ratio"];
	        this.confidence = source["confidence"];
	        this.first_seen_at = source["first_seen_at"];
	        this.last_seen_at = source["last_seen_at"];
	        this.note = source["note"];
	        this.created_at = source["created_at"];
	        this.updated_at = source["updated_at"];
	        this.status_changed_at = source["status_changed_at"];
	    }
	}
	export class Overview {
	    repositories: number;
	    prs: number;
	    comments: number;
	    reviewer_comments: number;
	    rules: number;
	    rules_by_status: CountItem[];
	    kind_counts: CountItem[];
	    category_counts: CountItem[];
	    top_reviewers: CountItem[];
	    top_rules: Rule[];
	    last_run?: SyncRun;
	    last_build?: SkillBuild;
	    schema_version: number;
	    db_path: string;
	
	    static createFrom(source: any = {}) {
	        return new Overview(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.repositories = source["repositories"];
	        this.prs = source["prs"];
	        this.comments = source["comments"];
	        this.reviewer_comments = source["reviewer_comments"];
	        this.rules = source["rules"];
	        this.rules_by_status = this.convertValues(source["rules_by_status"], CountItem);
	        this.kind_counts = this.convertValues(source["kind_counts"], CountItem);
	        this.category_counts = this.convertValues(source["category_counts"], CountItem);
	        this.top_reviewers = this.convertValues(source["top_reviewers"], CountItem);
	        this.top_rules = this.convertValues(source["top_rules"], Rule);
	        this.last_run = this.convertValues(source["last_run"], SyncRun);
	        this.last_build = this.convertValues(source["last_build"], SkillBuild);
	        this.schema_version = source["schema_version"];
	        this.db_path = source["db_path"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class Repository {
	    id: number;
	    full_name: string;
	    owner: string;
	    name: string;
	    enabled: boolean;
	    author_filter: string;
	    sync_status: string;
	    checkpoint_updated_at: string;
	    full_import_done: boolean;
	    last_attempt_at: string;
	    last_success_at: string;
	    last_error: string;
	    consecutive_failures: number;
	    created_at: string;
	    pr_count: number;
	    comment_count: number;
	
	    static createFrom(source: any = {}) {
	        return new Repository(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.full_name = source["full_name"];
	        this.owner = source["owner"];
	        this.name = source["name"];
	        this.enabled = source["enabled"];
	        this.author_filter = source["author_filter"];
	        this.sync_status = source["sync_status"];
	        this.checkpoint_updated_at = source["checkpoint_updated_at"];
	        this.full_import_done = source["full_import_done"];
	        this.last_attempt_at = source["last_attempt_at"];
	        this.last_success_at = source["last_success_at"];
	        this.last_error = source["last_error"];
	        this.consecutive_failures = source["consecutive_failures"];
	        this.created_at = source["created_at"];
	        this.pr_count = source["pr_count"];
	        this.comment_count = source["comment_count"];
	    }
	}
	
	export class RuleEdit {
	    title: string;
	    guidance: string;
	    category: string;
	    subcategory: string;
	    severity: string;
	    note: string;
	
	    static createFrom(source: any = {}) {
	        return new RuleEdit(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.title = source["title"];
	        this.guidance = source["guidance"];
	        this.category = source["category"];
	        this.subcategory = source["subcategory"];
	        this.severity = source["severity"];
	        this.note = source["note"];
	    }
	}
	export class RuleFilter {
	    status: string;
	    category: string;
	    query: string;
	    min_occurrences: number;
	    limit: number;
	
	    static createFrom(source: any = {}) {
	        return new RuleFilter(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.status = source["status"];
	        this.category = source["category"];
	        this.query = source["query"];
	        this.min_occurrences = source["min_occurrences"];
	        this.limit = source["limit"];
	    }
	}
	export class Settings {
	    schedule_enabled: boolean;
	    schedule_times: string[];
	    skill_name: string;
	    publish_targets: string[];
	    auto_publish: boolean;
	    include_pending: boolean;
	    min_occurrences: number;
	    keep_versions: number;
	    ignored_authors: string[];
	    auto_approve_min_prs: number;
	
	    static createFrom(source: any = {}) {
	        return new Settings(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.schedule_enabled = source["schedule_enabled"];
	        this.schedule_times = source["schedule_times"];
	        this.skill_name = source["skill_name"];
	        this.publish_targets = source["publish_targets"];
	        this.auto_publish = source["auto_publish"];
	        this.include_pending = source["include_pending"];
	        this.min_occurrences = source["min_occurrences"];
	        this.keep_versions = source["keep_versions"];
	        this.ignored_authors = source["ignored_authors"];
	        this.auto_approve_min_prs = source["auto_approve_min_prs"];
	    }
	}
	
	

}

export namespace extract {
	
	export class Summary {
	    comments_analyzed: number;
	    by_kind: Record<string, number>;
	    rules_total: number;
	    rules_added: number;
	    rules_removed: number;
	    auto_approved: number;
	
	    static createFrom(source: any = {}) {
	        return new Summary(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.comments_analyzed = source["comments_analyzed"];
	        this.by_kind = source["by_kind"];
	        this.rules_total = source["rules_total"];
	        this.rules_added = source["rules_added"];
	        this.rules_removed = source["rules_removed"];
	        this.auto_approved = source["auto_approved"];
	    }
	}

}

export namespace main {
	
	export class KBCategory {
	    id: string;
	    title: string;
	
	    static createFrom(source: any = {}) {
	        return new KBCategory(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.title = source["title"];
	    }
	}
	export class KBCommentPage {
	    items: db.CommentView[];
	    total: number;
	
	    static createFrom(source: any = {}) {
	        return new KBCommentPage(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.items = this.convertValues(source["items"], db.CommentView);
	        this.total = source["total"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class KBMeta {
	    categories: KBCategory[];
	    reviewers: string[];
	    running: boolean;
	
	    static createFrom(source: any = {}) {
	        return new KBMeta(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.categories = this.convertValues(source["categories"], KBCategory);
	        this.reviewers = source["reviewers"];
	        this.running = source["running"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class KBRuleDetail {
	    rule: db.Rule;
	    evidence: db.CommentView[];
	    merged: db.Rule[];
	
	    static createFrom(source: any = {}) {
	        return new KBRuleDetail(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.rule = this.convertValues(source["rule"], db.Rule);
	        this.evidence = this.convertValues(source["evidence"], db.CommentView);
	        this.merged = this.convertValues(source["merged"], db.Rule);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class KBSkillInfo {
	    version: string;
	    skill_name: string;
	    current_dir: string;
	    current_link: string;
	    targets: string[];
	    files: Record<string, string>;
	    file_order: string[];
	    builds: db.SkillBuild[];
	    zero_token: pipeline.ZeroTokenReport;
	
	    static createFrom(source: any = {}) {
	        return new KBSkillInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.version = source["version"];
	        this.skill_name = source["skill_name"];
	        this.current_dir = source["current_dir"];
	        this.current_link = source["current_link"];
	        this.targets = source["targets"];
	        this.files = source["files"];
	        this.file_order = source["file_order"];
	        this.builds = this.convertValues(source["builds"], db.SkillBuild);
	        this.zero_token = this.convertValues(source["zero_token"], pipeline.ZeroTokenReport);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}

}

export namespace models {
	
	export class PullRequest {
	    id: string;
	    number: number;
	    title: string;
	    repo_id: string;
	    repo_name: string;
	    base_branch: string;
	    head_branch: string;
	    base_label: string;
	    head_label: string;
	    state: string;
	    is_draft: boolean;
	    // Go type: time
	    created_at: any;
	    // Go type: time
	    updated_at: any;
	    ahead_count: number;
	    behind_count: number;
	    local_ahead_count: number;
	    local_behind_count: number;
	    mergeable_status: string;
	    html_url: string;
	    description: string;
	    ci_status: string;
	    author: string;
	    labels: string[];
	    requested_reviewers: string[];
	
	    static createFrom(source: any = {}) {
	        return new PullRequest(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.number = source["number"];
	        this.title = source["title"];
	        this.repo_id = source["repo_id"];
	        this.repo_name = source["repo_name"];
	        this.base_branch = source["base_branch"];
	        this.head_branch = source["head_branch"];
	        this.base_label = source["base_label"];
	        this.head_label = source["head_label"];
	        this.state = source["state"];
	        this.is_draft = source["is_draft"];
	        this.created_at = this.convertValues(source["created_at"], null);
	        this.updated_at = this.convertValues(source["updated_at"], null);
	        this.ahead_count = source["ahead_count"];
	        this.behind_count = source["behind_count"];
	        this.local_ahead_count = source["local_ahead_count"];
	        this.local_behind_count = source["local_behind_count"];
	        this.mergeable_status = source["mergeable_status"];
	        this.html_url = source["html_url"];
	        this.description = source["description"];
	        this.ci_status = source["ci_status"];
	        this.author = source["author"];
	        this.labels = source["labels"];
	        this.requested_reviewers = source["requested_reviewers"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class RebaseRequest {
	    id: string;
	    repo_id: string;
	    head_label: string;
	    base_label: string;
	
	    static createFrom(source: any = {}) {
	        return new RebaseRequest(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.repo_id = source["repo_id"];
	        this.head_label = source["head_label"];
	        this.base_label = source["base_label"];
	    }
	}
	export class Repository {
	    id: string;
	    owner: string;
	    name: string;
	    local_path: string;
	    sync_status: string;
	    // Go type: time
	    last_fetched_at: any;
	
	    static createFrom(source: any = {}) {
	        return new Repository(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.owner = source["owner"];
	        this.name = source["name"];
	        this.local_path = source["local_path"];
	        this.sync_status = source["sync_status"];
	        this.last_fetched_at = this.convertValues(source["last_fetched_at"], null);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class User {
	    login: string;
	    id: number;
	    avatar_url: string;
	    html_url: string;
	
	    static createFrom(source: any = {}) {
	        return new User(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.login = source["login"];
	        this.id = source["id"];
	        this.avatar_url = source["avatar_url"];
	        this.html_url = source["html_url"];
	    }
	}
	export class Session {
	    token_type: string;
	    scope: string;
	    user?: User;
	
	    static createFrom(source: any = {}) {
	        return new Session(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.token_type = source["token_type"];
	        this.scope = source["scope"];
	        this.user = this.convertValues(source["user"], User);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class Settings {
	    concurrency_limit: number;
	    amend_commit_timestamp: boolean;
	    force_push_after_rebase: boolean;
	    theme: string;
	    cron_enabled: boolean;
	    cron_times: string[];
	    cron_include_drafts: boolean;
	
	    static createFrom(source: any = {}) {
	        return new Settings(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.concurrency_limit = source["concurrency_limit"];
	        this.amend_commit_timestamp = source["amend_commit_timestamp"];
	        this.force_push_after_rebase = source["force_push_after_rebase"];
	        this.theme = source["theme"];
	        this.cron_enabled = source["cron_enabled"];
	        this.cron_times = source["cron_times"];
	        this.cron_include_drafts = source["cron_include_drafts"];
	    }
	}

}

export namespace pipeline {
	
	export class BuildResult {
	    build: db.SkillBuild;
	    publish?: skillgen.PublishResult;
	    errors?: string[];
	
	    static createFrom(source: any = {}) {
	        return new BuildResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.build = this.convertValues(source["build"], db.SkillBuild);
	        this.publish = this.convertValues(source["publish"], skillgen.PublishResult);
	        this.errors = source["errors"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class Preview {
	    version: string;
	    current_version: string;
	    changed: boolean;
	    rule_count: number;
	    files: Record<string, string>;
	    diff: string;
	    errors: string[];
	
	    static createFrom(source: any = {}) {
	        return new Preview(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.version = source["version"];
	        this.current_version = source["current_version"];
	        this.changed = source["changed"];
	        this.rule_count = source["rule_count"];
	        this.files = source["files"];
	        this.diff = source["diff"];
	        this.errors = source["errors"];
	    }
	}
	export class ZeroTokenReport {
	    linked_modules: number;
	    forbidden_linked: string[];
	    last_build_network_calls: number;
	    last_build_llm_calls: number;
	    generator_packages: string[];
	    statement: string;
	    build_info_unavailable: boolean;
	
	    static createFrom(source: any = {}) {
	        return new ZeroTokenReport(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.linked_modules = source["linked_modules"];
	        this.forbidden_linked = source["forbidden_linked"];
	        this.last_build_network_calls = source["last_build_network_calls"];
	        this.last_build_llm_calls = source["last_build_llm_calls"];
	        this.generator_packages = source["generator_packages"];
	        this.statement = source["statement"];
	        this.build_info_unavailable = source["build_info_unavailable"];
	    }
	}

}

export namespace skillgen {
	
	export class PublishResult {
	    version_dir: string;
	    current: string;
	    changed: boolean;
	    targets: string[];
	    warnings: string[];
	    pruned: string[];
	
	    static createFrom(source: any = {}) {
	        return new PublishResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.version_dir = source["version_dir"];
	        this.current = source["current"];
	        this.changed = source["changed"];
	        this.targets = source["targets"];
	        this.warnings = source["warnings"];
	        this.pruned = source["pruned"];
	    }
	}

}

export namespace stats {
	
	export class ReviewerSummary {
	    approved: number;
	    changes_requested: number;
	    commented: number;
	    total_comments: number;
	
	    static createFrom(source: any = {}) {
	        return new ReviewerSummary(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.approved = source["approved"];
	        this.changes_requested = source["changes_requested"];
	        this.commented = source["commented"];
	        this.total_comments = source["total_comments"];
	    }
	}
	export class DashboardStats {
	    total_prs: number;
	    open_prs: number;
	    draft_prs: number;
	    closed_prs: number;
	    total_comments: number;
	    total_commits: number;
	    total_additions: number;
	    total_deletions: number;
	    directly_merged_count: number;
	    avg_pr_age_days: number;
	    reviewer_activity: Record<string, ReviewerSummary>;
	    prs_by_repo: Record<string, number>;
	    prs_by_month: Record<string, number>;
	    stale_recomputed: number;
	    cache_hits: number;
	    // Go type: time
	    last_updated: any;
	
	    static createFrom(source: any = {}) {
	        return new DashboardStats(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.total_prs = source["total_prs"];
	        this.open_prs = source["open_prs"];
	        this.draft_prs = source["draft_prs"];
	        this.closed_prs = source["closed_prs"];
	        this.total_comments = source["total_comments"];
	        this.total_commits = source["total_commits"];
	        this.total_additions = source["total_additions"];
	        this.total_deletions = source["total_deletions"];
	        this.directly_merged_count = source["directly_merged_count"];
	        this.avg_pr_age_days = source["avg_pr_age_days"];
	        this.reviewer_activity = this.convertValues(source["reviewer_activity"], ReviewerSummary, true);
	        this.prs_by_repo = source["prs_by_repo"];
	        this.prs_by_month = source["prs_by_month"];
	        this.stale_recomputed = source["stale_recomputed"];
	        this.cache_hits = source["cache_hits"];
	        this.last_updated = this.convertValues(source["last_updated"], null);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class PRDashboardInput {
	    id: string;
	    repo_id: string;
	    number: number;
	    updated_at: string;
	    created_at: string;
	    state: string;
	    is_draft: boolean;
	    repo_name: string;
	
	    static createFrom(source: any = {}) {
	        return new PRDashboardInput(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.repo_id = source["repo_id"];
	        this.number = source["number"];
	        this.updated_at = source["updated_at"];
	        this.created_at = source["created_at"];
	        this.state = source["state"];
	        this.is_draft = source["is_draft"];
	        this.repo_name = source["repo_name"];
	    }
	}
	export class PRStats {
	    total_comments: number;
	    pr_conversation_comments: number;
	    review_comments: number;
	    author_replies: number;
	    review_comments_by_reviewer: Record<string, number>;
	    reviewer_states: Record<string, string>;
	    commits: number;
	    changed_files: number;
	    additions: number;
	    deletions: number;
	    directly_merged: boolean;
	    pr_age_days: number;
	    labels: string[];
	    author: string;
	    created_at: string;
	    // Go type: time
	    last_updated: any;
	    from_cache: boolean;
	
	    static createFrom(source: any = {}) {
	        return new PRStats(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.total_comments = source["total_comments"];
	        this.pr_conversation_comments = source["pr_conversation_comments"];
	        this.review_comments = source["review_comments"];
	        this.author_replies = source["author_replies"];
	        this.review_comments_by_reviewer = source["review_comments_by_reviewer"];
	        this.reviewer_states = source["reviewer_states"];
	        this.commits = source["commits"];
	        this.changed_files = source["changed_files"];
	        this.additions = source["additions"];
	        this.deletions = source["deletions"];
	        this.directly_merged = source["directly_merged"];
	        this.pr_age_days = source["pr_age_days"];
	        this.labels = source["labels"];
	        this.author = source["author"];
	        this.created_at = source["created_at"];
	        this.last_updated = this.convertValues(source["last_updated"], null);
	        this.from_cache = source["from_cache"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}

}

