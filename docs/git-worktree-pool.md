# Implementation Plan: Git Worktree Pool

## Problem

When multiple AI agents work on the same repository, they all write to the same branch, causing conflicts. Each agent needs isolated working directories with separate branches.

## Solution

A **self-managing worktree pool** that stays one ahead of demand. Users never wait because there's always a pre-warmed worktree ready.

## How It Works

```
Flock starts → Checks existing worktrees → Ensures pool has 1 free

User creates task → Assigns free worktree → Starts creating next one in background

Pool state example:
  [pool-001: task-001] [pool-002: task-002] [pool-003: ready] [pool-004: creating...]
                                                  ↑                    ↑
                                             next task            being prepared
```

### Directory Structure

```
~/.flock/
  ├── tasks.json
  ├── pool.json           # Pool state persistence
  └── worktrees/
      ├── pool-001/       # Assigned to task-001
      ├── pool-002/       # Assigned to task-002
      ├── pool-003/       # Free (ready)
      └── pool-004/       # Creating in background
```

### Pool States

Each worktree in the pool has a state:

| State | Description |
|-------|-------------|
| `creating` | Being created in background |
| `ready` | Available for assignment |
| `assigned` | In use by a task |
| `recycling` | Being reset for reuse |

## Key Behaviors

### On Flock Startup
1. Load pool state from `~/.flock/pool.json`
2. Verify worktree directories still exist
3. Count free worktrees
4. If free < `min_free_worktrees`, start creating more in background
5. If not in a git repo, disable worktree features

### On Task Creation
1. Check for a `ready` worktree
2. If available:
   - Assign it to the task (state → `assigned`)
   - Create new branch: `task-{taskID}-{sanitized-name}`
   - Update task's `Cwd` to worktree path
   - Start creating next worktree in background
3. If not available:
   - Show progress bar while worktree is created
   - Block task start until ready

### On Task Deletion
1. If task has assigned worktree:
   - Reset worktree: `git checkout main && git clean -fd && git branch -D task-branch`
   - Return to pool (state → `ready`)
   - Or delete if pool > `max_worktrees`

### On Task Completion (DONE status)
- Worktree stays assigned (user may want to review/push)
- User explicitly deletes task to free worktree

## Configuration

Add to `~/.flock/config.json`:

```json
{
  "worktree_pool": {
    "enabled": true,
    "min_free": 1,
    "max_total": 10,
    "base_dir": "~/.flock/worktrees",
    "recycle": true
  }
}
```

| Option | Default | Description |
|--------|---------|-------------|
| `enabled` | `true` | Enable worktree pool (auto-disabled if not in git repo) |
| `min_free` | `1` | Always keep this many worktrees ready |
| `max_total` | `10` | Maximum worktrees to prevent disk bloat |
| `base_dir` | `~/.flock/worktrees` | Where to create worktrees |
| `recycle` | `true` | Reset and reuse vs delete on task completion |

## Progress Bar for Slow Creation

When a user creates tasks faster than worktrees can be prepared, show a progress bar using the bubbles `progress` component.

### UI Integration

In the task list, add a progress column:

```
# │ Task                │ Status  │ Worktree     │ Age
──┼─────────────────────┼─────────┼──────────────┼─────
1 │ Add auth module     │ WORKING │ ready        │ 5m
2 │ Fix login bug       │ PENDING │ ready        │ 2m
3 │ Refactor API        │ PENDING │ ████░░░░ 45% │ 10s
```

### Progress Tracking

Git worktree creation has two phases:
1. `git worktree add` - Creates the worktree entry (fast)
2. File checkout - Writes files to disk (slow for large repos)

Track progress by:
- Counting files being written (if possible via git)
- Or using a time-based estimate from previous creations
- Or showing indeterminate progress spinner

### Bubbles Progress Component

```go
import "github.com/charmbracelet/bubbles/progress"

type WorktreeProgress struct {
    worktreeID string
    progress   progress.Model
    percent    float64
}

// In TUI model
type Model struct {
    // ... existing fields
    worktreeProgress map[string]*WorktreeProgress
}
```

### Progress Updates

The git pool manager sends progress updates via channel:

```go
type ProgressUpdate struct {
    WorktreeID string
    Percent    float64  // 0.0 to 1.0
    Done       bool
    Error      error
}
```

