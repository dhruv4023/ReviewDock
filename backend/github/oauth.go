package github

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os/exec"
	"regexp"
	"strings"
	"sync"

	"my-github-pr/backend/models"
)

// GetSession returns the current GitHub session from gh CLI auth state.
// Returns nil, nil when no user is authenticated (not an error).
func GetSession(ctx context.Context) (*models.Session, error) {
	// Check authentication status via exit code
	statusCmd := exec.CommandContext(ctx, "gh", "auth", "status")
	if err := statusCmd.Run(); err != nil {
		// Non-zero exit = not logged in
		return nil, nil
	}

	// Retrieve the active token
	tokenOut, err := exec.CommandContext(ctx, "gh", "auth", "token").Output()
	if err != nil {
		return nil, fmt.Errorf("gh auth token: %w", err)
	}
	token := strings.TrimSpace(string(tokenOut))

	// Fetch user profile
	user, err := fetchCurrentUser(ctx)
	if err != nil {
		return nil, err
	}

	return &models.Session{
		AccessToken: token,
		TokenType:   "bearer",
		Scope:       "repo,read:org",
		User:        user,
	}, nil
}

// Login initiates the GitHub authentication flow via gh auth login.
// It parses the one-time code and url, sending them back through the channels.
func Login(ctx context.Context, codeChan chan<- string, urlChan chan<- string) error {
	cmd := exec.CommandContext(ctx, "gh", "auth", "login",
		"--hostname", "github.com",
		"--git-protocol", "https",
	)

	stdinPipe, err := cmd.StdinPipe()
	if err != nil {
		return err
	}
	defer stdinPipe.Close()

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

	// Write newline to stdin to trigger non-interactive mode
	_, _ = stdinPipe.Write([]byte("\n"))
	stdinPipe.Close()

	var wg sync.WaitGroup
	parseOutput := func(r io.Reader) {
		scanner := bufio.NewScanner(r)
		// Match "one-time code: 1234-5678"
		codeRegex := regexp.MustCompile(`one-time code:\s*([A-Z0-9\-]+)`)
		// Match "https://github.com/login/device"
		urlRegex := regexp.MustCompile(`https://github.com/login/device`)

		for scanner.Scan() {
			line := scanner.Text()
			if matches := codeRegex.FindStringSubmatch(line); len(matches) > 1 {
				select {
				case codeChan <- matches[1]:
				case <-ctx.Done():
					return
				}
			}
			if urlRegex.MatchString(line) {
				select {
				case urlChan <- "https://github.com/login/device":
				case <-ctx.Done():
					return
				}
			}
		}
	}

	wg.Add(2)
	go func() {
		defer wg.Done()
		parseOutput(stdoutPipe)
	}()
	go func() {
		defer wg.Done()
		parseOutput(stderrPipe)
	}()

	wg.Wait()

	if err := cmd.Wait(); err != nil {
		return fmt.Errorf("gh auth login: %w", err)
	}
	return nil
}

// Logout revokes the active GitHub session via gh auth logout.
func Logout(ctx context.Context) error {
	cmd := exec.CommandContext(ctx, "gh", "auth", "logout",
		"--hostname", "github.com",
	)
	if out, err := cmd.CombinedOutput(); err != nil {
		msg := strings.TrimSpace(string(out))
		// Treat "not logged in" as a no-op, not an error
		if strings.Contains(msg, "not logged in") {
			return nil
		}
		return fmt.Errorf("gh auth logout: %s", msg)
	}
	return nil
}

// fetchCurrentUser retrieves the authenticated user profile using gh api.
func fetchCurrentUser(ctx context.Context) (*models.User, error) {
	out, err := exec.CommandContext(ctx, "gh", "api", "user").Output()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			return nil, fmt.Errorf("gh api user: %s", strings.TrimSpace(string(exitErr.Stderr)))
		}
		return nil, fmt.Errorf("gh api user: %w", err)
	}

	var user models.User
	if err := json.Unmarshal(out, &user); err != nil {
		return nil, fmt.Errorf("parsing user profile: %w", err)
	}
	return &user, nil
}
