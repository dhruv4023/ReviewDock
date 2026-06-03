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
          GetPullRequests(): Promise<PullRequest[]>;
          RebasePRs(requests: RebaseRequest[]): Promise<void>;
          CancelRebase(jobID: string): Promise<void>;
          GetPRCIStatus(repoID: string, headRef: string): Promise<string>;
        };
      };
    };
    runtime: {
      EventsOn(eventName: string, callback: (...args: any[]) => void): void;
      EventsOff(eventName: string): void;
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
  state: 'open' | 'closed' | 'draft';
  is_draft: boolean;
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
}

export interface Settings {
  concurrency_limit: number;
  default_remote_priority: string[];
  amend_commit_timestamp: boolean;
  force_push_after_rebase: boolean;
  auto_refresh_interval_mins: number;
  theme: string;
}

export interface RebaseRequest {
  id: string;
  repo_id: string;
  head_label: string;
  base_label: string;
}

interface AppState {
  repos: Repository[];
  prs: PullRequest[];
  session: Session | null;
  settings: Settings | null;
  selectedPR: PullRequest | null;
  selectedPRIds: string[];
  isLoadingRepos: boolean;
  isLoadingPRs: boolean;
  oauthError: string | null;
  
  // Actions
  init: () => Promise<void>;
  fetchRepos: () => Promise<void>;
  addRepo: (path: string) => Promise<void>;
  removeRepo: (id: string) => Promise<void>;
  fetchPRs: () => Promise<void>;
  login: () => Promise<void>;
  logout: () => Promise<void>;
  setSettings: (settings: Settings) => Promise<void>;
  setSelectedPR: (pr: PullRequest | null) => void;
  toggleSelectPR: (prId: string) => void;
  selectAllPRs: (visiblePrs: PullRequest[]) => void;
  deselectAllPRs: () => void;
  rebaseSelected: () => Promise<void>;
  cancelRebase: (jobID: string) => Promise<void>;
}

export const useAppStore = create<AppState>((set, get) => ({
  repos: [],
  prs: [],
  session: null,
  settings: null,
  selectedPR: null,
  selectedPRIds: [],
  isLoadingRepos: false,
  isLoadingPRs: false,
  oauthError: null,

  init: async () => {
    try {
      const session = await window.go.main.App.GetSession();
      const settings = await window.go.main.App.GetSettings();
      set({ session, settings });

      if (session) {
        await get().fetchRepos();
        await get().fetchPRs();
      }

      // Hook up OAuth Wails runtime events
      window.runtime.EventsOn('oauth:success', (session: Session) => {
        set({ session, oauthError: null });
        get().fetchRepos();
        get().fetchPRs();
      });

      window.runtime.EventsOn('oauth:error', (errorMsg: string) => {
        set({ oauthError: errorMsg });
      });

    } catch (err) {
      console.error('Failed initialization', err);
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

  fetchPRs: async () => {
    if (!get().session) return;
    set({ isLoadingPRs: true });
    try {
      const prs = await window.go.main.App.GetPullRequests();
      set({ prs: prs || [], isLoadingPRs: false });
    } catch (err) {
      console.error('Error fetching pull requests', err);
      set({ isLoadingPRs: false });
    }
  },

  login: async () => {
    try {
      set({ oauthError: null });
      await window.go.main.App.LoginGitHub();
    } catch (err) {
      set({ oauthError: String(err) });
    }
  },

  logout: async () => {
    try {
      await window.go.main.App.Logout();
      set({ session: null, prs: [], repos: [], selectedPRIds: [], selectedPR: null });
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

  setSelectedPR: (pr) => set({ selectedPR: pr }),

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
    const { prs, selectedPRIds } = get();
    const targets = prs.filter(pr => selectedPRIds.includes(pr.id));
    if (targets.length === 0) return;
    const requests: RebaseRequest[] = targets.map(pr => ({
      id: pr.id,
      repo_id: pr.repo_id,
      head_label: pr.head_label,
      base_label: pr.base_label,
    }));

    try {
      await window.go.main.App.RebasePRs(requests);
    } catch (err) {
      console.error('Rebase trigger failed', err);
    }
  },

  cancelRebase: async (jobID) => {
    try {
      await window.go.main.App.CancelRebase(jobID);
    } catch (err) {
      console.error('Cancel failed', err);
    }
  }
}));
