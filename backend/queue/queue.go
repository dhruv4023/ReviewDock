package queue

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/odoo/github-pr/backend/git"
	"github.com/odoo/github-pr/backend/models"
)

type LogFunc func(message string)

type Job struct {
	ID         string // Unique identifier for the job (e.g. repoName-PRNumber)
	RepoName   string
	RepoPath   string
	HeadBranch string
	BaseBranch string
	Options    models.Settings
}

type Manager struct {
	gitExecutor *git.Executor
	jobsChan    chan Job
	activeJobs  map[string]context.CancelFunc
	activeMu    sync.Mutex
	wg          sync.WaitGroup
	workers     int
	logCallback LogFunc
}

func NewManager(workers int, gitExecutor *git.Executor, logCallback LogFunc) *Manager {
	if workers <= 0 {
		workers = 3
	}
	return &Manager{
		gitExecutor: gitExecutor,
		jobsChan:    make(chan Job, 200),
		activeJobs:  make(map[string]context.CancelFunc),
		workers:     workers,
		logCallback: logCallback,
	}
}

func (m *Manager) Start(ctx context.Context) {
	for i := 0; i < m.workers; i++ {
		m.wg.Add(1)
		go m.worker(ctx)
	}
}

func (m *Manager) Submit(job Job) {
	m.jobsChan <- job
	m.log(fmt.Sprintf("\u001b[33m[%s] Queued PR branch '%s' for rebasing onto '%s'\u001b[0m\r\n", job.RepoName, job.HeadBranch, job.BaseBranch))
}

func (m *Manager) Cancel(jobID string) {
	m.activeMu.Lock()
	defer m.activeMu.Unlock()

	if cancel, exists := m.activeJobs[jobID]; exists {
		cancel()
		m.log(fmt.Sprintf("\u001b[31m[QUEUE] Cancellation requested for job: %s\u001b[0m\r\n", jobID))
	}
}

func (m *Manager) Stop() {
	close(m.jobsChan)
	m.wg.Wait()
}

func (m *Manager) log(msg string) {
	if m.logCallback != nil {
		timestamp := time.Now().Format("15:04:05")
		m.logCallback(fmt.Sprintf("[%s] %s", timestamp, msg))
	}
}

func (m *Manager) worker(ctx context.Context) {
	defer m.wg.Done()

	for {
		select {
		case <-ctx.Done():
			return
		case job, ok := <-m.jobsChan:
			if !ok {
				return
			}

			// Create a cancellation context for this specific job, nested under global context
			jobCtx, cancel := context.WithCancel(ctx)
			m.activeMu.Lock()
			m.activeJobs[job.ID] = cancel
			m.activeMu.Unlock()

			m.log(fmt.Sprintf("\u001b[32m[%s] Starting rebase process for branch '%s'...\u001b[0m\r\n", job.RepoName, job.HeadBranch))

			err := m.processRebase(jobCtx, job)

			m.activeMu.Lock()
			delete(m.activeJobs, job.ID)
			m.activeMu.Unlock()
			cancel()

			if err != nil {
				m.log(fmt.Sprintf("\u001b[31m[%s] FAILED rebase workflow: %v\u001b[0m\r\n", job.RepoName, err))
			} else {
				m.log(fmt.Sprintf("\u001b[32m[%s] SUCCESS: PR rebase workflow finished!\u001b[0m\r\n", job.RepoName))
			}
		}
	}
}

func (m *Manager) processRebase(ctx context.Context, job Job) error {
	logger := func(msg string) {
		m.log(fmt.Sprintf("[%s] %s", job.RepoName, msg))
	}

	// 1. Sanity Check: Git Working Tree Clean
	clean, err := m.gitExecutor.IsClean(ctx, job.RepoPath)
	if err != nil {
		return fmt.Errorf("failed checking repository status: %w", err)
	}
	if !clean {
		return fmt.Errorf("local working directory is dirty, please stash or commit your changes first")
	}

	// 2. Fetch all remotes to update tracking branches
	logger("Fetching remote branches...")
	if err := m.gitExecutor.Fetch(ctx, job.RepoPath, logger); err != nil {
		return fmt.Errorf("failed fetching remotes: %w", err)
	}

	// 3. Detect target Remote
	remote, err := m.gitExecutor.DetectBestRemote(ctx, job.RepoPath, job.Options.DefaultRemotePriority)
	if err != nil {
		return fmt.Errorf("failed detecting remote: %w", err)
	}
	logger(fmt.Sprintf("Using remote '%s' for rebase and push", remote))

	// 4. Checkout branch
	logger(fmt.Sprintf("Checking out head branch '%s'...", job.HeadBranch))
	if err := m.gitExecutor.Checkout(ctx, job.RepoPath, job.HeadBranch, logger); err != nil {
		logger("Branch not found locally, checking out from remote tracking branch...")
		if err := m.gitExecutor.CheckoutRemoteBranch(ctx, job.RepoPath, job.HeadBranch, remote, logger); err != nil {
			return fmt.Errorf("failed checking out head branch: %w", err)
		}
	}

	// 5. Rebase onto base branch
	logger(fmt.Sprintf("Rebasing '%s' onto '%s/%s'...", job.HeadBranch, remote, job.BaseBranch))
	if err := m.gitExecutor.Rebase(ctx, job.RepoPath, job.BaseBranch, remote, logger); err != nil {
		logger("Conflict detected! Attempting to abort rebase...")
		_ = m.gitExecutor.RebaseAbort(ctx, job.RepoPath, logger)
		return fmt.Errorf("rebase failed due to merge conflicts: %w", err)
	}

	// 6. Amend commit timestamp if enabled
	if job.Options.AmendCommitTimestamp {
		logger("Amending commit timestamp to current time...")
		if err := m.gitExecutor.AmendTimestamp(ctx, job.RepoPath, logger); err != nil {
			return fmt.Errorf("failed amending commit timestamp: %w", err)
		}
	}

	// 7. Force push branch if enabled
	if job.Options.ForcePushAfterRebase {
		logger(fmt.Sprintf("Force pushing branch '%s' to remote '%s' using safe push lease...", job.HeadBranch, remote))
		if err := m.gitExecutor.ForcePush(ctx, job.RepoPath, remote, job.HeadBranch, logger); err != nil {
			return fmt.Errorf("force push failed: %w", err)
		}
	}

	return nil
}
