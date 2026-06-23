package queue

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"my-github-pr/backend/git"
	"my-github-pr/backend/models"
)

type LogFunc func(message string)
type StatusFunc func(jobID string, status string, errMsg string, active int, queued int)

type GitExecutor interface {
	IsClean(ctx context.Context, dir string) (bool, error)
	Fetch(ctx context.Context, dir string, log git.LogWriter) error
	Checkout(ctx context.Context, dir string, branch string, log git.LogWriter) error
	Rebase(ctx context.Context, dir string, baseBranch string, remote string, log git.LogWriter) error
	RebaseAbort(ctx context.Context, dir string, log git.LogWriter) error
	AmendTimestamp(ctx context.Context, dir string, log git.LogWriter) error
	ForcePush(ctx context.Context, dir string, remote string, branch string, log git.LogWriter) error
}

type JobState string

const (
	statePending  JobState = "pending"
	stateRebasing JobState = "rebasing"
	stateRebased  JobState = "rebased"
	statePushing  JobState = "pushing"
	stateSuccess  JobState = "success"
	stateFailed   JobState = "failed"
)

type Job struct {
	ID        string // Unique identifier for the job (e.g. repoName-PRNumber)
	RepoName  string
	RepoPath  string
	HeadLabel string
	BaseLabel string
	Options   models.Settings

	// Scheduling & grouping fields
	State  JobState
	Error  error
	Cancel context.CancelFunc
	JobCtx context.Context
}

func (j *Job) GetBranchName() string {
	parts := strings.Split(j.HeadLabel, "/")
	if len(parts) == 2 {
		return parts[1]
	}
	return j.HeadLabel
}

type Manager struct {
	gitExecutor    GitExecutor
	jobs           []*Job
	workers        int
	mu             sync.Mutex
	wg             sync.WaitGroup
	notifyChan     chan struct{}
	logCallback    LogFunc
	statusCallback StatusFunc
	cancel         context.CancelFunc
}

func NewManager(workers int, gitExecutor GitExecutor, logCallback LogFunc, statusCallback StatusFunc) *Manager {
	if workers <= 0 {
		workers = 3
	}
	return &Manager{
		gitExecutor:    gitExecutor,
		workers:        workers,
		notifyChan:     make(chan struct{}, workers),
		logCallback:    logCallback,
		statusCallback: statusCallback,
	}
}

func (m *Manager) Start(ctx context.Context) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var startCtx context.Context
	startCtx, m.cancel = context.WithCancel(ctx)
	for i := 0; i < m.workers; i++ {
		m.wg.Add(1)
		go m.worker(startCtx)
	}
}

func (m *Manager) Submit(job Job) {
	m.mu.Lock()

	j := &Job{
		ID:        job.ID,
		RepoName:  job.RepoName,
		RepoPath:  job.RepoPath,
		HeadLabel: job.HeadLabel,
		BaseLabel: job.BaseLabel,
		Options:   job.Options,
		State:     statePending,
	}
	m.jobs = append(m.jobs, j)

	m.log(fmt.Sprintf("\u001b[33m[%s] Queued PR branch '%s' for rebasing onto '%s'\u001b[0m\r\n", j.RepoName, j.HeadLabel, j.BaseLabel))
	m.reportStatusLocked(j.ID, "queued", "")
	m.mu.Unlock()

	m.broadcast()
}

func (m *Manager) Cancel(jobID string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	for _, j := range m.jobs {
		if j.ID == jobID {
			if j.State == statePending {
				j.State = stateFailed
				j.Error = fmt.Errorf("job cancelled by user")
				m.log(fmt.Sprintf("\u001b[31m[%s] Cancelled by user while pending.\u001b[0m\r\n", j.RepoName))
				m.reportStatusLocked(j.ID, "failed", "cancelled by user")
				m.handleGroupFailureLocked(j)
			} else if j.State == stateRebasing || j.State == stateRebased || j.State == statePushing {
				if j.Cancel != nil {
					j.Cancel()
				}
				j.State = stateFailed
				j.Error = fmt.Errorf("job cancelled by user")
				m.log(fmt.Sprintf("\u001b[31m[%s] Cancelled by user.\u001b[0m\r\n", j.RepoName))
				m.reportStatusLocked(j.ID, "failed", "cancelled by user")
				m.handleGroupFailureLocked(j)
			}
			break
		}
	}
	m.cleanCompletedJobsLocked()
	m.broadcast()
}

func (m *Manager) Stop() {
	m.mu.Lock()
	if m.cancel != nil {
		m.cancel()
	}
	for _, j := range m.jobs {
		if j.Cancel != nil {
			j.Cancel()
		}
	}
	m.mu.Unlock()
	m.wg.Wait()
}

func (m *Manager) log(msg string) {
	if m.logCallback != nil {
		m.logCallback(msg)
	}
}

