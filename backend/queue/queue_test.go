package queue

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"my-github-pr/backend/git"
	"my-github-pr/backend/models"
)

type MockGitExecutor struct {
	mu           sync.Mutex
	locks        map[string]*sync.Mutex
	isCleanFunc  func(ctx context.Context, dir string) (bool, error)
	fetchFunc    func(ctx context.Context, dir string, log git.LogWriter) error
	checkoutFunc func(ctx context.Context, dir string, branch string, log git.LogWriter) error
	rebaseFunc   func(ctx context.Context, dir string, baseBranch string, remote string, log git.LogWriter) error
	abortFunc    func(ctx context.Context, dir string, log git.LogWriter) error
	amendFunc    func(ctx context.Context, dir string, log git.LogWriter) error
	pushFunc     func(ctx context.Context, dir string, remote string, branch string, log git.LogWriter) error

	// Track function execution history for assertions
	history []string
}

func (m *MockGitExecutor) GetRepoLock(repoPath string) *sync.Mutex {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.locks == nil {
		m.locks = make(map[string]*sync.Mutex)
	}
	lock, exists := m.locks[repoPath]
	if !exists {
		lock = &sync.Mutex{}
		m.locks[repoPath] = lock
	}
	return lock
}

func (m *MockGitExecutor) recordCall(msg string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.history = append(m.history, msg)
}

func (m *MockGitExecutor) IsClean(ctx context.Context, dir string) (bool, error) {
	m.recordCall(fmt.Sprintf("IsClean:%s", dir))
	if m.isCleanFunc != nil {
		return m.isCleanFunc(ctx, dir)
	}
	return true, nil
}

func (m *MockGitExecutor) Fetch(ctx context.Context, dir string, log git.LogWriter) error {
	m.recordCall(fmt.Sprintf("Fetch:%s", dir))
	if m.fetchFunc != nil {
		return m.fetchFunc(ctx, dir, log)
	}
	return nil
}

func (m *MockGitExecutor) Checkout(ctx context.Context, dir string, branch string, log git.LogWriter) error {
	m.recordCall(fmt.Sprintf("Checkout:%s:%s", dir, branch))
	if m.checkoutFunc != nil {
		return m.checkoutFunc(ctx, dir, branch, log)
	}
	return nil
}

func (m *MockGitExecutor) Rebase(ctx context.Context, dir string, baseBranch string, remote string, log git.LogWriter) error {
	m.recordCall(fmt.Sprintf("Rebase:%s:%s/%s", dir, remote, baseBranch))
	if m.rebaseFunc != nil {
		return m.rebaseFunc(ctx, dir, baseBranch, remote, log)
	}
	return nil
}

func (m *MockGitExecutor) RebaseAbort(ctx context.Context, dir string, log git.LogWriter) error {
	m.recordCall(fmt.Sprintf("RebaseAbort:%s", dir))
	if m.abortFunc != nil {
		return m.abortFunc(ctx, dir, log)
	}
	return nil
}

func (m *MockGitExecutor) AmendTimestamp(ctx context.Context, dir string, log git.LogWriter) error {
	m.recordCall(fmt.Sprintf("AmendTimestamp:%s", dir))
	if m.amendFunc != nil {
		return m.amendFunc(ctx, dir, log)
	}
	return nil
}

func (m *MockGitExecutor) ForcePush(ctx context.Context, dir string, remote string, branch string, log git.LogWriter) error {
	m.recordCall(fmt.Sprintf("ForcePush:%s:%s/%s", dir, remote, branch))
	if m.pushFunc != nil {
		return m.pushFunc(ctx, dir, remote, branch, log)
	}
	return nil
}

// Helper to wait for a slice to contain a specific number of completed jobs
func waitForJobsCount(m *Manager, count int, timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		m.mu.Lock()
		n := len(m.jobs)
		m.mu.Unlock()
		if n == count {
			return true
		}
		time.Sleep(10 * time.Millisecond)
	}
	return false
}

