# Implementation Plan: Git Worktree Pool

## Problem

When multiple AI agents work on the same repository, they all write to the same branch, causing conflicts. Each agent needs isolated working directories with separate branches.

## Solution

A **stateless lazy +1 strategy** using git as the source of truth. No persistent pool state needed—just query `git worktree list` on demand and keep one worktree ahead.

## How It Works

```
Task created → Check git worktree list → Find or create free worktree → Assign → Background: create +1

Example flow:
  Task 1 created (no free worktrees) → Create worktree (user waits) → Assign → Background: create +1
  Task 2 created (1 free worktree)   → Instant assign → Background: create +1
  Task 3 created (1 free worktree)   → Instant assign → Background: create +1
```

### Multi-Repository Support

Flock manages tasks across multiple projects. Each repository gets its own worktrees, discovered independently via `git worktree list`. No central pool state needed.

```
Task A: Cwd=/home/user/project-alpha → worktrees in project-alpha/.flock-worktrees/
Task B: Cwd=/home/user/project-beta  → worktrees in project-beta/.flock-worktrees/
Task C: Cwd=/home/user/project-alpha → reuses free worktree from project-alpha
```

### Directory Structure

Worktrees live inside each repository:

```
/home/user/project-alpha/
  ├── .git/
  ├── src/
  └── .flock-worktrees/
      ├── flock-001/    # Assigned to task-001
      ├── flock-002/    # Assigned to task-002
      └── flock-003/    # Free (ready for next task)

/home/user/project-beta/
  ├── .git/
  └── .flock-worktrees/
      └── flock-004/    # Assigned to task-004
```

## Key Behaviors

### On Task Creation

1. Get repo root from task's `Cwd` using `git rev-parse --show-toplevel`
2. If not a git repo, skip worktree assignment (task uses original `Cwd`)
3. Run `git worktree list` to find existing `flock-*` worktrees
4. Cross-reference with active tasks to find free worktrees
5. If free worktree exists:
   - Assign it to the task
   - Create task branch: `flock-{taskID}`
   - Check if only 1 free remains → background create +1
6. If no free worktree:
   - Create one synchronously (user waits)
   - Assign to task
   - Background create +1

### On Task Deletion

1. If task has assigned worktree:
   - Run `git worktree remove <path>`
   - Branch cleanup: `git branch -D flock-{taskID}`

### On Task Completion (DONE status)

- Worktree stays assigned (user may want to review/push)
- User explicitly deletes task to free worktree

## Finding Free Worktrees

No pool state needed. Use naming convention + active tasks:

```go
func FindFreeWorktree(repoRoot string, activeTasks []*Task) (string, error) {
    // Get all worktrees from git
    worktrees, err := ListWorktrees(repoRoot)
    if err != nil {
        return "", err
    }

    // Build set of assigned paths
    assignedPaths := make(map[string]bool)
    for _, t := range activeTasks {
        if t.WorktreePath != "" {
            assignedPaths[t.WorktreePath] = true
        }
    }

    // Find free flock worktree
    for _, wt := range worktrees {
        if strings.Contains(wt.Path, ".flock-worktrees/flock-") && !assignedPaths[wt.Path] {
            return wt.Path, nil
        }
    }

    return "", nil // None free
}
```

## Configuration

Add to `~/.flock/config.json`:

```json
{
  "worktrees": {
    "enabled": true,
    "max_per_repo": 10
  }
}
```

| Option | Default | Description |
|--------|---------|-------------|
| `enabled` | `true` | Enable worktree isolation (auto-disabled if not in git repo) |
| `max_per_repo` | `10` | Maximum worktrees per repository to prevent disk bloat |

## Progress Indicator

When a user must wait for worktree creation (first task in a repo, or rapid task creation), show a spinner in the TUI.

### UI Integration

