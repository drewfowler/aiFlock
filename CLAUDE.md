# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.
The TODO list in README.md is the place where I will put your tasks. If I mention a todo, check there. Sometimes the todo will link an .md file for better explanation.

## Project Overview

Flock is a TUI for managing multiple AI coding agents (Claude Code instances) from a single dashboard. It integrates with zellij terminal multiplexer to spawn and navigate between AI sessions, using Claude Code hooks for real-time status detection.

## Build & Run Commands

```bash
# Build the binary
go build -o flock ./cmd/flock

# Run (must be inside a zellij session)
./flock

# Run tests
go test ./...

# Run a single test
go test -run TestName ./internal/task/
```

## Architecture

### Core Components

- **cmd/flock/main.go** - Entry point; initializes components, starts status watcher, launches TUI
- **internal/tui/** - Bubble Tea TUI application (Model-View-Update pattern)
- **internal/task/** - Task model, CRUD operations, and JSON persistence to `~/.flock/tasks.json`
- **internal/status/** - File watcher monitoring `/tmp/flock/` for status updates
- **internal/zellij/** - Wrapper around `zellij action` commands for tab management

### Status Flow

Claude Code hooks (`.claude/hooks/update_status.sh`) write status files to `/tmp/flock/` when:
- `UserPromptSubmit` → WAITING (Claude needs input)
- `PreToolUse` → WORKING (Claude is executing)
- `Stop` → DONE (task complete)

The status watcher detects file changes and updates the TUI via channels.

### Task States

```
PENDING → WORKING → WAITING → WORKING → DONE
```

### Key Environment Variables

When spawning AI tabs, flock sets:
- `FLOCK_TASK_ID` - Task identifier
- `FLOCK_TAB_NAME` - Zellij tab name
- `FLOCK_STATUS_DIR` - Status file directory (`/tmp/flock`)

## Zellij Integration

Flock requires running inside a zellij session. It uses `zellij action` commands:
- `new-tab --name <name>` - Create task tabs
- `go-to-tab-name <name>` - Navigate between tabs
- `write-chars` / `write` - Send commands to panes
- `rename-tab` - Rename the controller tab to "flock"

## Tech Stack

- Go 1.24+
- [Bubble Tea](https://github.com/charmbracelet/bubbletea) - TUI framework
- [Lip Gloss](https://github.com/charmbracelet/lipgloss) - Terminal styling
- [Bubbles](https://github.com/charmbracelet/bubbles) - TUI components
- [fsnotify](https://github.com/fsnotify/fsnotify) - File system watching
