package main

import (
	"context"
	"fmt"
	"os/exec"
	"regexp"
	"sync"
	"time"

	wails "github.com/wailsapp/wails/v2/pkg/runtime"

	"my-github-pr/backend/git"
	"my-github-pr/backend/github"
	"my-github-pr/backend/models"
	"my-github-pr/backend/queue"
	"my-github-pr/backend/storage"
)

type App struct {
	ctx          context.Context
	storage      *storage.Service
	gitExecutor  *git.Executor
	queueManager *queue.Manager
	ghClient     *github.Client
}

func NewApp() *App {
	return &App{
		gitExecutor: git.NewExecutor(),
		ghClient:    github.NewClient(),
	}
}

func (a *App) startup(ctx context.Context) {
	a.ctx = ctx

	// Initialize Storage
	store, err := storage.NewService("github-pr-manager")
	if err != nil {
		wails.LogErrorf(a.ctx, "Failed initializing storage: %v", err)
	}
	a.storage = store

	// Load Settings
	settings, err := a.storage.ReadSettings()
	if err != nil {
		wails.LogErrorf(a.ctx, "Failed reading settings: %v", err)
		settings = &models.Settings{ConcurrencyLimit: 3}
	}

	// Initialize Queue Manager
	logCallback := func(msg string) {
		wails.EventsEmit(a.ctx, "terminal:log", msg)
	}
	a.queueManager = queue.NewManager(settings.ConcurrencyLimit, a.gitExecutor, logCallback)
	a.queueManager.Start(a.ctx)

	// Log startup git check
	if ver, err := a.gitExecutor.CheckGitVersion(a.ctx); err != nil {
		logCallback(fmt.Sprintf("\u001b[31m[STARTUP] ERROR: %v\u001b[0m\r\n", err))
	} else {
		logCallback(fmt.Sprintf("\u001b[32m[STARTUP] Detected: %s\u001b[0m\r\n", ver))
	}
}

func (a *App) shutdown(ctx context.Context) {
	if a.queueManager != nil {
		a.queueManager.Stop()
	}
}

// GetRepositories retrieves tracked repositories
func (a *App) GetRepositories() ([]models.Repository, error) {
	return a.storage.ReadRepos()
}

// AddRepository parses the git remote from a local path and tracks it
func (a *App) AddRepository(localPath string) (models.Repository, error) {
	// Sanity Check: Is valid git directory?
	cmd := exec.CommandContext(a.ctx, "git", "remote", "-v")
	cmd.Dir = localPath
	out, err := cmd.Output()
	if err != nil {
		return models.Repository{}, fmt.Errorf("provided path is not a valid git repository: %w", err)
	}

	// Parse remote owner and name
	// Matches git@github.com:owner/name.git or https://github.com/owner/name.git
	re := regexp.MustCompile(`github\.com[:/]([^/\s]+)/([^/\s\.]+)(?:\.git)?`)
	matches := re.FindStringSubmatch(string(out))
	if len(matches) < 3 {
		return models.Repository{}, fmt.Errorf("could not find github remote in repository configuration")
	}

	owner := matches[1]
	name := matches[2]
	repoID := fmt.Sprintf("%s-%s", owner, name)

	repos, err := a.storage.ReadRepos()
	if err != nil {
		repos = []models.Repository{}
	}

	// Check if already exists
	for _, r := range repos {
		if r.ID == repoID {
			return r, nil
		}
	}

	newRepo := models.Repository{
		ID:            repoID,
		Owner:         owner,
		Name:          name,
		LocalPath:     localPath,
		SyncStatus:    "synced",
		LastFetchedAt: time.Now(),
	}

	repos = append(repos, newRepo)
	if err := a.storage.WriteRepos(repos); err != nil {
		return models.Repository{}, err
	}

	return newRepo, nil
}

// RemoveRepository untracks a repo
func (a *App) RemoveRepository(id string) error {
	repos, err := a.storage.ReadRepos()
	if err != nil {
		return err
	}

	var updated []models.Repository
	for _, r := range repos {
		if r.ID != id {
			updated = append(updated, r)
		}
	}

	return a.storage.WriteRepos(updated)
}

// GetSettings fetches preferences
func (a *App) GetSettings() (*models.Settings, error) {
	return a.storage.ReadSettings()
}