TUI subscribes to these updates and renders progress bars for any worktree in `creating` state.

## Implementation

### New Files

| File | Purpose |
|------|---------|
| `internal/git/pool.go` | Worktree pool manager |
| `internal/git/worktree.go` | Individual worktree operations |
| `internal/git/progress.go` | Progress tracking for checkout |

### Modified Files

| File | Changes |
|------|---------|
| `internal/task/task.go` | Add `WorktreeID`, `GitBranch` fields |
| `internal/task/manager.go` | Integrate pool assignment on create/delete |
| `internal/tui/app.go` | Add progress bar rendering, pool status |
| `internal/tui/dashboard.go` | Show worktree column with progress |
| `internal/config/config.go` | Add worktree pool config |

### Data Model Changes

**Task struct additions** (`internal/task/task.go`):

```go
type Task struct {
    // ... existing fields
    WorktreeID string `json:"worktree_id,omitempty"` // Pool worktree ID
    GitBranch  string `json:"git_branch,omitempty"`  // Branch name in worktree
}
```

**Pool state** (`~/.flock/pool.json`):

```json
{
  "repo_path": "/home/user/projects/myrepo",
  "worktrees": [
    {
      "id": "pool-001",
      "path": "/home/user/.flock/worktrees/pool-001",
      "state": "assigned",
      "task_id": "001",
      "branch": "task-001-add-auth",
      "created_at": "2024-01-15T10:00:00Z"
    },
    {
      "id": "pool-002",
      "path": "/home/user/.flock/worktrees/pool-002",
      "state": "ready",
      "task_id": "",
      "branch": "",
      "created_at": "2024-01-15T10:05:00Z"
    }
  ]
}
```

## Implementation Phases

### Phase 1: Core Git Operations
1. Create `internal/git/worktree.go`
   - `CreateWorktree(basePath, worktreeID string) error`
   - `DeleteWorktree(path string) error`
   - `ResetWorktree(path string) error`
   - `IsGitRepo(path string) bool`
   - `GetRepoRoot(path string) (string, error)`

### Phase 2: Pool Manager
1. Create `internal/git/pool.go`
   - Pool struct with state management
   - `NewPool(config PoolConfig) *Pool`
   - `Initialize() error` - Load state, verify worktrees
   - `GetFreeWorktree() (*Worktree, error)`
   - `AssignWorktree(worktreeID, taskID, branchName string) error`
   - `ReleaseWorktree(worktreeID string) error`
   - `EnsureMinFree() error` - Background creation trigger

### Phase 3: Background Creation
1. Create `internal/git/progress.go`
   - Progress channel and update types
   - File counting or time estimation
2. Add goroutine in pool manager for async creation
3. Wire progress updates to TUI

### Phase 4: TUI Integration
1. Add progress bar to dashboard view
2. Subscribe to pool progress channel
3. Show worktree status column
4. Handle "no free worktree" state gracefully

### Phase 5: Task Integration
1. Modify task creation to request worktree from pool
2. Modify task deletion to release worktree
3. Update task model with worktree fields
4. Persist worktree assignment in tasks.json

### Phase 6: Configuration
1. Add pool config to config system
2. Add CLI flags for worktree settings
3. Handle "not a git repo" gracefully

## Edge Cases

| Scenario | Handling |
|----------|----------|
| Not in a git repo | Disable worktree features, tasks use `Cwd` as-is |
| First run | Show one-time progress bar while creating initial worktree |
| Rapid task creation | Queue requests, show progress bars, warn if pool exhausted |
| Worktree deleted externally | Detect on startup, remove from pool state |
| Disk full | Error gracefully, suggest cleanup |
| Max worktrees reached | Warn user, block new tasks until one is freed |
| Git operations fail | Show error in TUI, allow retry |

## Testing

### Unit Tests
- Pool state management
- Worktree creation/deletion
- Progress calculation
- Config parsing

### Integration Tests
- Full pool lifecycle
- Task create → worktree assign → task delete → worktree recycle
- Multiple concurrent task creations

### Manual Testing
1. Start flock in a git repo
2. Create task, verify worktree assigned
3. Create multiple tasks rapidly, verify progress bars appear
4. Delete task, verify worktree recycled
5. Test in non-git directory, verify graceful fallback
