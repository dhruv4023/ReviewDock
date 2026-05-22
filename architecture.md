# GitHub Pull Request Rebase Manager - Architectural Blueprint

This document outlines the complete architectural design, project structure, schemas, workflows, and API contracts for building a production-ready, cross-platform desktop application using **Wails v2** (Go backend), **ReactJS** (Frontend), and **TailwindCSS**.

---

## 1. Architectural Overview

The application follows a decoupled desktop architecture where the frontend (ReactJS) runs inside a native web view rendered by Wails, communicating with the Go backend via IPC (inter-process communication) bindings and event emitters.

```mermaid
flowchart TB
    subgraph Frontend [React Frontend (WebView2/WebKit)]
        UI[TailwindCSS UI Components]
        Store[Zustand State Store]
        XTerm[xterm.js Terminal Log Panel]
        ReactQuery[React Query - PR Data]
        
        UI --> Store
        UI --> XTerm
        UI --> ReactQuery
    end

    subgraph IPCBridge [Wails IPC Bridge]
        Bindings[Go Bound Methods]
        Events[Wails Event System]
    end

    subgraph Backend [Go Backend]
        App[App Instance]
        Auth[GitHub OAuth Handler]
        StoreSvc[JSON Storage Service]
        QueueSvc[Parallel Rebase Queue]
        GitSvc[Git CLI Executor]

        App --> Auth
        App --> StoreSvc
        App --> QueueSvc
        App --> GitSvc
    end

    subgraph External [External Resources]
        GH_API[GitHub REST/GraphQL API]
        Git_CLI[Local Git CLI Executable]
        JSON_Files[(Local JSON Storage)]
    end

    %% Communication paths
    Store -. Binding Calls .-> Bindings
    Bindings --> App
    
    %% Event paths (Streaming Logs & Queue updates)
    Events -. Event Emit .-> Store
    Events -. Streaming stdout/stderr .-> XTerm
    QueueSvc --> Events

    %% Backend integrations
    Auth --> GH_API
    GitSvc --> Git_CLI
    StoreSvc --> JSON_Files
```

### Flow Explanation
1. **User Action**: The user selects multiple pull requests in the UI and clicks **Rebase Selected**.
2. **State Transition**: The frontend updates the Zustand store, marking the PRs as "Queued" and sending a binding call `RebasePRs(prIDs)` to the Go backend.
3. **Queue Ingestion**: The backend `QueueSvc` adds the tasks to a thread-safe parallel worker pool.
4. **Execution & Event Streaming**:
   - The worker pool processes tasks. For each task, `GitSvc` spawns `git` commands using `os/exec`.
   - The stdout and stderr are captured in real-time and streamed back to the frontend using the Wails event emitter (`EventsEmit`).
   - The frontend's `xterm.js` component listens for these log events and appends them to the terminal.
5. **Persistence**: Throughout operations, changes in repos or settings are persisted to local JSON files by `StoreSvc`.

---

## 2. Wails Project Structure

Here is the recommended production directory layout:

```text
github-pr/
├── main.go                     # Application entrypoint
├── app.go                      # Wails App lifecycle handlers & Bindings
├── wails.json                  # Wails project configuration
├── go.mod                      # Go module definition
├── go.sum                      # Go dependency lockfile
├── backend/                    # Go backend modules
│   ├── git/
│   │   └── git.go              # Local git command execution
│   ├── github/
│   │   ├── client.go           # GitHub API client (REST & GraphQL)
│   │   └── oauth.go            # OAuth Device Flow login handler
│   ├── storage/
│   │   └── storage.go          # JSON persistence layer (Settings, Repos, Session)
│   ├── queue/
│   │   └── queue.go            # Thread-safe worker queue for parallel rebasing
│   └── models/
│       └── models.go           # Shared Go structs (PR, Repo, Settings)
└── frontend/                   # Frontend files (Vite + React)
    ├── index.html
    ├── package.json
    ├── tailwind.config.js
    ├── tsconfig.json
    ├── vite.config.ts
    └── src/
        ├── main.tsx
        ├── index.css           # Core styling and theme configuration
        ├── App.tsx             # Main layout assembler
        ├── components/         # Reusable presentation/UI components
        │   ├── PRTable.tsx     # Rich PR data table
        │   ├── Sidebar.tsx     # Repository management sidebar
        │   ├── Terminal.tsx    # Live terminal log panel (xterm.js)
        │   └── DetailPanel.tsx # PR description, commits, and CI status panel
        ├── hooks/
        │   └── useWailsEvent.ts# Hook to register listeners for Wails backend events
        ├── stores/
        │   └── appStore.ts     # Zustand store for global application state
        └── services/
            └── wails.ts        # Interface layer for Wails bindings
```

---

