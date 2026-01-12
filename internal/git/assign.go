package git

import (
	"fmt"
	"os"
	"sync"
)

// WorktreeAssignment holds info about a task's worktree assignment
type WorktreeAssignment struct {
	WorktreePath string
	GitBranch    string
	RepoRoot     string
}

// Assigner manages worktree assignment for tasks
type Assigner struct {
	mu             sync.Mutex
	maxPerRepo     int
	enabled        bool
	creatingWorktrees map[string]bool // tracks worktrees currently being created
}

// NewAssigner creates a new worktree assigner
func NewAssigner(enabled bool, maxPerRepo int) *Assigner {
	return &Assigner{
		enabled:           enabled,
		maxPerRepo:        maxPerRepo,
		creatingWorktrees: make(map[string]bool),
	}
}

// TaskWorktreeInfo is the interface that tasks must implement for worktree assignment
type TaskWorktreeInfo interface {
	GetID() string
	GetCwd() string
	GetWorktreePath() string
}

// AssignWorktree assigns a worktree to a task, creating one if needed
// Returns the assignment info or nil if worktrees are disabled or not in a git repo
func (a *Assigner) AssignWorktree(taskID, taskCwd string, activeTasks []TaskWorktreeInfo) (*WorktreeAssignment, error) {
	if !a.enabled {
		return nil, nil
	}

	// Check if we're in a git repo
	if !IsGitRepo(taskCwd) {
		return nil, nil
	}

	repoRoot, err := GetRepoRoot(taskCwd)
	if err != nil {
		return nil, nil
	}

	// Check if the task's cwd is already a worktree
	if IsPathInWorktree(taskCwd) {
		// Already in a worktree, return its info
		branch, err := GetCurrentBranch(taskCwd)
		if err != nil {
			return nil, nil
		}
		return &WorktreeAssignment{
			WorktreePath: taskCwd,
			GitBranch:    branch,
			RepoRoot:     repoRoot,
		}, nil
	}

	a.mu.Lock()
	defer a.mu.Unlock()

	// Find a free worktree
	freePath, err := a.findFreeWorktree(repoRoot, activeTasks)
	if err != nil {
		return nil, fmt.Errorf("failed to find free worktree: %w", err)
	}

	var assignment *WorktreeAssignment

	if freePath != "" {
		// Use existing free worktree
		worktrees, _ := ListWorktrees(repoRoot)
		for _, wt := range worktrees {
			if wt.Path == freePath {
				// Reset the branch to the current default branch HEAD
				// This ensures the reused worktree starts fresh with latest code
				if err := ResetWorktreeBranch(wt.Path); err != nil {
					return nil, fmt.Errorf("failed to reset worktree branch: %w", err)
				}

				assignment = &WorktreeAssignment{
					WorktreePath: wt.Path,
					GitBranch:    wt.Branch,
					RepoRoot:     repoRoot,
				}
				break
			}
		}
	} else {
		// Need to create a new worktree
		// First check if we've hit the max
		flockWorktreeCount := a.countFlockWorktrees(repoRoot)
		if a.maxPerRepo > 0 && flockWorktreeCount >= a.maxPerRepo {
			return nil, fmt.Errorf("maximum worktrees (%d) reached for this repository", a.maxPerRepo)
		}

		// Create new worktree
		worktreePath := WorktreePath(repoRoot, taskID)
		branch := BranchName(taskID)

		if err := a.ensureWorktreeDir(repoRoot); err != nil {
			return nil, fmt.Errorf("failed to create worktree directory: %w", err)
		}

		if err := CreateWorktree(repoRoot, worktreePath, branch); err != nil {
			return nil, fmt.Errorf("failed to create worktree: %w", err)
		}

		assignment = &WorktreeAssignment{
			WorktreePath: worktreePath,
			GitBranch:    branch,
			RepoRoot:     repoRoot,
		}
	}

	// Trigger background +1 creation if needed
	go a.ensurePlusOne(repoRoot, activeTasks, taskID)

	return assignment, nil
}

// ReleaseWorktree releases a worktree when a task is deleted
func (a *Assigner) ReleaseWorktree(worktreePath, repoRoot string) error {
	if worktreePath == "" || repoRoot == "" {
		return nil
	}

	return RemoveWorktree(repoRoot, worktreePath, true)
}

