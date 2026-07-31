package main

import (
	"context"
	"fmt"
	"os/exec"
	"regexp"
	"strings"
	"sync"
	"time"

	wails "github.com/wailsapp/wails/v2/pkg/runtime"

	"review-dock/backend/git"
	"review-dock/backend/github"
	"review-dock/backend/models"
	"review-dock/backend/queue"
	"review-dock/backend/storage"
	"review-dock/logger"
)

type App struct {
	ctx          context.Context
	storage      *storage.Service
	gitExecutor  *git.Executor
	queueManager *queue.Manager
	ghClient     *github.Client
	lastCronRun  time.Time
	cronMutex    sync.Mutex
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
	store, err := storage.NewService("review-dock-manager")
	if err != nil {
		wails.LogErrorf(a.ctx, "Failed initializing storage: %v", err)
	}
	a.storage = store

	// Load Settings
	settings, err := a.storage.ReadSettings()
	if err != nil {
		wails.LogErrorf(a.ctx, "Failed reading settings: %v", err)
		settings = &models.Settings{ConcurrencyLimit: 2}
	}

	// Initialize Queue Manager
	logCallback := func(msg string) {
		wails.EventsEmit(a.ctx, "terminal:log", msg)
	}
	statusCallback := func(jobID string, status string, errMsg string, active int, queued int) {
		wails.EventsEmit(a.ctx, "rebase:status", map[string]interface{}{
			"job_id":       jobID,
			"status":       status,
			"error":        errMsg,
			"active_count": active,
			"queued_count": queued,
		})
	}
	a.queueManager = queue.NewManager(settings.ConcurrencyLimit, a.gitExecutor, logCallback, statusCallback)
	a.queueManager.Start(a.ctx)

	// Start Scheduled Cron Rebase & Push check loop
	go a.startCronScheduler()

	// Log startup environment and git check
	env := wails.Environment(a.ctx)
	logCallback(fmt.Sprintf("\u001b[32m[STARTUP] Running in %s mode on %s/%s\u001b[0m\r\n", env.BuildType, env.Platform, env.Arch))

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

// LoginGitHub initiates the GitHub authentication flow via gh auth login
func (a *App) LoginGitHub() error {
	go func() {
		codeChan := make(chan string, 1)
		urlChan := make(chan string, 1)

		go func() {
			var code, url string
			select {
			case code = <-codeChan:
				select {
				case url = <-urlChan:
				default:
					url = "https://github.com/login/device"
				}
			case <-a.ctx.Done():
				return
			}

			wails.EventsEmit(a.ctx, "oauth:device_code", map[string]string{
				"code": code,
				"url":  url,
			})
		}()

		if err := github.Login(a.ctx, codeChan, urlChan); err != nil {
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
func (a *App) GetPullRequests(remoteUpdate bool) ([]models.PullRequest, error) {
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

			lock := a.gitExecutor.GetRepoLock(r.LocalPath)
			lock.Lock()
			defer lock.Unlock()
			if remoteUpdate {
				a.queueManager.ProcessRemoteUpdate(a.ctx, r.LocalPath)
			}
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
			ConcurrencyLimit:     2,
			AmendCommitTimestamp: true,
			ForcePushAfterRebase: false,
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
		logger.Infof("%v | %v", req, repo)
		job := queue.Job{
			ID:        req.ID,
			RepoName:  repo.Owner + "/" + repo.Name,
			RepoPath:  repo.LocalPath,
			HeadLabel: req.HeadLabel,
			BaseLabel: req.BaseLabel,
			Options:   *settings,
		}

		a.queueManager.Submit(job)
	}

	return nil
}

// GetRemotes returns the list of git remotes for the given repository.
// The frontend uses this to populate the remote selection dialog.
func (a *App) GetRemotes(repoID string) ([]string, error) {
	repos, err := a.storage.ReadRepos()
	if err != nil {
		return nil, err
	}
	for _, r := range repos {
		if r.ID == repoID {
			return a.gitExecutor.ListRemotes(a.ctx, r.LocalPath)
		}
	}
	return nil, fmt.Errorf("repository not found: %s", repoID)
}

// SetBranchTracking sets the upstream tracking remote for a local branch.
// This is called from the frontend when the user picks a remote for an
// untracked head branch before a rebase or force-push operation.
func (a *App) SetBranchTracking(repoID, branch, remote string) error {
	repos, err := a.storage.ReadRepos()
	if err != nil {
		return err
	}
	for _, r := range repos {
		if r.ID == repoID {
			lock := a.gitExecutor.GetRepoLock(r.LocalPath)
			lock.Lock()
			defer lock.Unlock()

			logCallback := func(msg string) {
				wails.EventsEmit(a.ctx, "terminal:log", fmt.Sprintf("[%s] %s", r.Owner+"/"+r.Name, msg))
			}
			if err := a.gitExecutor.SetBranchTracking(a.ctx, r.LocalPath, branch, remote, logCallback); err != nil {
				return fmt.Errorf("failed setting tracking for branch '%s' to remote '%s': %w", branch, remote, err)
			}
			logCallback(fmt.Sprintf("\u001b[32mBranch '%s' now tracking '%s/%s'\u001b[0m\r\n", branch, remote, branch))
			return nil
		}
	}
	return fmt.Errorf("repository not found: %s", repoID)
}

// CancelRebase cancels a queued or running job
func (a *App) CancelRebase(jobID string) error {
	if a.queueManager != nil {
		a.queueManager.Cancel(jobID)
	}
	return nil
}

// GetPRDiff returns the git diff output comparing baseLabel to headBranch in target repository.
func (a *App) GetPRDiff(repoID string, baseLabel string, headBranch string) (string, error) {
	repos, err := a.storage.ReadRepos()
	if err != nil {
		return "", err
	}
	for _, r := range repos {
		if r.ID == repoID {
			lock := a.gitExecutor.GetRepoLock(r.LocalPath)
			lock.Lock()
			defer lock.Unlock()

			return a.gitExecutor.Diff(a.ctx, r.LocalPath, baseLabel, headBranch)
		}
	}
	return "", fmt.Errorf("repository not found: %s", repoID)
}

// GetReviewTemplate returns the saved PR review prompt template.
func (a *App) GetReviewTemplate() (string, error) {
	return a.storage.ReadReviewTemplate()
}

// SaveReviewTemplate persists the PR review prompt template.
func (a *App) SaveReviewTemplate(template string) error {
	return a.storage.WriteReviewTemplate(template)
}

// IsDev returns true if the application is running in development mode.
func (a *App) IsDev() bool {
	return wails.Environment(a.ctx).BuildType != "production"
}

func (a *App) startCronScheduler() {
	ticker := time.NewTicker(55 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-a.ctx.Done():
			return
		case <-ticker.C:
			a.checkAndTriggerCron()
		}
	}
}

func (a *App) checkAndTriggerCron() {
	a.cronMutex.Lock()
	defer a.cronMutex.Unlock()

	settings, err := a.storage.ReadSettings()
	if err != nil {
		return
	}

	if !settings.CronEnabled || len(settings.CronTimes) == 0 {
		return
	}

	now := time.Now()
	// Prevent running multiple times in the same minute
	if now.Hour() == a.lastCronRun.Hour() &&
		now.Minute() == a.lastCronRun.Minute() &&
		now.Year() == a.lastCronRun.Year() &&
		now.YearDay() == a.lastCronRun.YearDay() {
		return
	}

	// Format current time as "03:04 PM"
	currTimeStr := strings.ToUpper(now.Format("03:04 PM"))

	matched := false
	for _, t := range settings.CronTimes {
		if normalizeTimeStr(t) == currTimeStr {
			matched = true
			break
		}
	}

	if matched {
		a.lastCronRun = now
		// Run trigger in a separate goroutine so it doesn't block the scheduler tick
		go a.triggerScheduledRebaseAndPush()
	}
}

func normalizeTimeStr(t string) string {
	t = strings.ToUpper(strings.TrimSpace(t))
	if len(t) == 7 && t[1] == ':' { // e.g. "9:00 AM" -> "09:00 AM"
		return "0" + t
	}
	return t
}

func (a *App) triggerScheduledRebaseAndPush() {
	wails.EventsEmit(a.ctx, "terminal:log", "\u001b[35m[SCHEDULED] Scheduled auto-rebase & force-push triggered!\u001b[0m\r\n")

	prs, err := a.GetPullRequests(true)
	if err != nil {
		wails.EventsEmit(a.ctx, "terminal:log", fmt.Sprintf("\u001b[31m[SCHEDULED] Failed fetching pull requests: %v\u001b[0m\r\n", err))
		return
	}

	repos, err := a.storage.ReadRepos()
	if err != nil {
		wails.EventsEmit(a.ctx, "terminal:log", fmt.Sprintf("\u001b[31m[SCHEDULED] Failed reading repositories: %v\u001b[0m\r\n", err))
		return
	}
	repoMap := make(map[string]models.Repository)
	for _, r := range repos {
		repoMap[r.ID] = r
	}

	settings, err := a.storage.ReadSettings()
	if err != nil {
		settings = &models.Settings{
			ConcurrencyLimit:     2,
			AmendCommitTimestamp: true,
			ForcePushAfterRebase: true,
		}
	}

	// Ensure force push is enabled for the scheduled run
	scheduledSettings := *settings
	scheduledSettings.ForcePushAfterRebase = true

	// --- Branch-level eligibility check ---
	// Rule: if ANY PR sharing the same head branch name (across all repos) has
	// AheadCount > 100, then ALL PRs on that branch must be processed — not just
	// the one that crossed the threshold.
	// We scan ALL PRs (open + draft) here so that a draft PR with a high ahead
	// count can make the whole branch group eligible, and so that open PRs on the
	// same branch also get pulled in.
	eligibleBranches := make(map[string]bool) // key: bare branch name (no "owner/" prefix)
	for _, pr := range prs {
		if !strings.Contains(pr.HeadLabel, "/") {
			continue
		}
		// HeadLabel is "owner/branch" — extract just the branch portion
		parts := strings.SplitN(pr.HeadLabel, "/", 2)
		branchName := parts[1]
		if pr.AheadCount > 100 {
			eligibleBranches[branchName] = true
		}
	}

	count := 0
	skippedDirtyBranchs := make(map[string]bool)
	for _, pr := range prs {
		// Determine whether this PR is a candidate:
		//   - Always include open (non-draft) PRs.
		//   - Include draft PRs only when CronIncludeDrafts setting is on.
		//   - Never include closed PRs.
		isCandidate := pr.State == "open" || (pr.IsDraft && settings.CronIncludeDrafts)
		if !isCandidate || !strings.Contains(pr.HeadLabel, "/") {
			continue
		}

		repo, exists := repoMap[pr.RepoID]
		if !exists {
			continue
		}

		parts := strings.SplitN(pr.HeadLabel, "/", 2)
		branchName := parts[1]

		// Skip this PR if its branch is not eligible (no PR on this branch exceeds the threshold)
		if !eligibleBranches[branchName] && !skippedDirtyBranchs[branchName] {
			wails.EventsEmit(a.ctx, "terminal:log", fmt.Sprintf(
				"\u001b[33m[SCHEDULED] Skipping PR #%d (%s/%s): branch '%s' has no PR with ahead count > 100\u001b[0m\r\n",
				pr.Number, repo.Owner, repo.Name, branchName,
			))
			continue
		}

		// --- Git clean-status guard ---
		// If the working tree is dirty, the user is likely actively working on this
		// repo. Skip it to avoid interfering with uncommitted changes.
		clean, err := a.gitExecutor.IsClean(a.ctx, repo.LocalPath)
		if err != nil {
			wails.EventsEmit(a.ctx, "terminal:log", fmt.Sprintf(
				"\u001b[31m[SCHEDULED] Could not check git status for %s/%s: %v — skipping\u001b[0m\r\n",
				repo.Owner, repo.Name, err,
			))
			skippedDirtyBranchs[branchName] = true
			continue
		}
		if !clean {
			wails.EventsEmit(a.ctx, "terminal:log", fmt.Sprintf(
				"\u001b[33m[SCHEDULED] Skipping PR #%d (%s/%s): working tree is dirty (uncommitted changes detected)\u001b[0m\r\n",
				pr.Number, repo.Owner, repo.Name,
			))
			skippedDirtyBranchs[branchName] = true
			continue
		}

		stateLabel := "open"
		if pr.IsDraft {
			stateLabel = "draft"
		}
		wails.EventsEmit(a.ctx, "terminal:log", fmt.Sprintf(
			"\u001b[36m[SCHEDULED] Queuing %s PR #%d (%s/%s) on branch '%s' (↑%d ahead)\u001b[0m\r\n",
			stateLabel, pr.Number, repo.Owner, repo.Name, branchName, pr.AheadCount,
		))

		job := queue.Job{
			ID:        pr.ID,
			RepoName:  repo.Owner + "/" + repo.Name,
			RepoPath:  repo.LocalPath,
			HeadLabel: pr.HeadLabel,
			BaseLabel: pr.BaseLabel,
			Options:   scheduledSettings,
		}
		a.queueManager.Submit(job)
		count++
	}

	wails.EventsEmit(a.ctx, "terminal:log", fmt.Sprintf(
		"\u001b[32m[SCHEDULED] Submitted %d PR auto-rebase & force-push jobs to queue (%d skipped — dirty working tree).\u001b[0m\r\n",
		count, len(skippedDirtyBranchs),
	))
}