```
# │ Task                │ Status  │ Branch           │ Age
──┼─────────────────────┼─────────┼──────────────────┼─────
1 │ Add auth module     │ WORKING │ flock-001        │ 5m
2 │ Fix login bug       │ PENDING │ flock-002        │ 2m
3 │ Refactor API        │ PENDING │ ⣾ creating...    │ 10s
```

## Implementation

### New Files

| File | Purpose |
|------|---------|
| `internal/git/worktree.go` | Worktree operations (create, list, remove) |
| `internal/git/repo.go` | Repository detection and utilities |

### Modified Files

| File | Changes |
|------|---------|
| `internal/task/task.go` | Add `WorktreePath`, `GitBranch` fields |
| `internal/task/manager.go` | Integrate worktree assignment on create/delete |
| `internal/tui/dashboard.go` | Show branch column, spinner for creating |
| `internal/config/config.go` | Add worktree config |

### Data Model Changes

**Task struct additions** (`internal/task/task.go`):

```go
type Task struct {
    // ... existing fields
    WorktreePath string `json:"worktree_path,omitempty"` // Absolute path to worktree
    GitBranch    string `json:"git_branch,omitempty"`    // Branch name (e.g., flock-001)
}
```

No separate pool state file. Git is the source of truth.

## Implementation Phases

### Phase 1: Core Git Operations

Create `internal/git/worktree.go`:
- `IsGitRepo(path string) bool`
- `GetRepoRoot(path string) (string, error)`
- `ListWorktrees(repoRoot string) ([]Worktree, error)`
- `CreateWorktree(repoRoot, worktreePath, branch string) error`
- `RemoveWorktree(repoRoot, worktreePath string) error`

### Phase 2: Worktree Assignment

Create `internal/git/assign.go`:
- `FindFreeWorktree(repoRoot string, activeTasks []*Task) (string, error)`
- `AssignWorktree(task *Task, activeTasks []*Task) error`
- `ReleaseWorktree(task *Task) error`
- `EnsurePlusOne(repoRoot string, activeTasks []*Task)` - Background goroutine

### Phase 3: Task Integration

Modify `internal/task/manager.go`:
- On `CreateTask()`: Call `AssignWorktree()`, set task's `Cwd` to worktree path
- On `DeleteTask()`: Call `ReleaseWorktree()`
- Pass active tasks list to worktree functions

### Phase 4: TUI Integration

Modify `internal/tui/dashboard.go`:
- Show branch name column
- Show spinner when worktree is being created
- Handle "creating worktree" state in task display

### Phase 5: Configuration

Modify `internal/config/config.go`:
- Add `Worktrees` config struct
- Add `enabled` and `max_per_repo` options
- Handle "not a git repo" gracefully (disable feature)

## Edge Cases

| Scenario | Handling |
|----------|----------|
| Not in a git repo | Disable worktree features, tasks use `Cwd` as-is |
| First task in repo | User waits for initial worktree creation |
| Rapid task creation | Queue waits, +1 strategy minimizes wait after first |
| Worktree deleted externally | `git worktree list` won't show it, create new one |
| Disk full | Error gracefully, show message in TUI |
| Max worktrees reached | Warn user, block new tasks until one is freed |
| Git operations fail | Show error in TUI, task stays in pending state |
| Task Cwd is already a worktree | Detect and use as-is, don't create nested worktree |

## Testing

### Unit Tests
- `IsGitRepo()` with git and non-git directories
- `FindFreeWorktree()` with various task states
- Worktree naming and path generation

### Integration Tests
- Task create → worktree assign → task delete → worktree removed
- Multiple tasks in same repo share worktree pool
- Multiple repos maintain separate worktrees
- +1 background creation triggers correctly

### Manual Testing
1. Start flock, create task in a git repo
2. Verify worktree created in `.flock-worktrees/`
3. Create second task, verify instant assignment (no wait)
4. Delete task, verify worktree removed
5. Test in non-git directory, verify graceful fallback
6. Test with multiple projects open