## 3. Frontend Component Structure

The frontend is structured to enable a multi-pane layout resembling VSCode or a modern Git client.

### Component Breakdown
1. **`App.tsx`**: Uses CSS grid to define a 4-pane responsive workspace:
   - Sidebar: Left pane (250px)
   - PR Table: Center pane (flexible width)
   - Detail Panel: Right pane (350px, toggleable)
   - Terminal Logs: Bottom pane (300px, resizable)
2. **`PRTable.tsx`**: Renders PR data utilizing Tailwind classes. Implements multi-select checkboxes, badges for conflict state/CI status, sorting, and inline filtering.
3. **`Sidebar.tsx`**: Includes the repository filter input, lists tracked repositories, and houses the "Add Repository" dialog.
4. **`Terminal.tsx`**: Houses the `xterm.js` viewport. Listens for backend `git:log` events and feeds them to the terminal.
5. **`DetailPanel.tsx`**: Displays detailed information of the currently selected PR, listing file changes, commits, description, and action buttons.

---

## 4. Backend Service Structure

The backend consists of clean, isolated packages:

*   **`storage`**: Handles file lock-based concurrent reading and writing of JSON files.
*   **`github`**: Consumes the GitHub API using the user's saved OAuth token. Fetches PR metadata, CI check-run statuses, and mergeability details.
*   **`git`**: Orchestrates external executions of the `git` binary. Executes commands relative to the local repository path.
*   **`queue`**: Manages a queue of Go routines (workers) executing Git operations, ensuring thread-safe logging and job cancellation using `context.Context`.

---

## 5. GitHub Auth Flow

Desktop apps cannot keep client secrets secure. Therefore, we implement the **GitHub OAuth Device Authorization Flow** (RFC 8628). This does not require launching a local web server or handling custom protocol redirects.

```mermaid
sequenceDiagram
    participant User
    participant Frontend
    participant Go Backend
    participant GitHub OAuth

    User->>Frontend: Click "Login with GitHub"
    Frontend->>Go Backend: Request Login
    Go Backend->>GitHub OAuth: POST /login/device/code (client_id, scopes)
    GitHub OAuth-->>Go Backend: Return user_code, verification_uri, device_code, interval
    Go Backend-->>Frontend: Send auth code and verification URL
    Frontend->>User: Display code & open verification URL in browser
    
    par Polling for Completion
        loop Every [interval] seconds
            Go Backend->>GitHub OAuth: POST /login/oauth/access_token (client_id, device_code)
            GitHub OAuth-->>Go Backend: Return "authorization_pending" OR access_token
        end
    and User Verification
        User->>GitHub OAuth: Enter code on verification page & authorize
    end
    
    GitHub OAuth-->>Go Backend: Returns Access Token
    Go Backend->>Go Backend: Save token to sessions.json & fetch user profile
    Go Backend-->>Frontend: Auth success (avatar_url, username)
```

---

## 6. JSON Storage Schema

All configurations are saved inside the user's local application data directory (e.g., `~/.config/github-pr-manager/` on Linux).

### `sessions.json`
Stores the OAuth token and metadata of the authenticated user.
```json
{
  "access_token": "gho_xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx",
  "token_type": "bearer",
  "scope": "repo,read:org",
  "user": {
    "login": "octocat",
    "id": 5832347,
    "avatar_url": "https://avatars.githubusercontent.com/u/5832347?v=4",
    "html_url": "https://github.com/octocat"
  }
}
```

### `repos.json`
Tracks the directories added by the user and their synchronization status.
```json
{
  "repositories": [
    {
      "id": "odoo-odoo",
      "owner": "odoo",
      "name": "odoo",
      "local_path": "/home/user/src/odoo/odoo",
      "sync_status": "synced",
      "last_fetched_at": "2026-05-22T15:20:11Z"
    }
  ]
}
```

### `settings.json`
Stores global preferences and user-specific configurations.
```json
{
  "concurrency_limit": 3,
  "default_remote_priority": ["origin", "upstream", "odoo", "ent"],
  "amend_commit_timestamp": true,
  "force_push_after_rebase": false,
  "auto_refresh_interval_mins": 10,
  "theme": "dark"
}
```

---

## 7. Batch Rebase Workflow

For each selected PR, the backend carries out the following sequential operations inside the repository's path (`local_path`). If any step fails, execution halts immediately and the failure is reported:

