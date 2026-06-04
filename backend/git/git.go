package git

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"my-github-pr/logger"
	"os/exec"
	"strings"
	"sync"
)

type LogWriter func(message string)

type Executor struct {
	locks   map[string]*sync.Mutex
	locksMu sync.Mutex
}

func NewExecutor() *Executor {
	return &Executor{
		locks: make(map[string]*sync.Mutex),
	}
}

func (e *Executor) GetRepoLock(repoPath string) *sync.Mutex {
	e.locksMu.Lock()
	defer e.locksMu.Unlock()

	lock, exists := e.locks[repoPath]
	if !exists {
		lock = &sync.Mutex{}
		e.locks[repoPath] = lock
	}
	return lock
}

func (e *Executor) CheckGitVersion(ctx context.Context) (string, error) {
	cmd := exec.CommandContext(ctx, "git", "--version")
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("git is not installed or not in PATH: %w", err)
	}
	return strings.TrimSpace(string(out)), nil
}

func (e *Executor) runCommand(ctx context.Context, dir string, log LogWriter, args ...string) error {
	if log != nil {
		log(fmt.Sprintf("\u001b[34m[GIT] Running: git %s\u001b[0m\r\n", strings.Join(args, " ")))
	}

	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = dir

	stdoutPipe, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	stderrPipe, err := cmd.StderrPipe()
	if err != nil {
		return err
	}

	if err := cmd.Start(); err != nil {
		return err
	}

	// Read stdout and stderr concurrently.
	// IMPORTANT: close(outputChan) must only happen after BOTH reader goroutines
	// have finished sending — cmd.Wait() returning does NOT guarantee that, since
	// the scanners may still be in the middle of a send when the process exits.
	// Use a WaitGroup so the closer goroutine waits for both readers.
	outputChan := make(chan string)
	var readersWg sync.WaitGroup

	readPipe := func(reader io.Reader) {
		defer readersWg.Done()
		scanner := bufio.NewScanner(reader)
		for scanner.Scan() {
			outputChan <- scanner.Text()
		}
	}

	readersWg.Add(2)
	go readPipe(stdoutPipe)
	go readPipe(stderrPipe)

	// Close the channel only after both readers have exited.
	go func() {
		_ = cmd.Wait()
		readersWg.Wait()
		close(outputChan)
	}()

	for line := range outputChan {
		if log != nil {
			log(line + "\r\n")
		}
	}

	if cmd.ProcessState != nil && !cmd.ProcessState.Success() {
		return fmt.Errorf("git command failed with exit code %d", cmd.ProcessState.ExitCode())
	}

	return nil
}

func (e *Executor) IsClean(ctx context.Context, dir string) (bool, error) {
	cmd := exec.CommandContext(ctx, "git", "status", "--porcelain")
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		return false, err
	}
	return len(strings.TrimSpace(string(out))) == 0, nil
}

func (e *Executor) Fetch(ctx context.Context, dir string, log LogWriter) error {
	return e.runCommand(ctx, dir, log, "fetch", "--all", "--prune")
}

func (e *Executor) RemoteUpdate(ctx context.Context, dir string, log LogWriter) error {
	return e.runCommand(ctx, dir, log, "remote", "update")
}

func (e *Executor) Checkout(ctx context.Context, dir string, branch string, log LogWriter) error {
	return e.runCommand(ctx, dir, log, "checkout", branch)
}

func (e *Executor) CheckoutRemoteBranch(ctx context.Context, dir string, branch string, remote string, log LogWriter) error {
	return e.runCommand(ctx, dir, log, "checkout", "-B", branch, fmt.Sprintf("%s/%s", remote, branch))
}

func (e *Executor) Rebase(ctx context.Context, dir string, baseBranch string, remote string, log LogWriter) error {
	return e.runCommand(ctx, dir, log, "rebase", fmt.Sprintf("%s/%s", remote, baseBranch))
}

func (e *Executor) RebaseAbort(ctx context.Context, dir string, log LogWriter) error {
	return e.runCommand(ctx, dir, log, "rebase", "--abort")
}

