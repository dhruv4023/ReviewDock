import { create } from 'zustand';

// ─── Types (mirror backend/knowledge/db and app_knowledge.go) ────────────────

export interface KBRepository {
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
  pr_count: number;
  comment_count: number;
}

export interface KBSettings {
  schedule_enabled: boolean;
  interval_minutes: number;
  skill_name: string;
  publish_targets: string[];
  auto_publish: boolean;
  include_pending: boolean;
  min_occurrences: number;
  keep_versions: number;
  ignored_authors: string[];
  auto_approve_min_prs: number;
}

export interface KBRule {
  id: number;
  code: string;
  rule_key: string;
  origin: 'signal' | 'candidate' | 'manual';
  category: string;
  subcategory: string;
  title: string;
  guidance: string;
  scope: 'general' | 'context';
  severity: string;
  status: 'pending' | 'approved' | 'rejected' | 'superseded' | 'merged';
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
}

export interface KBComment {
  id: number;
  source: 'inline' | 'issue' | 'review';
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
  is_resolved: boolean | null;
  is_outdated: boolean | null;
  code_changed: 'yes' | 'no' | 'unknown';
  created_at: string;
  deleted_at: string;
  kind: string;
  category: string;
  subcategory: string;
  signals: string;
  rule_ids: number[] | null;
  link_source?: string;
  revisions: number;
}

export interface KBCommentFilter {
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
}

export interface KBRuleFilter {
  status: string;
  category: string;
  query: string;
  min_occurrences: number;
  limit: number;
}

export interface KBSyncRunRepo {
  repo_id: number;
  full_name: string;
  status: string;
  prs_synced: number;
  comments: number;
  error: string;
  started_at: string;
  finished_at: string;
}

export interface KBSyncRun {
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
  repos?: KBSyncRunRepo[];
}

export interface KBSkillBuild {
  id: number;
  created_at: string;
  status: string;
  content_hash: string;
  generator_version: string;
  rule_count: number;
  file_count: number;
  total_bytes: number;
  version_dir: string;
  network_calls: number;
  llm_calls: number;
  changes: string;
  error: string;
}

export interface KBCount { key: string; count: number; }

export interface KBOverview {
  repositories: number;
  prs: number;
  comments: number;
  reviewer_comments: number;
  rules: number;
  rules_by_status: KBCount[] | null;
  kind_counts: KBCount[] | null;
  category_counts: KBCount[] | null;
  top_reviewers: KBCount[] | null;
  top_rules: KBRule[] | null;
  last_run: KBSyncRun | null;
  last_build: KBSkillBuild | null;
  schema_version: number;
  db_path: string;
}

export interface KBZeroToken {
  linked_modules: number;
  forbidden_linked: string[];
  last_build_network_calls: number;
  last_build_llm_calls: number;
  generator_packages: string[];
  statement: string;
  build_info_unavailable: boolean;
}

export interface KBSkillInfo {
  version: string;
  skill_name: string;
  current_dir: string;
  current_link: string;
  targets: string[] | null;
  files: Record<string, string>;
  file_order: string[] | null;
  builds: KBSkillBuild[] | null;
  zero_token: KBZeroToken;
}

export interface KBPreview {
  version: string;
  current_version: string;
  changed: boolean;
  rule_count: number;
  diff: string;
  errors: string[] | null;
}

export interface KBRuleEdit {
  title: string;
  guidance: string;
  category: string;
  subcategory: string;
  severity: string;
  note: string;
}

export interface KBRuleDetail {
  rule: KBRule;
  evidence: KBComment[] | null;
  merged: KBRule[] | null;
}

export interface KBEvent { stage: string; repo?: string; message: string; run_id?: number; at: number; }