```text
[Step 1: Sanity Check]
  Verify local_path exists and is a valid git repository.
  Run: git status --porcelain
  Ensure worktree is clean. If dirty, abort to prevent overwriting uncommitted code.

[Step 2: Fetch Base and Head]
  Run: git fetch --all --prune
  Update all remotes (ensuring the base branch and head branch are up to date).

[Step 3: Checkout Head Branch]
  Verify head branch exists locally.
  Run: git checkout <head_branch>
  If not present locally, check out from remote:
  Run: git checkout -b <head_branch> <remote>/<head_branch>

[Step 4: Execute Rebase]
  Run: git rebase <remote>/<base_branch>
  Monitor stdout/stderr in real-time.
  If rebase conflict occurs:
    Run: git rebase --abort
    Report conflict error. Exit.

[Step 5: Optional Amend Timestamp]
  If "amend_commit_timestamp" is enabled:
    Run: git commit --amend --no-edit --date=now
    Updates the author date and commit date of the head commit.

[Step 6: Optional Force Push]
  If "force_push_after_rebase" is enabled:
    Run: git push --force-with-lease <remote> <head_branch>
    (Uses --force-with-lease instead of --force for safety).
```

---

## 8. UI Wireframe Idea

```text
+--------------------------------------------------------------------------------------------------+
|  [App Icon]  PR Rebase Manager   [Search Repos...]            [Octocat] User Profile V  [Settings] |
+---------------------------------------------------+-----------------------------+----------------+
| REPOSITORIES                         [+ Add Repo] | PULL REQUESTS               | PR DETAILS     |
| > odoo/odoo                      (Synced) [Sync]  | [Filter: Open v] [Exclude ] | #10485: Fix PR |
|   /home/odoo/src/odoo/odoo                        | [Bulk Actions: Rebase v] [Run] | Authored by:   |
| > odoo/enterprise                (Synced) [Sync]  | [x] PR #  Title   Base  Head| @octocat       |
|   /home/odoo/src/odoo/enterprise                  | [x] 10485 Fix base main  patch1|                |
| > custom/private-repo            (Error)  [Sync]  | [ ] 10471 Bump dep  main  dep-up| [Open in Browser]|
|                                                   | [x] 10399 Add logs  saas-19 feat-log|                |
|                                                   +-----------------------------| CI Checks:      |
|                                                   | [x] 2 Selected  [Rebase & Push] | - Build: Success|
|                                                   |                             | - Lint: Pending |
|                                                   |                             |                |
|                                                   |                             | Commits:       |
|                                                   |                             | - Fix memory leak|
|                                                   |                             | - Add debug logging|
+---------------------------------------------------+-----------------------------+----------------+
| TERMINAL LOG PANEL                                                                 [x] Clear  [-] |
| [16:21:00] [odoo/odoo] Initializing rebase for PR #10485...                                       |
| [16:21:01] [odoo/odoo] Executing: git checkout patch1                                            |
| [16:21:02] [odoo/odoo] Executing: git rebase origin/main                                         |
| [16:21:03] [odoo/odoo] Successfully rebased patch1 onto origin/main                              |
| [16:21:04] [odoo/odoo] Executing: git commit --amend --no-edit --date=now                         |
| [16:21:05] [odoo/odoo] Executing: git push --force-with-lease origin patch1                       |
| [16:21:07] [odoo/odoo] PR #10485 completed successfully!                                          |
+--------------------------------------------------------------------------------------------------+
```

---

## 9. Recommended Libraries

### Backend (Go)
1.  **`github.com/wailsapp/wails/v2`**: Desktop application framework.
2.  **`github.com/google/go-github/v53`**: Full-featured GitHub API client.
3.  **`golang.org/x/oauth2`**: Standard package for handling OAuth credentials and token rotation.
4.  **`github.com/gofrs/flock`**: Thread-safe and cross-platform file locking utility for JSON state files.

### Frontend (React)
1.  **`@xterm/xterm` & `@xterm/addon-fit`**: High-performance terminal emulator component and layout adapter.
2.  **`zustand`**: Light, robust state management.
3.  **`lucide-react`**: Sleek modern icons.
4.  **`@tanstack/react-query`**: Caching and synchronization of API data from GitHub.
5.  **`shadcn/ui` (built on TailwindCSS + Radix UI)**: Professional, design-system-based accessible UI components.

---

## 10. Suggested Concurrency Model in Go

To perform parallel rebases safely, use a **worker pool pattern** with Go channels and structured context cancellation.

