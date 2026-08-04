import { create } from 'zustand';

// Declare Wails bindings globally for TypeScript safety
declare global {
  interface Window {
    go: {
      main: {
        App: {
          GetRepositories(): Promise<Repository[]>;
          AddRepository(path: string): Promise<Repository>;
          RemoveRepository(id: string): Promise<void>;
          GetSettings(): Promise<Settings>;
          SaveSettings(settings: Settings): Promise<void>;
          GetSession(): Promise<Session | null>;
          Logout(): Promise<void>;
          LoginGitHub(): Promise<void>;
          GetPullRequests(remoteUpdate: boolean): Promise<PullRequest[]>;
          RebasePRs(requests: RebaseRequest[]): Promise<void>;
          CancelRebase(jobID: string): Promise<void>;
          GetPRCIStatus(repoID: string, headRef: string): Promise<string>;
          GetRemotes(repoID: string): Promise<string[]>;
          SetBranchTracking(repoID: string, branch: string, remote: string): Promise<void>;
          GetPRDiff(repoID: string, baseLabel: string, headBranch: string): Promise<string>;
          GetReviewTemplate(): Promise<string>;
          SaveReviewTemplate(template: string): Promise<void>;
          IsDev(): Promise<boolean>;
          GetPRStats(prID: string, repoID: string, prNumber: number, prUpdatedAt: string, author: string): Promise<PRStats>;
          GetDashboardStats(inputs: PRDashboardInput[]): Promise<DashboardStats>;
        };
      };
    };
    runtime: {
      EventsOn(eventName: string, callback: (...args: any[]) => void): void;
      EventsOff(eventName: string): void;
      BrowserOpenURL(url: string): void;
      ClipboardSetText(text: string): Promise<boolean>;
      ClipboardGetText(): Promise<string>;
    };
  }
}

export interface Repository {
  id: string;
  owner: string;
  name: string;
  local_path: string;
  sync_status: 'synced' | 'error' | 'pending';
  last_fetched_at: string;
}

export interface User {
  login: string;
  id: number;
  avatar_url: string;
  html_url: string;
}

export interface Session {
  access_token: string;
  token_type: string;
  scope: string;
  user: User;
}

export interface PullRequest {
  id: string;
  number: number;
  title: string;
  repo_id: string;
  repo_name: string;
  base_branch: string;
  head_branch: string;
  state: 'open' | 'closed' | 'draft' | 'merged';
  is_draft: boolean;
  created_at: string;
  updated_at: string;
  behind_count: number;
  ahead_count: number;
  local_ahead_count: number;
  local_behind_count: number;
  mergeable_status: 'mergeable' | 'conflicting' | 'unknown';
  html_url: string;
  base_label: string;
  head_label: string;
  description: string;
  author: string;
  labels: string[];
  requested_reviewers: string[];
}

/** Computed statistics for a single pull request, returned by GetPRStats. */
export interface PRStats {
  total_comments: number;
  pr_conversation_comments: number; // issue/timeline comments on the PR
  review_comments: number;
  author_replies: number;
  review_comments_by_reviewer: Record<string, number>;
  reviewer_states: Record<string, string>; // APPROVED / CHANGES_REQUESTED / COMMENTED / DISMISSED
  commits: number;
  changed_files: number;
  additions: number;
  deletions: number;
  directly_merged: boolean;
  pr_age_days: number;
  labels: string[];
  author: string;
  created_at: string;
  last_updated: string;
  from_cache: boolean;
}

/** Activity summary for one reviewer across all tracked PRs (used in DashboardStats). */
export interface ReviewerSummary {
  approved: number;
  changes_requested: number;
  commented: number;
  total_comments: number;
}

/** Aggregate statistics across all tracked pull requests. */
export interface DashboardStats {
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
  prs_by_month: Record<string, number>; // "2024-01" → count
  stale_recomputed: number;
  cache_hits: number;
  last_updated: string;
}

/** Minimal PR data sent to the backend when requesting dashboard statistics. */
interface PRDashboardInput {
  id: string;
  repo_id: string;
  number: number;
  updated_at: string;
  created_at: string;
  state: string;
  is_draft: boolean;
  repo_name: string;
}