func (m *Manager) worker(ctx context.Context) {
	defer m.wg.Done()

	for {
		select {
		case <-ctx.Done():
			return
		default:
			job := m.acquireNextJob(ctx)
			if job == nil {
				select {
				case <-ctx.Done():
					return
				case <-m.notifyChan:
					continue
				}
			}

			m.runJob(ctx, job)
			m.broadcast()
		}
	}
}

func (m *Manager) acquireNextJob(ctx context.Context) *Job {
	m.mu.Lock()
	defer m.mu.Unlock()

	// 1. Identify which repos are currently busy (any job in rebasing, rebased, or pushing state)
	busyRepos := make(map[string]bool)
	for _, j := range m.jobs {
		if j.State == stateRebasing || j.State == stateRebased || j.State == statePushing {
			busyRepos[j.RepoPath] = true
		}
	}

	// 2. Find the first pending job whose repo is NOT busy
	for _, j := range m.jobs {
		if j.State == statePending && !busyRepos[j.RepoPath] {
			jobCtx, cancel := context.WithCancel(ctx)
			j.State = stateRebasing
			j.Cancel = cancel
			j.JobCtx = jobCtx

			m.reportStatusLocked(j.ID, "running", "")
			return j
		}
	}

	return nil
}

func (m *Manager) broadcast() {
	for i := 0; i < m.workers; i++ {
		select {
		case m.notifyChan <- struct{}{}:
		default:
		}
	}
}

func (m *Manager) reportStatusLocked(jobID string, status string, errMsg string) {
	if m.statusCallback != nil {
		active := 0
		queued := 0
		for _, j := range m.jobs {
			if j.State == statePending {
				queued++
			} else if j.State == stateRebasing || j.State == stateRebased || j.State == statePushing {
				active++
			}
		}
		m.statusCallback(jobID, status, errMsg, active, queued)
	}
}

func (m *Manager) runJob(ctx context.Context, job *Job) {
	err := m.executeRebasePhase(job.JobCtx, job)

	m.mu.Lock()
	defer m.mu.Unlock()

	if err != nil {
		job.State = stateFailed
		job.Error = err
		m.log(fmt.Sprintf("\u001b[31m[%s] FAILED rebase workflow: %v\u001b[0m\r\n", job.RepoName, err))
		m.reportStatusLocked(job.ID, "failed", err.Error())
		m.handleGroupFailureLocked(job)
		m.cleanCompletedJobsLocked()
		return
	}

	job.State = stateRebased
	m.log(fmt.Sprintf("\u001b[32m[%s] Rebase phase finished successfully.\u001b[0m\r\n", job.RepoName))

	// Check if part of a group
	group := m.getGroupJobsLocked(job)
	if len(group) <= 1 {
		// Single job (no group or group of size 1). Push immediately.
		m.mu.Unlock()
		pushErr := m.executePushPhase(job.JobCtx, job)
		m.mu.Lock()

		if pushErr != nil {
			job.State = stateFailed
			job.Error = pushErr
			m.log(fmt.Sprintf("\u001b[31m[%s] FAILED force push: %v\u001b[0m\r\n", job.RepoName, pushErr))
			m.reportStatusLocked(job.ID, "failed", pushErr.Error())
		} else {
			job.State = stateSuccess
			m.log(fmt.Sprintf("\u001b[32m[%s] SUCCESS: PR rebase & push workflow finished!\u001b[0m\r\n", job.RepoName))
			m.reportStatusLocked(job.ID, "success", "")
		}
		m.cleanCompletedJobsLocked()
		return
	}

	// Group job. Check if all are rebased
	allRebased := true
	for _, gj := range group {
		if gj.State != stateRebased {
			allRebased = false
			break
		}
	}

	if allRebased {
		m.log(fmt.Sprintf("\u001b[32m[GROUP] All rebases for branch '%s' succeeded. Starting group push...\u001b[0m\r\n", job.GetBranchName()))
		m.executeGroupPushLocked(group)
	} else {
		m.log(fmt.Sprintf("\u001b[33m[GROUP] Branch '%s' waiting for other group PRs to finish rebase...\u001b[0m\r\n", job.GetBranchName()))
	}
}