```go
package queue

import (
	"context"
	"sync"
)

type Job struct {
	ID         string
	RepoPath   string
	HeadBranch string
	BaseBranch string
	Remote     string
	Options    RebaseOptions
}

type RebaseOptions struct {
	AmendTimestamp bool
	ForcePush      bool
}

type JobResult struct {
	JobID string
	Error error
}

type Manager struct {
	jobsChan   chan Job
	resultsChan chan JobResult
	activeJobs  map[string]context.CancelFunc
	mu          sync.Mutex
	wg          sync.WaitGroup
}

func NewManager(workers int) *Manager {
	m := &Manager{
		jobsChan:   make(chan Job, 100),
		resultsChan: make(chan JobResult, 100),
		activeJobs:  make(map[string]context.CancelFunc),
	}
	m.start(workers)
	return m
}

func (m *Manager) start(workers int) {
	for i := 0; i < workers; i++ {
		m.wg.Add(1)
		go func() {
			defer m.wg.Done()
			for job := range m.jobsChan {
				// Create cancellable context for each specific job
				ctx, cancel := context.WithCancel(context.Background())
				m.mu.Lock()
				m.activeJobs[job.ID] = cancel
				m.mu.Unlock()

				// Execute job logic
				err := executeRebaseJob(ctx, job)

				m.mu.Lock()
				delete(m.activeJobs, job.ID)
				m.mu.Unlock()

				m.resultsChan <- JobResult{JobID: job.ID, Error: err}
			}
		}()
	}
}

func (m *Manager) Submit(job Job) {
	m.jobsChan <- job
}

func (m *Manager) CancelJob(jobID string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if cancel, exists := m.activeJobs[jobID]; exists {
		cancel()
	}
}

func executeRebaseJob(ctx context.Context, job Job) error {
	// Execute shell commands checking ctx.Done() at each step
	return nil
}
```

---

## 11. Example API Contracts

Here are the bindings contracts that bridge the frontend and backend.

### Go Application Interface (Bindings exposed to React)

```go
type App struct {
	storageSvc *storage.Service
	githubSvc  *github.Service
	queueSvc   *queue.Manager
}

// GetRepositories returns the list of tracked repositories
func (a *App) GetRepositories() ([]models.Repository, error)

// AddRepository adds a repository path and metadata
func (a *App) AddRepository(path string) (models.Repository, error)

// RemoveRepository removes a repo by ID
func (a *App) RemoveRepository(id string) error

// LoginGitHub initiates the device login flow, emitting events for frontend
func (a *App) LoginGitHub() error

// GetPullRequests fetches PRs for all tracked repos authored by the user
func (a *App) GetPullRequests() ([]models.PullRequest, error)

// RebasePRs submits selected PR jobs to the queue
func (a *App) RebasePRs(jobRequests []models.RebaseRequest) error

// CancelRebase cancels an active job
func (a *App) CancelRebase(jobID string) error
```

### TypeScript Definitions (`frontend/src/bindings.d.ts`)

```typescript
export interface Repository {
  id: string;
  owner: string;
  name: string;
  local_path: string;
  sync_status: 'synced' | 'out_of_sync' | 'error';
  last_fetched_at: string;
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
  mergeable: 'mergeable' | 'conflicting' | 'unknown';
}

export interface RebaseRequest {
  id: string;
  repo_id: string;
  head_branch: string;
  base_branch: string;
}
```

---

## 12. Error Handling Strategy

A robust error handling mechanism is critical for git actions.

### Conflict Capture
When running `git rebase`, we look for exit code `1` and specific matching strings in stderr:
- **`CONFLICT (content): Merge conflict in...`**: Indicates direct merge conflict.
- **`error: Your local changes to the following files would be overwritten by merge:`**: Uncommitted workspace files.
- **`fatal: Need to specify how to reconcile divergent branches.`**: Configuration error.

### Reporting
Errors are formatted with ANSI escape sequences (`\x1b[31m` for Red text) and sent to the UI via Wails events.
```go
func streamLogError(repoName string, prNumber int, msg string) {
    // \u001b[31m makes text red in xterm.js
    formattedMsg := fmt.Sprintf("\u001b[31m[%s] [PR #%d] ERROR: %s\u001b[0m\r\n", repoName, prNumber, msg)
    wails.EventsEmit(ctx, "git:log", formattedMsg)
}
```

---

## 13. Production-Ready Best Practices

1.  **Command Sanitization**:
    *   Never concatenate input parameters inside a raw terminal shell command.
    *   Always use `os/exec.CommandContext(ctx, "git", "rebase", baseBranch)` rather than `bash -c` execution to avoid shell injection.
2.  **Verify local Git installation**:
    *   At backend startup, run `git --version` to check if Git CLI is installed and accessible. Inform the user gracefully with a splash screen notification if not.
3.  **Use Safe Pushes**:
    *   Never use standard `git push -f` or `--force`.
    *   Use `--force-with-lease` to prevent overwriting commits pushed by other team members in the interim.
4.  **Auto-Pruning and Garbage Collection**:
    *   Maintain local repository health by adding standard `--prune` flags on git fetches to clear deleted remote tracking branches.
5.  **Secure Storage**:
    *   Ensure configuration files have restricted permission access masks (e.g., `0600` on Unix systems) to protect stored OAuth tokens from unauthorized local users.