func TestParallelRebasesDifferentRepos(t *testing.T) {
	mockExecutor := &MockGitExecutor{}
	var activeRebases sync.Map
	var maxConcurrent int32
	var concurrentMu sync.Mutex
	var activeCount int32

	// Setup mock to sleep during rebase to trace concurrent execution
	mockExecutor.rebaseFunc = func(ctx context.Context, dir string, baseBranch string, remote string, log git.LogWriter) error {
		activeRebases.Store(dir, true)
		concurrentMu.Lock()
		activeCount++
		if activeCount > maxConcurrent {
			maxConcurrent = activeCount
		}
		concurrentMu.Unlock()

		time.Sleep(100 * time.Millisecond)

		concurrentMu.Lock()
		activeCount--
		concurrentMu.Unlock()
		activeRebases.Delete(dir)
		return nil
	}

	mgr := NewManager(3, mockExecutor, nil, nil)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	mgr.Start(ctx)

	job1 := Job{
		ID:        "job1",
		RepoName:  "owner/repo1",
		RepoPath:  "/path/repo1",
		HeadLabel: "origin/feat1",
		BaseLabel: "origin/main",
		Options:   models.Settings{ForcePushAfterRebase: true},
	}
	job2 := Job{
		ID:        "job2",
		RepoName:  "owner/repo2",
		RepoPath:  "/path/repo2",
		HeadLabel: "origin/feat2",
		BaseLabel: "origin/main",
		Options:   models.Settings{ForcePushAfterRebase: true},
	}

	mgr.Submit(job1)
	mgr.Submit(job2)

	// Wait for jobs to finish processing (they get removed from mgr.jobs once completed)
	waitForJobsCount(mgr, 0, 500*time.Millisecond)
	mgr.Stop()

	// Verify that at some point both were running concurrently (maxConcurrent should be 2)
	concurrentMu.Lock()
	max := maxConcurrent
	concurrentMu.Unlock()

	if max < 2 {
		t.Errorf("expected parallel execution (maxConcurrent = %d, wanted >= 2)", max)
	}
}

func TestSequentialRebasesSameRepo(t *testing.T) {
	mockExecutor := &MockGitExecutor{}

	var (
		job1Start time.Time
		job1End   time.Time
		job2Start time.Time
		mu        sync.Mutex
	)

	mockExecutor.rebaseFunc = func(
		ctx context.Context,
		dir string,
		baseBranch string,
		remote string,
		log git.LogWriter,
	) error {
		mu.Lock()

		firstJob := job1Start.IsZero()
		if firstJob {
			job1Start = time.Now()
		} else {
			job2Start = time.Now()
		}

		mu.Unlock()

		time.Sleep(50 * time.Millisecond)

		if firstJob {
			mu.Lock()
			job1End = time.Now()
			mu.Unlock()
		}

		return nil
	}

	mgr := NewManager(3, mockExecutor, nil, nil)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	mgr.Start(ctx)

	mgr.Submit(Job{
		ID:        "job1",
		RepoName:  "owner/repo1",
		RepoPath:  "/path/repo1",
		HeadLabel: "origin/feat1",
		BaseLabel: "origin/main",
		Options:   models.Settings{ForcePushAfterRebase: true},
	})

	mgr.Submit(Job{
		ID:        "job2",
		RepoName:  "owner/repo1",
		RepoPath:  "/path/repo1",
		HeadLabel: "origin/feat2",
		BaseLabel: "origin/main",
		Options:   models.Settings{ForcePushAfterRebase: true},
	})

	waitForJobsCount(mgr, 0, 500*time.Millisecond)
	mgr.Stop()

	mu.Lock()
	defer mu.Unlock()

	if job1End.IsZero() || job2Start.IsZero() {
		t.Fatal("expected both jobs to execute")
	}

	if job2Start.Before(job1End) {
		t.Fatalf(
			"expected sequential execution: job2 started at %v before job1 finished at %v",
			job2Start,
			job1End,
		)
	}
}

