package git

import (
	"os"
	"path/filepath"
	"testing"
)

func TestIsGitRepo(t *testing.T) {
	// Current directory should be a git repo (we're in the flock project)
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("failed to get cwd: %v", err)
	}

	if !IsGitRepo(cwd) {
		t.Error("expected current directory to be a git repo")
	}

	// Temp directory should not be a git repo
	tmpDir, err := os.MkdirTemp("", "notgit")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	if IsGitRepo(tmpDir) {
		t.Error("expected temp directory to not be a git repo")
	}
}

func TestGetRepoRoot(t *testing.T) {
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("failed to get cwd: %v", err)
	}

	root, err := GetRepoRoot(cwd)
	if err != nil {
		t.Fatalf("failed to get repo root: %v", err)
	}

	// The root should contain a .git directory
	gitDir := filepath.Join(root, ".git")
	if _, err := os.Stat(gitDir); os.IsNotExist(err) {
		t.Errorf("repo root %s does not contain .git", root)
	}
}

func TestListWorktrees(t *testing.T) {
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("failed to get cwd: %v", err)
	}

	root, err := GetRepoRoot(cwd)
	if err != nil {
		t.Fatalf("failed to get repo root: %v", err)
	}

	worktrees, err := ListWorktrees(root)
	if err != nil {
		t.Fatalf("failed to list worktrees: %v", err)
	}

	// Should have at least the main worktree
	if len(worktrees) < 1 {
		t.Error("expected at least one worktree (main)")
	}

	// First worktree should be the repo root
	if len(worktrees) > 0 && worktrees[0].Path != root {
		t.Errorf("expected first worktree to be %s, got %s", root, worktrees[0].Path)
	}
}

func TestIsFlockWorktree(t *testing.T) {
	tests := []struct {
		path     string
		expected bool
	}{
		{"/home/user/project/.flock-worktrees/flock-001", true},
		{"/home/user/project/.flock-worktrees/flock-spare-001", true},
		{"/home/user/project", false},
		{"/home/user/project/.git/worktrees/some-worktree", false},
	}

	for _, tt := range tests {
		result := IsFlockWorktree(tt.path)
		if result != tt.expected {
			t.Errorf("IsFlockWorktree(%s) = %v, expected %v", tt.path, result, tt.expected)
		}
	}
}

func TestBranchName(t *testing.T) {
	result := BranchName("001")
	expected := "flock-001"
	if result != expected {
		t.Errorf("BranchName(001) = %s, expected %s", result, expected)
	}
}

func TestWorktreePath(t *testing.T) {
	result := WorktreePath("/home/user/project", "001")
	expected := "/home/user/project/.flock-worktrees/flock-001"
	if result != expected {
		t.Errorf("WorktreePath result = %s, expected %s", result, expected)
	}
}