// SaveSettings persists preferences
func (a *App) SaveSettings(settings *models.Settings) error {
	return a.storage.WriteSettings(settings)
}

// GetSession checks if a user is authenticated via gh CLI
func (a *App) GetSession() (*models.Session, error) {
	return github.GetSession(a.ctx)
}

// Logout revokes the active gh CLI session
func (a *App) Logout() error {
	return github.Logout(a.ctx)
}

// LoginGitHub initiates the GitHub OAuth flow via gh auth login
func (a *App) LoginGitHub() error {
	go func() {
		if err := github.Login(context.Background()); err != nil {
			wails.EventsEmit(a.ctx, "oauth:error", err.Error())
			return
		}

		session, err := github.GetSession(a.ctx)
		if err != nil || session == nil {
			wails.EventsEmit(a.ctx, "oauth:error", "Login succeeded but could not load profile")
			return
		}

		wails.EventsEmit(a.ctx, "oauth:success", session)
	}()

	return nil
}

// GetPullRequests aggregates active PRs for all repositories
func (a *App) GetPullRequests() ([]models.PullRequest, error) {
	// Verify gh is authenticated before making any calls
	session, err := github.GetSession(a.ctx)
	if err != nil || session == nil {
		return nil, fmt.Errorf("unauthorized: please run 'gh auth login' first")
	}

	repos, err := a.storage.ReadRepos()
	if err != nil {
		return nil, err
	}

	var allPRs []models.PullRequest
	var wg sync.WaitGroup
	var mu sync.Mutex
	errsChan := make(chan error, len(repos))

	for _, repo := range repos {
		wg.Add(1)
		go func(r models.Repository) {
			defer wg.Done()
			prs, err := a.ghClient.FetchPRs(a.ctx, r.Owner, r.Name, r.LocalPath)
			if err != nil {
				errsChan <- fmt.Errorf("failed fetching for %s/%s: %w", r.Owner, r.Name, err)
				return
			}
			mu.Lock()
			allPRs = append(allPRs, prs...)
			mu.Unlock()
		}(repo)
	}


	wg.Wait()
	close(errsChan)

	for e := range errsChan {
		wails.LogErrorf(a.ctx, "%v", e)
	}

	return allPRs, nil
}

// GetPRCIStatus fetches status checks for a PR commit ref
func (a *App) GetPRCIStatus(repoID string, headRef string) (string, error) {
	repos, err := a.storage.ReadRepos()
	if err != nil {
		return "unknown", err
	}

	var targetRepo *models.Repository
	for _, r := range repos {
		if r.ID == repoID {
			targetRepo = &r
			break
		}
	}
	if targetRepo == nil {
		return "unknown", fmt.Errorf("repository not found")
	}

	return a.ghClient.FetchCombinedCIStatus(a.ctx, targetRepo.Owner, targetRepo.Name, headRef)
}

// RebasePRs submits selected PRs for rebase jobs
func (a *App) RebasePRs(requests []models.RebaseRequest) error {
	repos, err := a.storage.ReadRepos()
	if err != nil {
		return err
	}

	settings, err := a.storage.ReadSettings()
	if err != nil {
		settings = &models.Settings{
			ConcurrencyLimit:      3,
			DefaultRemotePriority: []string{"origin", "upstream", "odoo", "ent"},
			AmendCommitTimestamp:  true,
			ForcePushAfterRebase:  false,
		}
	}

	repoMap := make(map[string]models.Repository)
	for _, r := range repos {
		repoMap[r.ID] = r
	}

	for _, req := range requests {
		repo, exists := repoMap[req.RepoID]
		if !exists {
			continue
		}

		job := queue.Job{
			ID:         req.ID,
			RepoName:   repo.Owner + "/" + repo.Name,
			RepoPath:   repo.LocalPath,
			HeadBranch: req.HeadBranch,
			BaseBranch: req.BaseBranch,
			Options:    *settings,
		}

		a.queueManager.Submit(job)
	}

	return nil
}

// CancelRebase cancels a queued or running job
func (a *App) CancelRebase(jobID string) error {
	if a.queueManager != nil {
		a.queueManager.Cancel(jobID)
	}
	return nil
}