func (e *Executor) AmendTimestamp(ctx context.Context, dir string, log LogWriter) error {
	return e.runCommand(ctx, dir, log, "commit", "--amend", "--no-edit", "--date=now")
}

func (e *Executor) ForcePush(ctx context.Context, dir string, remote string, branch string, log LogWriter) error {
	return e.runCommand(ctx, dir, log, "push", "--force-with-lease", remote, branch)
}

func (e *Executor) DetectBestRemote(ctx context.Context, dir string, priorities []string) (string, error) {
	cmd := exec.CommandContext(ctx, "git", "remote")
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}

	remotes := strings.Split(strings.TrimSpace(string(out)), "\n")
	remoteMap := make(map[string]bool)
	for _, r := range remotes {
		remoteMap[strings.TrimSpace(r)] = true
	}

	// Try priorities in order
	for _, p := range priorities {
		if remoteMap[p] {
			return p, nil
		}
	}

	// Fallback to first remote found
	if len(remotes) > 0 && remotes[0] != "" {
		return remotes[0], nil
	}

	return "origin", nil
}

// ListRemotes returns the names of all git remotes configured for the given repo directory.
func (e *Executor) ListRemotes(ctx context.Context, dir string) ([]string, error) {
	cmd := exec.CommandContext(ctx, "git", "remote")
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("failed listing remotes: %w", err)
	}
	var remotes []string
	for _, r := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		r = strings.TrimSpace(r)
		if r != "" {
			remotes = append(remotes, r)
		}
	}
	return remotes, nil
}

// SetBranchTracking configures the upstream tracking reference for a local branch
// by running: git branch --set-upstream-to=<remote>/<branch> <branch>
// After this call, git push/pull will default to the specified remote.
func (e *Executor) SetBranchTracking(ctx context.Context, dir string, branch string, remote string, log LogWriter) error {
	return e.runCommand(ctx, dir, log, "branch", fmt.Sprintf("--set-upstream-to=%s/%s", remote, branch), branch)
}

// Diff returns the git diff output comparing the baseLabel to the headBranch.
func (e *Executor) Diff(ctx context.Context, dir string, baseLabel string, headBranch string) (string, error) {
	// Try three-dot diff (changes in head branch since it diverged from base)
	cmd := exec.CommandContext(ctx, "git", "-C", dir, "diff", baseLabel+"..."+headBranch)
	out, err := cmd.Output()
	if err != nil {
		// Fallback to two-dot diff
		cmd2 := exec.CommandContext(ctx, "git", "-C", dir, "diff", baseLabel+".."+headBranch)
		out2, err2 := cmd2.Output()
		if err2 != nil {
			return "", fmt.Errorf("failed to run git diff: %w", err)
		}
		return string(out2), nil
	}
	return string(out), nil
}

// LocalAheadBehind returns how many commits the local branch is ahead and behind
// its remote tracking branch (remote/branch). Returns 0, 0, nil if the branch
// does not exist locally or has no remote tracking ref — not treated as an error.
func LocalAheadBehind(ctx context.Context, dir, baseLabel, headBranch string) (ahead, behind int, err error) {
	out, err := exec.CommandContext(
		ctx,
		"git", "-C", dir,
		"rev-list", "--left-right", "--count",
		baseLabel+"..."+headBranch,
	).Output()
	if err != nil {
		logger.Infof("Ahead/Behind counts: %s, %s, %s, %s", out, dir, baseLabel, headBranch)
		return 0, 0, nil
	}

	_, err = fmt.Sscanf(strings.TrimSpace(string(out)), "%d %d", &ahead, &behind)
	if err != nil {
		return 0, 0, err
	}

	return ahead, behind, nil
}

func GetUpstreamByBranch(ctx context.Context, dir, branchName string) (string, error) {
	out, err := exec.CommandContext(
		ctx,
		"git",
		"-C",
		dir,
		"config",
		"--get",
		fmt.Sprintf("branch.%s.remote", branchName),
	).Output()
	if err != nil {
		return "", err
	}

	return strings.TrimSpace(string(out)), nil
}
