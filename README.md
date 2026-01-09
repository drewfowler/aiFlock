# aiFlock

A TUI for managing multiple AI coding agents from one dashboard.

## What It Does

Monitor all your AI coding sessions at a glance, see which ones need attention, jump in to help, and pop back to the overview. Like a tech manager visiting each engineer's desk.

## How It Works

1. **Create tasks** in the dashboard
2. **Start tasks** → spawns zellij tabs running Claude Code
3. **Status updates** via Claude Code hooks (WAITING/WORKING/DONE)
4. **Jump to** any session needing attention, then return to dashboard

## Requirements

- Go 1.24+
- Zellij (must be running inside a zellij session)
- Claude Code (with hooks configured)

## TODO

- [x] alt +f opens a floating window in zellij (changed to Ctrl+g)
- [x] still have the missing status_id error (moved hooks to ~/.claude/settings.json user config, added FLOCK_PROJECT_DIR env var)
- [ ] Move tasks to "agents" section and create separate "tasks" section for .md-based task files (enables easier editing and task queuing)

- [x] still have the missing status_id error (added Notification hook to settings.json, added better validation in hook script)
- [x] the permission notification and change to status not working (added Notification hook config to .claude/settings.json)
- [x] stale task notification on startup with 0 tasks (skip notifications during initial load, cleanup stale status files on startup)
- [x] permission requests are still not working (fixed: use `export` for env vars to propagate to hooks)
- [x] when starting an agent, don't immediately open that tab (now returns to flock tab)
- [x] the agents zellij tabs should be agent-XXX-taskName (e.g., agent-001-changingReadMe)
- [x] when I start flock I have an error "Error: task 004 not found" (silently ignores stale status files, cleans up on delete)
- [x] put all this UI into a bordered box and center in terminal

## Future
