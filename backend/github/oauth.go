package github

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"

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
// It opens the system browser automatically for OAuth.
func Login(ctx context.Context) error {
	cmd := exec.CommandContext(ctx, "gh", "auth", "login",
		"--hostname", "github.com",
		"--git-protocol", "https",
		"--web",
	)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("gh auth login: %s", strings.TrimSpace(string(out)))
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