export interface Settings {
  concurrency_limit: number;
  amend_commit_timestamp: boolean;
  force_push_after_rebase: boolean;
  theme: string;
  cron_enabled: boolean;
  cron_times: string[];
  /** When true, draft PRs are included in scheduled auto-rebase runs. */
  cron_include_drafts: boolean;
}

export interface RebaseRequest {
  id: string;
  repo_id: string;
  head_label: string;
  base_label: string;
}

// Represents a PR that needs a remote configured before its rebase job can be submitted.
export interface PendingRemoteSetup {
  /** The PR whose head branch has no remote tracking. */
  pr: PullRequest;
  /** Available remotes fetched from the backend. */
  remotes: string[];
  /** The full queue of rebase requests waiting to be submitted (includes this PR). */
  pendingRequests: RebaseRequest[];
  // When true, RebasePRs is called after all remotes are resolved. False = tracking-only (WiFi icon).
  submitAfterSetup: boolean;
}

export interface RebaseJobState {
  status: 'queued' | 'running' | 'success' | 'failed';
  error?: string;
}

interface AppState {
  repos: Repository[];
  prs: PullRequest[];
  session: Session | null;
  settings: Settings | null;
  selectedPR: PullRequest | null;
  selectedPRIds: string[];
  isCheckingSession: boolean;
  deviceCode: string | null;
  deviceUrl: string | null;
  isLoadingRepos: boolean;
  isLoadingPRs: boolean;
  /** True while a rebase (or rebase+force-push) operation is in flight. */
  isRebasing: boolean;
  activeRebaseCount: number;
  queuedRebaseCount: number;
  rebaseJobs: Record<string, RebaseJobState>;
  oauthError: string | null;
  /** Set when a PR's head branch has no remote tracking; drives the RemoteSetupModal. */
  pendingRemoteSetup: PendingRemoteSetup | null;
  /** The current PR review prompt template (empty string = use default). */
  reviewTemplate: string;
  /** Statistics for the currently-selected PR (null until fetched). */
  prStats: PRStats | null;
  /** True while GetPRStats is in-flight. */
  isPRStatsLoading: boolean;
  /** Aggregate dashboard stats across all PRs (null until first fetch). */
  dashboardStats: DashboardStats | null;
  /** True while GetDashboardStats is in-flight. */
  isDashboardLoading: boolean;

  // Actions
  init: () => Promise<void>;
  fetchRepos: () => Promise<void>;
  addRepo: (path: string) => Promise<void>;
  removeRepo: (id: string) => Promise<void>;
  fetchPRs: (remoteUpdate?: boolean) => Promise<void>;
  login: () => Promise<void>;
  logout: () => Promise<void>;
  setSettings: (settings: Settings) => Promise<void>;
  setSelectedPR: (pr: PullRequest | null) => void;
  toggleSelectPR: (prId: string) => void;
  selectAllPRs: (visiblePrs: PullRequest[]) => void;
  deselectAllPRs: () => void;
  rebaseSelected: () => Promise<void>;
  /** Rebases a single PR directly (used by DetailsPanel). Does NOT touch selectedPRIds. */
  rebaseSinglePR: (prId: string, amend: boolean, push: boolean) => Promise<void>;
  cancelRebase: (jobID: string) => Promise<void>;
  /** Called by RemoteSetupModal when the user picks a remote and confirms. */
  confirmRemoteSetup: (remote: string) => Promise<void>;
  /** Called by RemoteSetupModal when the user skips the current PR. */
  skipRemoteSetup: () => Promise<void>;
  /** @internal Walks pending requests, opens modal for the first untracked branch or submits all. */
  _processNextRemoteSetup: (requests: RebaseRequest[], submitAfterSetup: boolean) => Promise<void>;
  /** Loads the review template from the backend into the store. */
  fetchReviewTemplate: () => Promise<void>;
  /** Saves the review template to the backend and updates the store. */
  saveReviewTemplate: (template: string) => Promise<void>;
  /** Fetches (or loads from cache) statistics for the given PR. */
  fetchPRStats: (pr: PullRequest) => Promise<void>;
  /** Clears the current PR stats (called on PR deselect or manual refresh). */
  clearPRStats: () => void;
  /** Fetches aggregate dashboard stats (incremental: only recomputes stale PRs). */
  fetchDashboardStats: () => Promise<void>;
}

