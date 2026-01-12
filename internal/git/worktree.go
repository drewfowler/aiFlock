package git

import (
	"bufio"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
)

const (
	// FlockWorktreeDir is the directory name where flock worktrees are stored
	FlockWorktreeDir = ".flock-worktrees"
	// FlockWorktreePrefix is the prefix for flock-managed worktree directories
	FlockWorktreePrefix = "flock-"
)

// Worktree represents a git worktree entry
type Worktree struct {
	Path   string
	Commit string
	Branch string
}

// IsGitRepo checks if the given path is inside a git repository
func IsGitRepo(path string) bool {
	cmd := exec.Command("git", "-C", path, "rev-parse", "--is-inside-work-tree")
	output, err := cmd.Output()
	if err != nil {
		return false
	}
	return strings.TrimSpace(string(output)) == "true"
}

// GetRepoRoot returns the root directory of the git repository containing the given path
func GetRepoRoot(path string) (string, error) {
	cmd := exec.Command("git", "-C", path, "rev-parse", "--show-toplevel")
	output, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("not a git repository: %w", err)
	}
	return strings.TrimSpace(string(output)), nil
}

// GetDefaultBranch returns the default branch name (main or master)
func GetDefaultBranch(repoRoot string) (string, error) {
	// Try to get the default branch from remote
	cmd := exec.Command("git", "-C", repoRoot, "symbolic-ref", "refs/remotes/origin/HEAD")
	output, err := cmd.Output()
	if err == nil {
		// refs/remotes/origin/main -> main
		ref := strings.TrimSpace(string(output))
		parts := strings.Split(ref, "/")
		if len(parts) > 0 {
			return parts[len(parts)-1], nil
		}
	}

	// Fallback: check if main exists
	cmd = exec.Command("git", "-C", repoRoot, "show-ref", "--verify", "--quiet", "refs/heads/main")
	if err := cmd.Run(); err == nil {
		return "main", nil
	}

	// Fallback: check if master exists
	cmd = exec.Command("git", "-C", repoRoot, "show-ref", "--verify", "--quiet", "refs/heads/master")
	if err := cmd.Run(); err == nil {
		return "master", nil
	}

	return "main", nil // Default to main
}

// ListWorktrees returns all worktrees for the given repository
func ListWorktrees(repoRoot string) ([]Worktree, error) {
	cmd := exec.Command("git", "-C", repoRoot, "worktree", "list", "--porcelain")
	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("failed to list worktrees: %w", err)
	}

	var worktrees []Worktree
	var current Worktree

	scanner := bufio.NewScanner(strings.NewReader(string(output)))
	for scanner.Scan() {
		line := scanner.Text()

		if line == "" {
			if current.Path != "" {
				worktrees = append(worktrees, current)
			}
			current = Worktree{}
			continue
		}

		if strings.HasPrefix(line, "worktree ") {
			current.Path = strings.TrimPrefix(line, "worktree ")
		} else if strings.HasPrefix(line, "HEAD ") {
			current.Commit = strings.TrimPrefix(line, "HEAD ")
		} else if strings.HasPrefix(line, "branch ") {
			// refs/heads/main -> main
			ref := strings.TrimPrefix(line, "branch ")
			current.Branch = strings.TrimPrefix(ref, "refs/heads/")
		}
	}

	// Don't forget the last entry
	if current.Path != "" {
		worktrees = append(worktrees, current)
	}

	return worktrees, scanner.Err()
}

// CreateWorktree creates a new worktree with the given branch name
func CreateWorktree(repoRoot, worktreePath, branch string) error {
	// Create the worktree with a new branch based on the default branch
	defaultBranch, err := GetDefaultBranch(repoRoot)
	if err != nil {
		return fmt.Errorf("failed to get default branch: %w", err)
	}

	cmd := exec.Command("git", "-C", repoRoot, "worktree", "add", "-b", branch, worktreePath, defaultBranch)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to create worktree: %s: %w", string(output), err)
	}

	return nil
}

// RemoveWorktree removes a worktree and optionally its branch
func RemoveWorktree(repoRoot, worktreePath string, deleteBranch bool) error {
	// Get the branch name before removing
	var branch string
	if deleteBranch {
		worktrees, err := ListWorktrees(repoRoot)
		if err == nil {
			for _, wt := range worktrees {
				if wt.Path == worktreePath {
					branch = wt.Branch
					break
				}
			}
		}
	}

	// Remove the worktree
	cmd := exec.Command("git", "-C", repoRoot, "worktree", "remove", "--force", worktreePath)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to remove worktree: %s: %w", string(output), err)
	}

	// Delete the branch if requested and it's a flock branch
	if deleteBranch && branch != "" && strings.HasPrefix(branch, FlockWorktreePrefix) {
		cmd = exec.Command("git", "-C", repoRoot, "branch", "-D", branch)
		// Ignore errors - branch may already be deleted
		_ = cmd.Run()
	}

	return nil
}

// WorktreeDirPath returns the path to the flock worktrees directory for a repo
func WorktreeDirPath(repoRoot string) string {
	return filepath.Join(repoRoot, FlockWorktreeDir)
}

// WorktreePath returns the full path for a worktree with the given ID
func WorktreePath(repoRoot, worktreeID string) string {
	return filepath.Join(repoRoot, FlockWorktreeDir, FlockWorktreePrefix+worktreeID)
}

// BranchName returns the branch name for a worktree with the given ID
func BranchName(worktreeID string) string {
	return FlockWorktreePrefix + worktreeID
}

// IsFlockWorktree checks if the given worktree path is a flock-managed worktree
func IsFlockWorktree(path string) bool {
	base := filepath.Base(path)
	return strings.HasPrefix(base, FlockWorktreePrefix)
}

// IsPathInWorktree checks if the given path is inside a worktree (not the main repo)
func IsPathInWorktree(path string) bool {
	cmd := exec.Command("git", "-C", path, "rev-parse", "--is-inside-work-tree")
	if err := cmd.Run(); err != nil {
		return false
	}

	// Check if this is a worktree by looking for .git file (worktrees have a .git file, not directory)
	gitPath := filepath.Join(path, ".git")
	cmd = exec.Command("test", "-f", gitPath)
	return cmd.Run() == nil
}