interface KBApi {
  KBOverview(): Promise<KBOverview>;
  KBMeta(): Promise<{ categories: { id: string; title: string }[]; reviewers: string[] | null; running: boolean }>;
  KBRepositories(): Promise<KBRepository[] | null>;
  KBAddRepository(fullName: string, author: string): Promise<void>;
  KBUpdateRepository(id: number, enabled: boolean, author: string): Promise<void>;
  KBResetRepository(id: number): Promise<void>;
  KBRemoveRepository(id: number): Promise<void>;
  KBSettings(): Promise<KBSettings>;
  KBSaveSettings(s: KBSettings): Promise<void>;
  KBRunSync(repoIDs: number[], trigger: string): Promise<void>;
  KBRetryFailed(): Promise<void>;
  KBRuns(limit: number): Promise<KBSyncRun[] | null>;
  KBSearchComments(f: KBCommentFilter): Promise<{ items: KBComment[] | null; total: number }>;
  KBCommentThread(id: number): Promise<KBComment[] | null>;
  KBCommentRevisions(id: number): Promise<string[] | null>;
  KBRules(f: KBRuleFilter): Promise<KBRule[] | null>;
  KBRule(id: number): Promise<KBRuleDetail>;
  KBSetRuleStatus(ids: number[], status: string, note: string): Promise<void>;
  KBEditRule(id: number, e: KBRuleEdit): Promise<void>;
  KBMergeRules(target: number, sources: number[]): Promise<void>;
  KBCreateRuleFromComment(commentID: number, e: KBRuleEdit): Promise<number>;
  KBSetEvidence(ruleID: number, commentID: number, mode: 'manual' | 'excluded' | 'remove'): Promise<void>;
  KBExtract(): Promise<unknown>;
  KBSkillInfo(): Promise<KBSkillInfo>;
  KBPreviewSkill(): Promise<KBPreview>;
  KBRebuildSkill(publish: boolean): Promise<{ build: KBSkillBuild; errors?: string[] }>;
}

/** Typed access to the Wails bindings (undefined in browser/mock mode). */
export const kbApi = (): KBApi | undefined => (window as any).go?.main?.App as KBApi | undefined;

export const EMPTY_COMMENT_FILTER: KBCommentFilter = {
  repo: '', reviewer: '', category: '', kind: '', query: '', since: '', until: '',
  pr_number: 0, include_deleted: false, limit: 100, offset: 0,
};

interface KnowledgeState {
  overview: KBOverview | null;
  categories: { id: string; title: string }[];
  reviewers: string[];
  running: boolean;
  events: KBEvent[];
  error: string | null;
  listening: boolean;

  init: () => Promise<void>;
  refreshOverview: () => Promise<void>;
  runSync: (repoIDs?: number[]) => Promise<void>;
  retryFailed: () => Promise<void>;
  setError: (e: string | null) => void;
  /** Incremented whenever data changed (sync done, rules edited) so views reload. */
  version: number;
  bump: () => void;
}

export const useKnowledgeStore = create<KnowledgeState>((set, get) => ({
  overview: null,
  categories: [],
  reviewers: [],
  running: false,
  events: [],
  error: null,
  listening: false,
  version: 0,

  init: async () => {
    const api = kbApi();
    if (!api) return;
    if (!get().listening && window.runtime) {
      window.runtime.EventsOn('kb:progress', (e: Omit<KBEvent, 'at'>) => {
        // 'done' ends a run; an error without run_id (e.g. lock busy) does not start one.
        const running = e.stage === 'done' ? false : (e.stage === 'error' && !e.run_id) ? undefined : true;
        set(s => ({
          running: running ?? s.running,
          events: [...s.events.slice(-99), { ...e, at: Date.now() }],
        }));
      });
      window.runtime.EventsOn('kb:done', (p: { error?: string }) => {
        set(s => ({ running: false, error: p.error ?? null, version: s.version + 1 }));
        get().refreshOverview();
      });
      set({ listening: true });
    }
    try {
      const meta = await api.KBMeta();
      set({ categories: meta.categories, reviewers: meta.reviewers ?? [], running: meta.running });
    } catch (e) {
      set({ error: String(e) });
    }
    await get().refreshOverview();
  },

  refreshOverview: async () => {
    const api = kbApi();
    if (!api) return;
    try {
      const [overview, meta] = await Promise.all([api.KBOverview(), api.KBMeta()]);
      set({ overview, reviewers: meta.reviewers ?? [], running: meta.running });
    } catch (e) {
      set({ error: String(e) });
    }
  },

  runSync: async (repoIDs = []) => {
    const api = kbApi();
    if (!api) return;
    try {
      set({ error: null, running: true });
      await api.KBRunSync(repoIDs, 'manual');
    } catch (e) {
      set({ error: String(e), running: false });
    }
  },

  retryFailed: async () => {
    const api = kbApi();
    if (!api) return;
    try {
      set({ error: null, running: true });
      await api.KBRetryFailed();
    } catch (e) {
      set({ error: String(e), running: false });
    }
  },

  setError: (error) => set({ error }),
  bump: () => set(s => ({ version: s.version + 1 })),
}));
