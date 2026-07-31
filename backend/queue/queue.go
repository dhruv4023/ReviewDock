package queue

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"review-dock/backend/git"
	"review-dock/backend/models"
)

type LogFunc func(message string)
type StatusFunc func(jobID string, status string, errMsg string, active int, queued int)

type GitExecutor interface {
	GetRepoLock(repoPath string) *sync.Mutex
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
	State    JobState
	Error    error
	Cancel   context.CancelFunc
	JobCtx   context.Context
	SkipPush bool // if true, complete the rebase but do NOT force-push at the end
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
	rebaseSem      chan struct{}
	cond           *sync.Cond
	logCallback    LogFunc
	statusCallback StatusFunc
	ctx            context.Context
	cancel         context.CancelFunc

	// branchGroups records the complete set of jobs submitted for each branch
	// name. Unlike m.jobs (which shrinks as jobs complete), this map preserves
	// every member so that group-size checks are never fooled by a sibling that
	// was cleaned from m.jobs before the check runs.
	branchGroups map[string][]*Job
}

func NewManager(workers int, gitExecutor GitExecutor, logCallback LogFunc, statusCallback StatusFunc) *Manager {
	if workers <= 0 {
		workers = 3
	}
	m := &Manager{
		gitExecutor:    gitExecutor,
		workers:        workers,
		rebaseSem:      make(chan struct{}, workers),
		logCallback:    logCallback,
		statusCallback: statusCallback,
		branchGroups:   make(map[string][]*Job),
	}
	m.cond = sync.NewCond(&m.mu)
	return m
}

func (m *Manager) Start(ctx context.Context) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.ctx, m.cancel = context.WithCancel(ctx)
}

func (m *Manager) Submit(job Job) {
	m.mu.Lock()

	for _, existing := range m.jobs {
		if existing.ID == job.ID {
			m.mu.Unlock()
			return
		}
	}

	jobCtx, cancel := context.WithCancel(m.ctx)
	j := &Job{
		ID:        job.ID,
		RepoName:  job.RepoName,
		RepoPath:  job.RepoPath,
		HeadLabel: job.HeadLabel,
		BaseLabel: job.BaseLabel,
		Options:   job.Options,
		State:     statePending,
		JobCtx:    jobCtx,
		Cancel:    cancel,
	}
	m.jobs = append(m.jobs, j)

	// Register in the branch group registry.
	branch := j.GetBranchName()
	m.branchGroups[branch] = append(m.branchGroups[branch], j)

	m.log(fmt.Sprintf("\u001b[33m[%s] Queued PR branch '%s' for rebasing onto '%s'\u001b[0m\r\n", j.RepoName, j.HeadLabel, j.BaseLabel))
	m.reportStatusLocked(j.ID, "queued", "")

	m.wg.Add(1)
	go m.processJob(j)

	m.mu.Unlock()
}

func (m *Manager) Cancel(jobID string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	for _, j := range m.jobs {
		if j.ID != jobID {
			continue
		}

		switch j.State {
		case statePending, stateRebasing, stateRebased:
			// Do NOT cancel the context or fail the job.
			// A running rebase must be allowed to finish cleanly — interrupting it
			// mid-way leaves the git working tree dirty. Setting SkipPush=true
			// tells processJob to complete the rebase and then silently skip the
			// force-push at the end.
			j.SkipPush = true
			m.log(fmt.Sprintf("\u001b[33m[%s] Push aborted by user — rebase will complete but the branch will NOT be force-pushed.\u001b[0m\r\n", j.RepoName))
			m.reportStatusLocked(j.ID, "running", "")

		case statePushing:
			// Already in the push phase — safe to cancel the context here because
			// no git rebase is in progress; only the network push call can be interrupted.
			if j.Cancel != nil {
				j.Cancel()
			}
			j.State = stateFailed
			j.Error = fmt.Errorf("force-push cancelled by user")
			m.log(fmt.Sprintf("\u001b[31m[%s] Force-push cancelled by user.\u001b[0m\r\n", j.RepoName))
			m.reportStatusLocked(j.ID, "failed", j.Error.Error())
			m.handleGroupFailureLocked(j)
			m.cleanCompletedJobsLocked()
			m.cleanBranchGroupLocked(j.GetBranchName())
		}
		break
	}
	m.cond.Broadcast()
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
	m.cond.Broadcast()
	m.mu.Unlock()
	m.wg.Wait()
}

