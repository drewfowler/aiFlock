package git

import (
	"fmt"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Cache for git status results
var (
	statusCache     = make(map[string]cachedStatus)
	statusCacheMu   sync.RWMutex
	cacheTTL        = 30 * time.Second // Refresh every 30 seconds
)

type cachedStatus struct {
	status    BranchStatus
	fetchedAt time.Time
}

// BranchStatus holds the ahead/behind commit counts relative to main
type BranchStatus struct {
	Branch  string
	Ahead   int
	Behind  int
	IsMain  bool  // True if on main/master branch
	Error   error // Non-nil if we couldn't determine status
}

// GetBranchStatus returns the current branch's ahead/behind status relative to main
// Results are cached for 30 seconds to avoid slow renders
// If the directory is not a git repo or there's an error, Error will be set
func GetBranchStatus(dir string) BranchStatus {
	if dir == "" || dir == "." {
		// Use current directory
		dir = "."
	}

	// Check cache first
	statusCacheMu.RLock()
	if cached, ok := statusCache[dir]; ok && time.Since(cached.fetchedAt) < cacheTTL {
		statusCacheMu.RUnlock()
		return cached.status
	}
	statusCacheMu.RUnlock()

	// Fetch fresh status
	status := fetchBranchStatus(dir)

	// Update cache
	statusCacheMu.Lock()
	statusCache[dir] = cachedStatus{status: status, fetchedAt: time.Now()}
	statusCacheMu.Unlock()

	return status
}

// fetchBranchStatus does the actual git commands to get branch status
func fetchBranchStatus(dir string) BranchStatus {
	// Get current branch name
	branch, err := getCurrentBranch(dir)
	if err != nil {
		return BranchStatus{Error: err}
	}

	// Determine the main branch (main or master)
	mainBranch := getMainBranch(dir)
	if mainBranch == "" {
		return BranchStatus{Branch: branch, Error: fmt.Errorf("no main branch")}
	}

	// If we're on main, just return that
	if branch == mainBranch {
		return BranchStatus{Branch: branch, IsMain: true}
	}

	// Get ahead/behind counts relative to main
	ahead, behind, err := getAheadBehind(dir, mainBranch, branch)
	if err != nil {
		return BranchStatus{Branch: branch, Error: err}
	}

	return BranchStatus{
		Branch: branch,
		Ahead:  ahead,
		Behind: behind,
	}
}

// getCurrentBranch returns the current branch name
func getCurrentBranch(dir string) (string, error) {
	cmd := exec.Command("git", "-C", dir, "rev-parse", "--abbrev-ref", "HEAD")
	output, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("not a git repo")
	}
	return strings.TrimSpace(string(output)), nil
}

// getMainBranch determines if the repo uses "main" or "master" as the primary branch
func getMainBranch(dir string) string {
	// Check if 'main' branch exists
	cmd := exec.Command("git", "-C", dir, "rev-parse", "--verify", "main")
	if err := cmd.Run(); err == nil {
		return "main"
	}

	// Check if 'master' branch exists
	cmd = exec.Command("git", "-C", dir, "rev-parse", "--verify", "master")
	if err := cmd.Run(); err == nil {
		return "master"
	}

	return ""
}

// getAheadBehind returns how many commits the current branch is ahead/behind relative to the base branch
func getAheadBehind(dir, baseBranch, currentBranch string) (ahead, behind int, err error) {
	// Use git rev-list to count commits
	// Ahead: commits in current branch not in base
	// Behind: commits in base not in current branch
	cmd := exec.Command("git", "-C", dir, "rev-list", "--left-right", "--count", baseBranch+"..."+currentBranch)
	output, err := cmd.Output()
	if err != nil {
		return 0, 0, fmt.Errorf("failed to get commit counts")
	}

	// Output format: "behind\tahead\n"
	parts := strings.Fields(strings.TrimSpace(string(output)))
	if len(parts) != 2 {
		return 0, 0, fmt.Errorf("unexpected git output")
	}

	behind, err = strconv.Atoi(parts[0])
	if err != nil {
		return 0, 0, fmt.Errorf("invalid behind count")
	}

	ahead, err = strconv.Atoi(parts[1])
	if err != nil {
		return 0, 0, fmt.Errorf("invalid ahead count")
	}

	return ahead, behind, nil
}

// FormatStatus returns a compact string representation of the branch status
// Examples: "main", "+3/-2", "+5", "-1", "err"
func (s BranchStatus) FormatStatus() string {
	if s.Error != nil {
		return "-"
	}
	if s.IsMain {
		return "main"
	}

	// Format: +ahead/-behind, omit zeros
	if s.Ahead == 0 && s.Behind == 0 {
		return "="
	}
	if s.Behind == 0 {
		return fmt.Sprintf("+%d", s.Ahead)
	}
	if s.Ahead == 0 {
		return fmt.Sprintf("-%d", s.Behind)
	}
	return fmt.Sprintf("+%d/-%d", s.Ahead, s.Behind)
}
