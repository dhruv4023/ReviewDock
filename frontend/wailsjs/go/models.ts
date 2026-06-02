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
	    updated_at: any;
	    behind_count: number;
	    ahead_count: number;
	    local_ahead_count: number;
	    local_behind_count: number;
	    mergeable_status: string;
	    html_url: string;
	    description: string;
	
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
	        this.updated_at = this.convertValues(source["updated_at"], null);
	        this.behind_count = source["behind_count"];
	        this.ahead_count = source["ahead_count"];
	        this.local_ahead_count = source["local_ahead_count"];
	        this.local_behind_count = source["local_behind_count"];
	        this.mergeable_status = source["mergeable_status"];
	        this.html_url = source["html_url"];
	        this.description = source["description"];
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
	    head_branch: string;
	    base_branch: string;
	
	    static createFrom(source: any = {}) {
	        return new RebaseRequest(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.repo_id = source["repo_id"];
	        this.head_branch = source["head_branch"];
	        this.base_branch = source["base_branch"];
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
	    default_remote_priority: string[];
	    amend_commit_timestamp: boolean;
	    force_push_after_rebase: boolean;
	    auto_refresh_interval_mins: number;
	    theme: string;
	
	    static createFrom(source: any = {}) {
	        return new Settings(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.concurrency_limit = source["concurrency_limit"];
	        this.default_remote_priority = source["default_remote_priority"];
	        this.amend_commit_timestamp = source["amend_commit_timestamp"];
	        this.force_push_after_rebase = source["force_push_after_rebase"];
	        this.auto_refresh_interval_mins = source["auto_refresh_interval_mins"];
	        this.theme = source["theme"];
	    }
	}

}