export const useAppStore = create<AppState>((set, get) => ({
  repos: [],
  prs: [],
  session: null,
  settings: null,
  selectedPR: null,
  selectedPRIds: [],
  isCheckingSession: true,
  deviceCode: null,
  deviceUrl: null,
  isLoadingRepos: false,
  isLoadingPRs: false,
  isRebasing: false,
  activeRebaseCount: 0,
  queuedRebaseCount: 0,
  rebaseJobs: {},
  oauthError: null,
  pendingRemoteSetup: null,
  reviewTemplate: '',
  prStats: null,
  isPRStatsLoading: false,
  dashboardStats: null,
  isDashboardLoading: false,

  init: async () => {
    try {
      set({ isCheckingSession: true });
      
      if (!window.go || !window.go.main || !window.go.main.App) {
        console.warn('Wails bindings not found. Running in browser/mock mode.');
        set({ isCheckingSession: false });
        return;
      }

      const session = await window.go.main.App.GetSession();
      const settings = await window.go.main.App.GetSettings();
      set({ session, settings, isCheckingSession: false });

      // Load review template regardless of session state
      await get().fetchReviewTemplate();

      if (session) {
        await get().fetchRepos();
        let isDev = false;
        try {
          isDev = await window.go.main.App.IsDev();
        } catch (e) {
          console.error('Failed checking dev mode', e);
        }
        await get().fetchPRs(!isDev);
      }

      // Hook up OAuth Wails runtime events
      if (window.runtime) {
        window.runtime.EventsOn('oauth:success', (session: Session) => {
          set({ session, oauthError: null, deviceCode: null, deviceUrl: null });
          get().fetchRepos();
          get().fetchPRs();
        });

        window.runtime.EventsOn('oauth:error', (errorMsg: string) => {
          set({ oauthError: errorMsg, deviceCode: null, deviceUrl: null });
        });

        window.runtime.EventsOn('oauth:device_code', (data: { code: string; url: string }) => {
          set({ deviceCode: data.code, deviceUrl: data.url });
        });

        window.runtime.EventsOn('rebase:status', (data: { job_id: string; status: 'queued' | 'running' | 'success' | 'failed'; error?: string; active_count: number; queued_count: number }) => {
          const { rebaseJobs } = get();
          const nextJobs = { ...rebaseJobs };
          nextJobs[data.job_id] = {
            status: data.status,
            error: data.error,
          };
          set({
            rebaseJobs: nextJobs,
            activeRebaseCount: data.active_count,
            queuedRebaseCount: data.queued_count,
            isRebasing: (data.active_count + data.queued_count) > 0,
          });

          // Fetch fresh PRs on success so the sync status/counts update
          if (data.status === 'success') {
            get().fetchPRs(false);
          }
        });
      }

    } catch (err) {
      console.error('Failed initialization', err);
      set({ isCheckingSession: false });
    }
  },

  fetchRepos: async () => {
    set({ isLoadingRepos: true });
    try {
      const repos = await window.go.main.App.GetRepositories();
      set({ repos: repos || [], isLoadingRepos: false });
    } catch (err) {
      console.error('Error fetching repos', err);
      set({ isLoadingRepos: false });
    }
  },

  addRepo: async (path: string) => {
    try {
      await window.go.main.App.AddRepository(path);
      await get().fetchRepos();
    } catch (err) {
      alert(err);
      throw err;
    }
  },

  removeRepo: async (id: string) => {
    try {
      await window.go.main.App.RemoveRepository(id);
      await get().fetchRepos();
    } catch (err) {
      console.error('Error removing repo', err);
    }
  },

  fetchPRs: async (remoteUpdate: boolean = false) => {
    if (!get().session) return;
    set({ isLoadingPRs: true });
    try {
      const prs = await window.go.main.App.GetPullRequests(remoteUpdate);
      set({ prs: prs || [], isLoadingPRs: false });
    } catch (err) {
      console.error('Error fetching pull requests', err);
      set({ isLoadingPRs: false });
    }
  },

  login: async () => {
    try {
      set({ oauthError: null, deviceCode: null, deviceUrl: null });
      await window.go.main.App.LoginGitHub();
    } catch (err) {
      set({ oauthError: String(err) });
    }
  },

  logout: async () => {
    try {
      await window.go.main.App.Logout();
      set({ session: null, prs: [], repos: [], selectedPRIds: [], selectedPR: null, deviceCode: null, deviceUrl: null });
    } catch (err) {
      console.error('Logout error', err);
    }
  },

  setSettings: async (settings: Settings) => {
    try {
      await window.go.main.App.SaveSettings(settings);
      set({ settings });
    } catch (err) {
      console.error('Error saving settings', err);
    }
  },

  setSelectedPR: (pr) => set({ selectedPR: pr, prStats: null, isPRStatsLoading: false }),

  toggleSelectPR: (prId) => {
    const selected = get().selectedPRIds;
    if (selected.includes(prId)) {
      set({ selectedPRIds: selected.filter(id => id !== prId) });
    } else {
      set({ selectedPRIds: [...selected, prId] });
    }
  },

  selectAllPRs: (visiblePrs) => {
    set({ selectedPRIds: visiblePrs.map(pr => pr.id) });
  },

  deselectAllPRs: () => set({ selectedPRIds: [] }),

  rebaseSelected: async () => {
    const { prs, selectedPRIds, rebaseJobs } = get();
    const targets = prs.filter(pr => selectedPRIds.includes(pr.id));
    if (targets.length === 0) return;

    // Sort by head_branch so all PRs on the same branch are submitted
    // contiguously — the backend group coordinator groups by branch name and
    // only triggers a coordinated force-push once ALL members have rebased.
    // Submitting them together maximises the chance they are grouped correctly.
    const sorted = [...targets].sort((a, b) =>
      (a.head_branch || '').localeCompare(b.head_branch || '')
    );

    const requests: RebaseRequest[] = sorted.map(pr => ({
      id: pr.id,
      repo_id: pr.repo_id,
      head_label: pr.head_label,
      base_label: pr.base_label,
    }));

    // Clear old status for targets
    const nextJobs = { ...rebaseJobs };
    targets.forEach(t => {
      delete nextJobs[t.id];
    });
    set({ rebaseJobs: nextJobs });

    // Check each request for missing remote tracking.
    // A head_label without "/" means GetUpstreamByBranch failed — no tracking configured.
    // submitAfterSetup=true: once all remotes resolved, submit jobs to queue.
    await get()._processNextRemoteSetup(requests, true);
  },

  rebaseSinglePR: async (prId: string, amend: boolean, push: boolean) => {
    const { prs, settings, rebaseJobs } = get();
    const pr = prs.find(p => p.id === prId);
    if (!pr || !settings) return;

    // Save settings with the desired amend/push flags
    const updatedSettings = {
      ...settings,
      amend_commit_timestamp: amend,
      force_push_after_rebase: push,
    };
    await window.go.main.App.SaveSettings(updatedSettings);
    set({ settings: updatedSettings });

    const requests: RebaseRequest[] = [{
      id: pr.id,
      repo_id: pr.repo_id,
      head_label: pr.head_label,
      base_label: pr.base_label,
    }];

    // Clear old status for target
    const nextJobs = { ...rebaseJobs };
    delete nextJobs[pr.id];
    set({ rebaseJobs: nextJobs });

    await get()._processNextRemoteSetup(requests, true);
  },

  // Internal: walk the pending requests list; for the first one that lacks a remote,
  // open the modal. Otherwise submit all to the backend (only if submitAfterSetup=true).
  _processNextRemoteSetup: async (requests: RebaseRequest[], submitAfterSetup: boolean) => {
    const { prs } = get();

    for (let i = 0; i < requests.length; i++) {
      const req = requests[i];
      if (!req.head_label.includes('/')) {
        // No remote tracked — find the PR object and fetch available remotes
        const pr = prs.find(p => p.id === req.id);
        if (!pr) continue;
        try {
          const remotes = await window.go.main.App.GetRemotes(req.repo_id);
          set({
            pendingRemoteSetup: {
              pr,
              remotes: remotes || [],
              pendingRequests: requests,
              submitAfterSetup,
            },
          });
          // Modal takes over from here; confirmRemoteSetup / skipRemoteSetup will continue.
          return;
        } catch (err) {
          console.error('Failed fetching remotes for', pr.head_branch, err);
          // Skip this PR and continue
        }
      }
    }

    // All requests have valid head_labels.
    set({ pendingRemoteSetup: null });
    if (submitAfterSetup) {
      // Rebase button path: submit jobs to the queue.
      try {
        await window.go.main.App.RebasePRs(requests);
      } catch (err) {
        console.error('Rebase trigger failed', err);
      }
    }
    // WiFi icon path (submitAfterSetup=false): tracking is set, just close — nothing more to do.
  },

  confirmRemoteSetup: async (remote: string) => {
    const setup = get().pendingRemoteSetup;
    if (!setup) return;

    const { pr, pendingRequests } = setup;
    try {
      // Set tracking on the backend
      await window.go.main.App.SetBranchTracking(pr.repo_id, pr.head_branch, remote);

      // Update the head_label in the local prs list so the table reflects it
      const updatedPRs = get().prs.map(p =>
        p.id === pr.id
          ? { ...p, head_label: `${remote}/${p.head_branch}` }
          : p
      );
      set({ prs: updatedPRs });

      // Update the head_label in the pending request list too
      const updatedRequests = pendingRequests.map(req =>
        req.id === pr.id
          ? { ...req, head_label: `${remote}/${pr.head_branch}` }
          : req
      );

      // Continue processing the (now-updated) queue, preserving the submit intent.
      await get()._processNextRemoteSetup(updatedRequests, setup.submitAfterSetup);
    } catch (err) {
      console.error('SetBranchTracking failed', err);
      set({ pendingRemoteSetup: null });
    }
  },

  skipRemoteSetup: async () => {
    const setup = get().pendingRemoteSetup;
    if (!setup) return;

    // Remove the skipped PR from the pending batch and continue
    const { pr, pendingRequests } = setup;
    const remaining = pendingRequests.filter(req => req.id !== pr.id);
    await get()._processNextRemoteSetup(remaining, setup.submitAfterSetup);
  },

  cancelRebase: async (jobID) => {
    try {
      await window.go.main.App.CancelRebase(jobID);
    } catch (err) {
      console.error('Cancel failed', err);
    }
  },

  fetchReviewTemplate: async () => {
    try {
      const tmpl = await window.go.main.App.GetReviewTemplate();
      set({ reviewTemplate: tmpl || '' });
    } catch (err) {
      console.error('Failed to load review template', err);
    }
  },

  saveReviewTemplate: async (template: string) => {
    try {
      await window.go.main.App.SaveReviewTemplate(template);
      set({ reviewTemplate: template });
    } catch (err) {
      console.error('Failed to save review template', err);
      throw err;
    }
  },

  fetchPRStats: async (pr: PullRequest) => {
    if (!window.go?.main?.App) return;
    set({ isPRStatsLoading: true });
    try {
      const result = await window.go.main.App.GetPRStats(
        pr.id,
        pr.repo_id,
        pr.number,
        pr.updated_at,
        pr.author ?? '',
      );
      set({ prStats: result, isPRStatsLoading: false });
    } catch (err) {
      console.error('Failed to fetch PR stats', err);
      set({ isPRStatsLoading: false });
    }
  },

  clearPRStats: () => set({ prStats: null, isPRStatsLoading: false }),

  fetchDashboardStats: async () => {
    if (!window.go?.main?.App) return;
    const { prs } = get();
    if (!prs.length) return;
    set({ isDashboardLoading: true });
    try {
      const inputs: PRDashboardInput[] = prs.map(pr => ({
        id: pr.id,
        repo_id: pr.repo_id,
        number: pr.number,
        updated_at: pr.updated_at,
        created_at: pr.created_at,
        state: pr.state,
        is_draft: pr.is_draft,
        repo_name: pr.repo_name,
      }));
      const result = await window.go.main.App.GetDashboardStats(inputs);
      set({ dashboardStats: result, isDashboardLoading: false });
    } catch (err) {
      console.error('Failed to fetch dashboard stats', err);
      set({ isDashboardLoading: false });
    }
  },
}));
