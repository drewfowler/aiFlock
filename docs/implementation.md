# Implementation Plan: flock

## Overview

Build a TUI-based controller for managing multiple AI coding agents using zellij. The controller displays task status, allows jumping to agent tabs, and detects agent state via Claude Code hooks.

## Phase 1: Project Setup & Core Infrastructure

### 1.1 Initialize Go Project
- `go mod init github.com/dfowler/flock`
- Create directory structure:
  ```
  cmd/flock/main.go
  internal/tui/
  internal/task/
  internal/status/
  internal/zellij/
  ```

### 1.2 Install Dependencies
```bash
go get github.com/charmbracelet/bubbletea
go get github.com/charmbracelet/lipgloss
go get github.com/charmbracelet/bubbles
go get github.com/fsnotify/fsnotify
```

### 1.3 Create Core Models
- **File: `internal/task/task.go`**
  - Task struct: ID, Name, Status, TabName, CreatedAt, UpdatedAt
  - Status enum: PENDING, WORKING, WAITING, DONE

## Phase 2: Task Manager

### 2.1 Task CRUD Operations
- **File: `internal/task/manager.go`**
  - Create, Read, Update, Delete tasks
  - List all tasks
  - Find task by ID

### 2.2 Task Persistence
- **File: `internal/task/store.go`**
  - JSON file storage at `~/.flock/tasks.json`
  - Load tasks on startup
  - Save on every change

## Phase 3: Status Watcher

### 3.1 File System Watcher
- **File: `internal/status/watcher.go`**
  - Watch `/tmp/flock/` directory using fsnotify
  - Parse status files on change
  - Emit status update events

### 3.2 Status File Parser
- **File: `internal/status/parser.go`**
  - Parse format: `status=WAITING\ntask_id=001\nupdated=timestamp\ntab_name=task-001`
  - Validate and return structured data

## Phase 4: Zellij Controller

### 4.1 Zellij Action Wrapper
- **File: `internal/zellij/controller.go`**
  - `NewTab(taskID, prompt, cwd string)` - Create new tab with layout
  - `GoToTab(tabName string)` - Jump to specific tab
  - `GoToController()` - Return to controller tab
  - `CloseTab(tabName string)` - Close a tab

### 4.2 Layout Template
- **File: `configs/ai-session.kdl`**
  - Update existing layout to accept environment variables
  - Pass `FLOCK_TASK_ID` and `FLOCK_TAB_NAME` to Claude session

## Phase 5: TUI Dashboard

### 5.1 Main Application
- **File: `internal/tui/app.go`**
  - Bubble Tea Model-View-Update setup
  - Subscribe to status watcher events
  - Route keyboard input

### 5.2 Dashboard View
- **File: `internal/tui/dashboard.go`**
  - Render task table with columns: #, Task, Status, Tab, Age
  - Status colors: WAITING=yellow, WORKING=blue, DONE=green, PENDING=gray
  - Highlight selected row

### 5.3 Keyboard Handling
- **File: `internal/tui/input.go`**
  - `n` - New task (opens input modal)
  - `s` - Start selected task
  - `j/k` - Navigate up/down
  - `Enter` - Jump to task tab
  - `d` - Delete task
  - `q` - Quit

### 5.4 Input Components
- **File: `internal/tui/forms.go`**
  - Task creation form (name, prompt, working directory)
  - Use bubbles text input component

## Phase 6: Claude Code Hooks

### 6.1 Hook Scripts
- **File: `.claude/hooks/update_status.sh`**
  ```bash
  #!/bin/bash
  # Read task ID from environment, write status file
  ```

### 6.2 Hook Configuration
- **File: `.claude/settings.json`** (template for users)
  - Configure UserPromptSubmit → WAITING
  - Configure PreToolUse → WORKING
  - Configure Stop → DONE

## Phase 7: Entry Point & Wiring

### 7.1 Main Entry Point
- **File: `cmd/flock/main.go`**
  - Initialize all components
  - Start status watcher in background
  - Launch TUI
  - Graceful shutdown

## Implementation Order

1. **Phase 1.1-1.3**: Project setup and models
2. **Phase 2**: Task manager (CRUD + persistence)
3. **Phase 4**: Zellij controller (can test tab creation)
4. **Phase 5.1-5.2**: Basic TUI dashboard (display tasks)
5. **Phase 5.3-5.4**: Keyboard handling and input
6. **Phase 3**: Status watcher (connect to TUI)
7. **Phase 6**: Claude Code hooks
8. **Phase 7**: Integration and main entry point

## Key Files to Create

| File | Purpose |
|------|---------|
| `cmd/flock/main.go` | Entry point |
| `internal/task/task.go` | Task model |
| `internal/task/manager.go` | Task CRUD |
| `internal/task/store.go` | JSON persistence |
| `internal/status/watcher.go` | File watching |
| `internal/status/parser.go` | Status parsing |
| `internal/zellij/controller.go` | Zellij commands |
| `internal/tui/app.go` | TUI main |
| `internal/tui/dashboard.go` | Task list view |
| `internal/tui/input.go` | Keyboard handling |
| `.claude/hooks/update_status.sh` | Hook script |

## Verification

1. **Unit tests**: Task manager CRUD, status parser
2. **Manual testing**:
   - Run `flock` in a zellij session
   - Create a task
   - Start the task (verify tab spawns)
   - Verify status updates appear in dashboard
   - Jump to tab with Enter, return with keybind
3. **End-to-end**: Full workflow from task creation to completion
