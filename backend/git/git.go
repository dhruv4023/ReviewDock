package git

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os/exec"
	"strings"
)

type LogWriter func(message string)

type Executor struct{}

func NewExecutor() *Executor {
	return &Executor{}
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

	// Read stdout and stderr concurrently
	outputChan := make(chan string)
	readPipe := func(reader io.Reader) {
		scanner := bufio.NewScanner(reader)
		for scanner.Scan() {
			outputChan <- scanner.Text()
		}
	}

	go readPipe(stdoutPipe)
	go readPipe(stderrPipe)

	// Wait for readers in background
	go func() {
		_ = cmd.Wait()
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

// LocalAheadBehind returns how many commits the local branch is ahead and behind
// its remote tracking branch (remote/branch). Returns 0, 0, nil if the branch
// does not exist locally or has no remote tracking ref — not treated as an error.
func LocalAheadBehind(ctx context.Context, dir, branch string) (ahead, behind int, err error) {
	out, err := exec.CommandContext(
		ctx,
		"git", "-C", dir,
		"rev-list", "--left-right", "--count",
		branch+"..."+branch+"@{upstream}",
	).Output()
	if err != nil {
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