func (m *Manager) executeRebasePhase(ctx context.Context, job *Job) error {
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

	headBranch := ""
	// 3. Detect target Remote
	parts := strings.Split(job.HeadLabel, "/")
	if len(parts) != 2 {
		return fmt.Errorf(
			"head branch '%s' has no remote tracking configured — please select a remote for it in the UI before rebasing",
			job.HeadLabel,
		)
	} else {
		headBranch = parts[1]
	}
	parts = strings.Split(job.BaseLabel, "/")
	baseBranch, baseBranchRemote := "", ""
	if len(parts) != 2 {
		return fmt.Errorf("invalid base label format: %s", job.BaseLabel)
	}
	baseBranchRemote = parts[0]
	baseBranch = parts[1]

	// 4. Checkout branch
	logger(fmt.Sprintf("Checking out head branch '%s'...", headBranch))
	if err := m.gitExecutor.Checkout(ctx, job.RepoPath, headBranch, logger); err != nil {
		return fmt.Errorf("failed checking out head branch: %w", err)
	}

	// 5. Rebase onto base branch
	logger(fmt.Sprintf("Rebasing '%s' onto '%s/%s'...", job.HeadLabel, baseBranchRemote, baseBranch))
	if err := m.gitExecutor.Rebase(ctx, job.RepoPath, baseBranch, baseBranchRemote, logger); err != nil {
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

	return nil
}

func (m *Manager) executePushPhase(ctx context.Context, job *Job) error {
	if !job.Options.ForcePushAfterRebase {
		return nil
	}

	logger := func(msg string) {
		m.log(fmt.Sprintf("[%s] %s", job.RepoName, msg))
	}

	parts := strings.Split(job.HeadLabel, "/")
	if len(parts) != 2 {
		return fmt.Errorf("invalid head label: %s", job.HeadLabel)
	}
	headBranchRemote := parts[0]
	headBranch := parts[1]

	logger(fmt.Sprintf("Force pushing branch '%s' to remote '%s' using safe push lease...", headBranch, headBranchRemote))
	if err := m.gitExecutor.ForcePush(ctx, job.RepoPath, headBranchRemote, headBranch, logger); err != nil {
		return fmt.Errorf("force push failed: %w", err)
	}

	return nil
}

func (m *Manager) executeGroupPushLocked(group []*Job) {
	for _, j := range group {
		j.State = statePushing
		m.reportStatusLocked(j.ID, "running", "")
	}

	groupCopy := make([]*Job, len(group))
	copy(groupCopy, group)

	go func() {
		var wg sync.WaitGroup
		pushErrors := make([]error, len(groupCopy))

		for i, j := range groupCopy {
			wg.Add(1)
			go func(idx int, job *Job) {
				defer wg.Done()
				pushErrors[idx] = m.executePushPhase(job.JobCtx, job)
			}(i, j)
		}

		wg.Wait()

		m.mu.Lock()
		defer m.mu.Unlock()

		for i, job := range groupCopy {
			err := pushErrors[i]
			if err != nil {
				job.State = stateFailed
				job.Error = err
				m.log(fmt.Sprintf("\u001b[31m[%s] GROUP FAILED force push: %v\u001b[0m\r\n", job.RepoName, err))
				m.reportStatusLocked(job.ID, "failed", err.Error())
			} else {
				job.State = stateSuccess
				m.log(fmt.Sprintf("\u001b[32m[%s] GROUP SUCCESS: PR rebase & push workflow finished!\u001b[0m\r\n", job.RepoName))
				m.reportStatusLocked(job.ID, "success", "")
			}
		}

		m.cleanCompletedJobsLocked()
		m.broadcast()
	}()
}

func (m *Manager) handleGroupFailureLocked(failedJob *Job) {
	branch := failedJob.GetBranchName()
	group := m.getGroupJobsLocked(failedJob)

	if len(group) > 0 {
		m.log(fmt.Sprintf("\u001b[31m[GROUP] Aborting group push for branch '%s' because '%s' failed.\u001b[0m\r\n", branch, failedJob.RepoName))

		for _, j := range group {
			if j.ID == failedJob.ID {
				continue
			}

			if j.State == stateRebasing && j.Cancel != nil {
				j.Cancel()
			}

			j.State = stateFailed
			j.Error = fmt.Errorf("group push aborted due to failure in %s", failedJob.RepoName)
			m.log(fmt.Sprintf("\u001b[31m[%s] Aborted: companion branch in group failed.\u001b[0m\r\n", j.RepoName))
			m.reportStatusLocked(j.ID, "failed", j.Error.Error())
		}
	}
}

func (m *Manager) cleanCompletedJobsLocked() {
	var active []*Job
	for _, j := range m.jobs {
		if j.State != stateSuccess && j.State != stateFailed {
			active = append(active, j)
		}
	}
	m.jobs = active
}

func (m *Manager) getGroupJobsLocked(job *Job) []*Job {
	branch := job.GetBranchName()
	var group []*Job
	for _, j := range m.jobs {
		if j.State != stateSuccess && j.State != stateFailed {
			if j.GetBranchName() == branch {
				group = append(group, j)
			}
		}
	}
	return group
}

func (m *Manager) ProcessRemoteUpdate(ctx context.Context, localPath string) error {
	logger := func(msg string) {
		m.log(msg)
	}
	if err := m.gitExecutor.Fetch(ctx, localPath, logger); err != nil {
		return fmt.Errorf("failed fetching remotes: %w", err)
	}
	return nil
}