func TestGroupPushSuccess(t *testing.T) {
	mockExecutor := &MockGitExecutor{}
	var pushTimes sync.Map
	var rebaseEndTimes sync.Map
	var mu sync.Mutex

	mockExecutor.rebaseFunc = func(ctx context.Context, dir string, baseBranch string, remote string, log git.LogWriter) error {
		time.Sleep(50 * time.Millisecond)
		mu.Lock()
		rebaseEndTimes.Store(dir, time.Now())
		mu.Unlock()
		return nil
	}

	mockExecutor.pushFunc = func(ctx context.Context, dir string, remote string, branch string, log git.LogWriter) error {
		mu.Lock()
		pushTimes.Store(dir, time.Now())
		mu.Unlock()
		return nil
	}

	mgr := NewManager(3, mockExecutor, nil, nil)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	mgr.Start(ctx)

	// Two PRs on the same branch name "feat-shared" in different repos
	job1 := Job{
		ID:        "job1",
		RepoName:  "owner/repoA",
		RepoPath:  "/path/repoA",
		HeadLabel: "origin/feat-shared",
		BaseLabel: "origin/main",
		Options:   models.Settings{ForcePushAfterRebase: true},
	}
	job2 := Job{
		ID:        "job2",
		RepoName:  "owner/repoB",
		RepoPath:  "/path/repoB",
		HeadLabel: "origin/feat-shared",
		BaseLabel: "origin/main",
		Options:   models.Settings{ForcePushAfterRebase: true},
	}

	mgr.Submit(job1)
	mgr.Submit(job2)

	waitForJobsCount(mgr, 0, 500*time.Millisecond)
	mgr.Stop()

	// Retrieve times
	var rATime, rBTime, pATime, pBTime time.Time
	mu.Lock()
	if val, ok := rebaseEndTimes.Load("/path/repoA"); ok {
		rATime = val.(time.Time)
	}
	if val, ok := rebaseEndTimes.Load("/path/repoB"); ok {
		rBTime = val.(time.Time)
	}
	if val, ok := pushTimes.Load("/path/repoA"); ok {
		pATime = val.(time.Time)
	}
	if val, ok := pushTimes.Load("/path/repoB"); ok {
		pBTime = val.(time.Time)
	}
	mu.Unlock()

	if rATime.IsZero() || rBTime.IsZero() || pATime.IsZero() || pBTime.IsZero() {
		t.Fatalf("expected all rebases and pushes to have execution times recorded")
	}

	// Verify that pushes occurred after BOTH rebases finished
	lastRebase := rATime
	if rBTime.After(lastRebase) {
		lastRebase = rBTime
	}

	if pATime.Before(lastRebase) || pBTime.Before(lastRebase) {
		t.Errorf("expected pushes to occur only after all rebases finished. Last rebase: %v, Push A: %v, Push B: %v", lastRebase, pATime, pBTime)
	}

	// Verify pushes were done close together (within 1 second in mock, constraint is 1min)
	pushDiff := pATime.Sub(pBTime)
	if pushDiff < 0 {
		pushDiff = -pushDiff
	}
	if pushDiff > 1*time.Second {
		t.Errorf("expected pushes to happen together, but diff was %v", pushDiff)
	}
}

func TestGroupPushAbortedOnRebaseFailure(t *testing.T) {
	mockExecutor := &MockGitExecutor{}
	var pushCalled bool
	var mu sync.Mutex

	mockExecutor.rebaseFunc = func(ctx context.Context, dir string, baseBranch string, remote string, log git.LogWriter) error {
		if dir == "/path/repoB" {
			// repoB rebase fails
			return errors.New("rebase conflict")
		}
		return nil
	}

	mockExecutor.pushFunc = func(ctx context.Context, dir string, remote string, branch string, log git.LogWriter) error {
		mu.Lock()
		pushCalled = true
		mu.Unlock()
		return nil
	}

	mgr := NewManager(3, mockExecutor, nil, nil)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	mgr.Start(ctx)

	// Two PRs on the same branch "feat-shared" in different repos; repoB fails
	job1 := Job{
		ID:        "job1",
		RepoName:  "owner/repoA",
		RepoPath:  "/path/repoA",
		HeadLabel: "origin/feat-shared",
		BaseLabel: "origin/main",
		Options:   models.Settings{ForcePushAfterRebase: true},
	}
	job2 := Job{
		ID:        "job2",
		RepoName:  "owner/repoB",
		RepoPath:  "/path/repoB",
		HeadLabel: "origin/feat-shared",
		BaseLabel: "origin/main",
		Options:   models.Settings{ForcePushAfterRebase: true},
	}

	mgr.Submit(job1)
	mgr.Submit(job2)

	waitForJobsCount(mgr, 0, 500*time.Millisecond)
	mgr.Stop()

	mu.Lock()
	pushWasCalled := pushCalled
	mu.Unlock()

	if pushWasCalled {
		t.Errorf("expected force push to be aborted for both repos because repoB rebase failed")
	}
}