func (m *Manager) log(msg string) {
	if m.logCallback != nil {
		m.logCallback(msg)
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

func (m *Manager) processJob(job *Job) {
	defer m.wg.Done()

	lock := m.gitExecutor.GetRepoLock(job.RepoPath)

	// ── PHASE 1: REBASE ──────────────────────────────────────────────────────
	// Hold the repo lock ONLY for the rebase phase, not the entire function.
	// Releasing it before m.cond.Wait() is critical: if multiple PRs from the
	// same repo share the same branch, the first job would hold the lock while
	// waiting on the cond, blocking siblings from ever starting → deadlock.

	lock.Lock()

	// A pending job may have had SkipPush set while it was waiting for the lock.
	// If so, bail out now before touching git — the tree is still clean.
	m.mu.Lock()
	if job.SkipPush && job.State == statePending {
		m.log(fmt.Sprintf("\u001b[33m[%s] Push-skip requested while queued — not starting rebase.\u001b[0m\r\n", job.RepoName))
		job.State = stateSuccess
		m.reportStatusLocked(job.ID, "success", "")
		m.cleanCompletedJobsLocked()
		m.cleanBranchGroupLocked(job.GetBranchName())
		m.cond.Broadcast()
		m.mu.Unlock()
		lock.Unlock()
		return
	}
	m.mu.Unlock()

	// Wait to acquire rebase semaphore (or bail on cancellation)
	select {
	case <-job.JobCtx.Done():
		lock.Unlock()
		m.mu.Lock()
		job.State = stateFailed
		job.Error = job.JobCtx.Err()
		m.reportStatusLocked(job.ID, "failed", job.Error.Error())
		m.handleGroupFailureLocked(job)
		m.cleanCompletedJobsLocked()
		m.cleanBranchGroupLocked(job.GetBranchName())
		m.cond.Broadcast()
		m.mu.Unlock()
		return
	case m.rebaseSem <- struct{}{}:
	}

	m.mu.Lock()
	job.State = stateRebasing
	m.reportStatusLocked(job.ID, "running", "")
	m.mu.Unlock()

	m.log(fmt.Sprintf("\u001b[32m[%s] Starting rebase process for branch '%s'...\u001b[0m\r\n", job.RepoName, job.HeadLabel))
	err := m.executeRebasePhase(job.JobCtx, job)

	// Release rebase semaphore immediately to allow other rebases to start.
	<-m.rebaseSem
	// Release repo lock BEFORE group coordination so that same-repo siblings
	// can acquire it and run their own rebase without deadlocking.
	lock.Unlock()

	m.mu.Lock()
	if err != nil || job.State == stateFailed {
		if err != nil {
			job.State = stateFailed
			job.Error = err
			m.log(fmt.Sprintf("\u001b[31m[%s] FAILED rebase workflow: %v\u001b[0m\r\n", job.RepoName, err))
			m.reportStatusLocked(job.ID, "failed", err.Error())
			m.handleGroupFailureLocked(job)
		} else {
			m.log(fmt.Sprintf("\u001b[31m[%s] Rebase succeeded locally, but push skipped because companion branch in group failed.\u001b[0m\r\n", job.RepoName))
		}
		m.cleanCompletedJobsLocked()
		m.cleanBranchGroupLocked(job.GetBranchName())
		m.cond.Broadcast()
		m.mu.Unlock()
		return
	}

	job.State = stateRebased
	m.log(fmt.Sprintf("\u001b[32m[%s] Rebase phase finished successfully.\u001b[0m\r\n", job.RepoName))

	// ── PHASE 2: GROUP COORDINATION ──────────────────────────────────────────
	// Use the persistent branchGroups registry (not m.jobs) so that we always
	// see the full group even if some siblings have already been cleaned up.

	group := m.getGroupJobsLocked(job)
	if len(group) <= 1 {
		// Solo job — push immediately.
		m.mu.Unlock()
		lock.Lock()
		pushErr := m.executePushPhase(job.JobCtx, job)
		lock.Unlock()
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
		m.cleanBranchGroupLocked(job.GetBranchName())
		m.mu.Unlock()
		return
	}

	// Group job: all-or-nothing push.
	// Check if this is the last member to finish rebasing.
	allRebased := true
	for _, gj := range group {
		if gj.State != stateRebased {
			allRebased = false
			break
		}
	}

	if allRebased {
		m.log(fmt.Sprintf("\u001b[32m[GROUP] All %d rebases for branch '%s' succeeded. Starting group push...\u001b[0m\r\n", len(group), job.GetBranchName()))
		m.executeGroupPushLocked(group)
	} else {
		m.log(fmt.Sprintf("\u001b[33m[GROUP] Branch '%s' waiting for other group PRs to finish rebase...\u001b[0m\r\n", job.GetBranchName()))
	}

	// Wait until all group members have rebased (or one has failed).
	// handleGroupFailureLocked sets our state to stateFailed and broadcasts,
	// so this loop always terminates.
	for job.State == stateRebased {
		m.cond.Wait()
	}

	// ── PHASE 3: PUSH ────────────────────────────────────────────────────────
	// Re-acquire repo lock for the push phase (serialises git operations per repo).
	// If state is stateFailed (group aborted) the push is skipped entirely.
	if job.State == statePushing {
		m.mu.Unlock()
		lock.Lock()
		pushErr := m.executePushPhase(job.JobCtx, job)
		lock.Unlock()
		m.mu.Lock()

		if pushErr != nil {
			job.State = stateFailed
			job.Error = pushErr
			m.log(fmt.Sprintf("\u001b[31m[%s] GROUP FAILED force push: %v\u001b[0m\r\n", job.RepoName, pushErr))
			m.reportStatusLocked(job.ID, "failed", pushErr.Error())
		} else {
			job.State = stateSuccess
			m.log(fmt.Sprintf("\u001b[32m[%s] GROUP SUCCESS: PR rebase & push workflow finished!\u001b[0m\r\n", job.RepoName))
			m.reportStatusLocked(job.ID, "success", "")
		}
	}
	// Always clean up — covers both the push path and the group-abort path
	// where this job was failed by handleGroupFailureLocked without pushing.
	m.cleanCompletedJobsLocked()
	m.cleanBranchGroupLocked(job.GetBranchName())
	m.mu.Unlock()
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
	if job.SkipPush {
		m.log(fmt.Sprintf("\u001b[33m[%s] Rebase complete — force-push skipped (aborted by user).\u001b[0m\r\n", job.RepoName))
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
	m.cond.Broadcast()
}

// handleGroupFailureLocked marks all non-failed group siblings as failed and
// cancels any that are still pending or rebasing so they bail out immediately.
// This enforces all-or-nothing: if any member fails to rebase, the whole group
// is aborted and no one pushes.
func (m *Manager) handleGroupFailureLocked(failedJob *Job) {
	branch := failedJob.GetBranchName()
	group := m.getGroupJobsLocked(failedJob)

	if len(group) > 0 {
		m.log(fmt.Sprintf("\u001b[31m[GROUP] Aborting group for branch '%s' — '%s' failed. All-or-nothing: no PR in this group will be pushed.\u001b[0m\r\n", branch, failedJob.RepoName))

		for _, j := range group {
			if j.ID == failedJob.ID {
				continue
			}

			// Cancel context for pending AND rebasing jobs so they bail out as
			// soon as they next check ctx.Done(). A statePending job blocked on
			// the repo lock will detect the cancellation once it acquires the
			// lock and will skip its rebase.
			if (j.State == statePending || j.State == stateRebasing) && j.Cancel != nil {
				j.Cancel()
			}

			j.State = stateFailed
			j.Error = fmt.Errorf("group aborted: rebase failed in %s — push skipped for all group members", failedJob.RepoName)
			m.log(fmt.Sprintf("\u001b[31m[%s] Skipped push: another PR in group '%s' failed to rebase.\u001b[0m\r\n", j.RepoName, branch))
			m.reportStatusLocked(j.ID, "failed", j.Error.Error())
		}
	}
	m.cond.Broadcast()
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

// cleanBranchGroupLocked removes the branch group entry once all members have
// reached a terminal state. This prevents the branchGroups map from leaking
// entries for long-running deployments.
func (m *Manager) cleanBranchGroupLocked(branch string) {
	group, ok := m.branchGroups[branch]
	if !ok {
		return
	}
	for _, j := range group {
		if j.State != stateSuccess && j.State != stateFailed {
			return // at least one member still running
		}
	}
	delete(m.branchGroups, branch)
}

// getGroupJobsLocked returns ALL jobs registered for the same branch name via
// the persistent branchGroups registry. Unlike scanning m.jobs, this includes
// members that have already completed (success/failed) and been cleaned up,
// so the group-size check is never fooled by a fast-failing sibling.
func (m *Manager) getGroupJobsLocked(job *Job) []*Job {
	branch := job.GetBranchName()
	return m.branchGroups[branch]
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
