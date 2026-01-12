# Flock

A TUI for managing multiple AI coding agents from one dashboard.

## What It Does

Monitor all your AI coding sessions at a glance, see which ones need attention, jump in to help, and pop back to the overview. Like a tech manager visiting each engineer's desk.

## How It Works

1. **Create tasks** in the dashboard with a name, working directory, and prompt
2. **Start tasks** → spawns zellij tabs running Claude Code
3. **Status updates** via Claude Code hooks (PENDING/WORKING/WAITING/DONE)
4. **Jump to** any session needing attention, then return to dashboard

## Requirements

- Go 1.24+
- Zellij (must be running inside a zellij session)
- Claude Code (hooks installed on first run)
- Optional: `fzf` and `fd` for directory picker

## Installation

```bash
go build -o flock ./cmd/flock
./flock  # Must run inside a zellij session
```

## Features

### Dashboard Layout

Three-panel interface:
- **Task Panel** (left) - Task list with ID, name, status, branch, git status, directory, and age
- **Prompt Panel** (right) - Markdown preview of the selected task's prompt
- **Status Panel** (bottom) - Recent notifications and system messages

### Task Management

- Create tasks with name, working directory, and markdown prompt
- Edit pending tasks (name, directory, prompt)
- Delete tasks with optional confirmation
- Start tasks to spawn Claude agents
- Jump to active task tabs with Enter

### Git Integration

- **Branch display** - Shows current branch for each task
- **Ahead/behind indicators** - Green `+N` for commits ahead, red `-N` for commits behind
- **Worktree support** - Automatic worktree creation for isolated branches
- **Branch merging** - Merge task branches into main with diff preview

### Status Tracking

Real-time status updates via Claude Code hooks:
- **PENDING** - Task created, not started
- **WORKING** - Claude is executing (animated spinner)
- **WAITING** - Claude needs input
- **DONE** - Task complete

### Desktop Notifications

System notifications when task status changes (toggle in settings).

### Prompt Templates

- Default template with Goal/Context/Constraints sections
- Project-specific templates in `.claude/flock/templates/default.md`
- Variable substitution: `{{name}}`, `{{working_dir}}`

## Keybindings

### Dashboard

| Key | Action |
|-----|--------|
| `n` | New task |
| `e` | Edit task (pending only) |
| `s` | Start task |
| `m` | Merge branch into main |
| `d` | Delete task |
| `S` | Open settings |
| `j`/`k` | Navigate up/down |
| `Enter` | Jump to task tab |
| `q` | Quit |

### New/Edit Task Form

| Key | Action |
|-----|--------|
| `Tab`/`Shift+Tab` | Cycle fields |
| `Ctrl+f` | Open directory picker (fzf) |
| `Ctrl+w` | Toggle worktree option |
| `Ctrl+e` | Force open editor |
| `Enter` | Create/update task |
| `Esc` | Cancel |

### Settings

| Key | Action |
|-----|--------|
| `j`/`k` | Navigate settings |
| `Enter`/`Space` | Toggle setting |
| `Esc`/`S` | Close settings |

## Settings

Press `S` to open settings:

1. **Notifications** - Desktop notifications on status change
2. **Auto-start tasks** - Start tasks immediately after creation
3. **Confirm before delete** - Show confirmation dialog
4. **Use worktree** - Default worktree toggle for new tasks
5. **Worktree cleanup** - Ask/Delete/Keep when deleting tasks

## Directory Structure

```
~/.flock/
├── config.json      # Settings
├── tasks.json       # Task data
├── prompts/         # Task prompt files
└── hooks/           # Claude Code hooks

.flock-worktrees/    # Per-repo worktree storage (in repo root)

.claude/flock/templates/  # Project-specific prompt templates
```

## Environment Variables

Set by flock when spawning agents:
- `FLOCK_TASK_ID` - Task identifier
- `FLOCK_TASK_NAME` - Task name
- `FLOCK_TAB_NAME` - Zellij tab name
- `FLOCK_STATUS_DIR` - Status file directory

## Status Hook

On first run, flock installs a Claude Code hook at `~/.claude/hooks/update_status.sh`. This hook writes status updates to `/tmp/flock/` only when `FLOCK_TASK_ID` is set, so it doesn't interfere with regular Claude usage.
