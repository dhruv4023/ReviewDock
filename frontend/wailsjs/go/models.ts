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
	    access_token: string;
	    token_type: string;
	    scope: string;
	    user?: User;
	
	    static createFrom(source: any = {}) {
	        return new Session(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.access_token = source["access_token"];
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

