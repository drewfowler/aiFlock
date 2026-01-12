package tui

import (
	"os/exec"
	"strconv"
	"strings"
)

// GitStatus holds the current git repository status
type GitStatus struct {
	Branch           string
	HasUncommitted   bool // Working tree has uncommitted changes
	HasUnpushed      bool // Local branch is ahead of remote
	IsBehind         bool // Local branch is behind remote
	UnpushedCount    int  // Number of commits ahead
	BehindCount      int  // Number of commits behind
}

// GetGitStatus returns the current git status for the working directory
func GetGitStatus() *GitStatus {
	status := &GitStatus{}

	// Get current branch name
	branch, err := exec.Command("git", "rev-parse", "--abbrev-ref", "HEAD").Output()
	if err != nil {
		return nil // Not a git repo or git not available
	}
	status.Branch = strings.TrimSpace(string(branch))

	// Check for uncommitted changes (both staged and unstaged)
	// git status --porcelain returns empty if clean
	porcelain, _ := exec.Command("git", "status", "--porcelain").Output()
	status.HasUncommitted = len(strings.TrimSpace(string(porcelain))) > 0

	// Check ahead/behind status relative to upstream
	// git rev-list --left-right --count @{upstream}...HEAD
	// Returns "behind\tahead" (tab-separated)
	revList, err := exec.Command("git", "rev-list", "--left-right", "--count", "@{upstream}...HEAD").Output()
	if err == nil {
		parts := strings.Fields(string(revList))
		if len(parts) == 2 {
			behind, _ := strconv.Atoi(parts[0])
			ahead, _ := strconv.Atoi(parts[1])
			status.BehindCount = behind
			status.UnpushedCount = ahead
			status.IsBehind = behind > 0
			status.HasUnpushed = ahead > 0
		}
	}

	return status
}

// FormatGitStatusIcons returns a formatted string with branch name and status icons
func (s *GitStatus) FormatGitStatusIcons() string {
	if s == nil || s.Branch == "" {
		return ""
	}

	var icons strings.Builder

	// Branch icon and name
	icons.WriteString(" ")
	icons.WriteString(s.Branch)

	// Uncommitted changes (dirty working tree)
	if s.HasUncommitted {
		icons.WriteString(" *")
	}

	// Commits ahead (unpushed)
	if s.HasUnpushed {
		icons.WriteString(" ↑")
		if s.UnpushedCount > 0 {
			icons.WriteString(strconv.Itoa(s.UnpushedCount))
		}
	}

	// Commits behind
	if s.IsBehind {
		icons.WriteString(" ↓")
		if s.BehindCount > 0 {
			icons.WriteString(strconv.Itoa(s.BehindCount))
		}
	}

	return icons.String()
}