// findFreeWorktree finds a free flock worktree in the repo
func (a *Assigner) findFreeWorktree(repoRoot string, activeTasks []TaskWorktreeInfo) (string, error) {
	worktrees, err := ListWorktrees(repoRoot)
	if err != nil {
		return "", err
	}

	// Build set of assigned paths
	assignedPaths := make(map[string]bool)
	for _, t := range activeTasks {
		if t.GetWorktreePath() != "" {
			assignedPaths[t.GetWorktreePath()] = true
		}
	}

	// Also exclude worktrees currently being created
	for path := range a.creatingWorktrees {
		assignedPaths[path] = true
	}

	// Find free flock worktree
	for _, wt := range worktrees {
		if IsFlockWorktree(wt.Path) && !assignedPaths[wt.Path] {
			return wt.Path, nil
		}
	}

	return "", nil // None free
}

// countFlockWorktrees counts the number of flock-managed worktrees
func (a *Assigner) countFlockWorktrees(repoRoot string) int {
	worktrees, err := ListWorktrees(repoRoot)
	if err != nil {
		return 0
	}

	count := 0
	for _, wt := range worktrees {
		if IsFlockWorktree(wt.Path) {
			count++
		}
	}
	return count
}

// ensureWorktreeDir creates the .flock-worktrees directory if it doesn't exist
func (a *Assigner) ensureWorktreeDir(repoRoot string) error {
	dir := WorktreeDirPath(repoRoot)
	return os.MkdirAll(dir, 0755)
}

// ensurePlusOne creates an additional worktree in the background if needed
func (a *Assigner) ensurePlusOne(repoRoot string, activeTasks []TaskWorktreeInfo, excludeTaskID string) {
	a.mu.Lock()

	// Count free worktrees (excluding the one we just assigned)
	freeCount := 0
	worktrees, err := ListWorktrees(repoRoot)
	if err != nil {
		a.mu.Unlock()
		return
	}

	assignedPaths := make(map[string]bool)
	for _, t := range activeTasks {
		if t.GetWorktreePath() != "" {
			assignedPaths[t.GetWorktreePath()] = true
		}
	}

	for _, wt := range worktrees {
		if IsFlockWorktree(wt.Path) && !assignedPaths[wt.Path] {
			freeCount++
		}
	}

	// If we already have at least 1 free, no need to create more
	if freeCount >= 1 {
		a.mu.Unlock()
		return
	}

	// Check if we've hit the max
	flockWorktreeCount := a.countFlockWorktrees(repoRoot)
	if a.maxPerRepo > 0 && flockWorktreeCount >= a.maxPerRepo {
		a.mu.Unlock()
		return
	}

	// Generate a unique ID for the +1 worktree
	nextID := fmt.Sprintf("spare-%03d", flockWorktreeCount+1)
	worktreePath := WorktreePath(repoRoot, nextID)

	// Mark as creating
	a.creatingWorktrees[worktreePath] = true
	a.mu.Unlock()

	// Create the worktree (outside lock)
	branch := BranchName(nextID)
	_ = a.ensureWorktreeDir(repoRoot)
	_ = CreateWorktree(repoRoot, worktreePath, branch)

	// Unmark as creating
	a.mu.Lock()
	delete(a.creatingWorktrees, worktreePath)
	a.mu.Unlock()
}

// CountFreeWorktrees returns the number of free worktrees for a repo
func (a *Assigner) CountFreeWorktrees(repoRoot string, activeTasks []TaskWorktreeInfo) int {
	a.mu.Lock()
	defer a.mu.Unlock()

	worktrees, err := ListWorktrees(repoRoot)
	if err != nil {
		return 0
	}

	assignedPaths := make(map[string]bool)
	for _, t := range activeTasks {
		if t.GetWorktreePath() != "" {
			assignedPaths[t.GetWorktreePath()] = true
		}
	}

	count := 0
	for _, wt := range worktrees {
		if IsFlockWorktree(wt.Path) && !assignedPaths[wt.Path] {
			count++
		}
	}
	return count
}
