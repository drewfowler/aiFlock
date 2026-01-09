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

- [x] why am I getting the missing task_id in status file error?
- [x] create another bordered section for status's. We can put errors such as the task_id print

## Future
- [ ] Move tasks to "agents" section and create separate "tasks" section for .md-based task files (enables easier editing and task queuing)
