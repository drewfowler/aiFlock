# Idea for aiController

This doc is here to document a new idea that is only an idea at this point. This is for idea creation and review. 
**Bias:** I am a huge terminal fan and love not using my mouse

## Idea 
The precedent to this is that we are moving more and more to a team of AI agents running multiple different tasks or projects at once. This issue for this is that it is hard to manage multiple vscode windows or multiple terminal windows or whatever tool you use. Therefore, I am imagining a text based UI that would be a controller of multiple windows.
Currently, I use zellij and multiple clones of the same repo to manage multiple AIs running on different tasks and it does tend to work pretty well, but my idea is that there is a main interface that you can view all the currently running AIs and see what ones need attention, then pop into whatever window needs attention and pop back up to the main controller. This would be like a tech mananger controlling all his employees by going to each engineers desk, helping them and then popping back up to help the next. 
I tend to like TUIs as they are easy to develop and use but open to options here. Also, how should we manange all the different AI locations? Should we connect with something like tmux or zellij? I like zellij as they have templates that can be used for each new AI window. I think making this cool useable for everyone would be very difficult by trying to manage vscode windows or whatever editor the user desires but I want feedback here. 
Is connecting to and viewing each AI agent status even possible here? Or would we have custom instructions for them to do checkins before needing permission or task status? 

## Future
- Some sort of 'standup' would be ideal for checking in and starting multiple tasks off easier.
- Creating a task list
  - Kick off work from task list

AI Gen below this line

---

## Discussion Notes (2026-01-08)

### Decision: Zellij-First Approach
Build tightly coupled to zellij initially. This leverages:
- Zellij's layout templates (`.kdl` files) for consistent AI session setup
- `zellij action` commands for programmatic control
- Tab naming for task identification

Can abstract later for tmux/other multiplexers if needed.

### Status Detection Strategy
Use **Claude Code hooks** as primary mechanism:
- Hooks can write status to files on prompt/response events
- Controller watches status directory for changes
- Fallback: Custom instructions asking AI to report status (less reliable)

### Technical Hurdles Identified
1. **Agent status detection** - Solved via hooks + file watching
2. **Zellij integration** - Use `zellij action` CLI commands
3. **"Pop in/out" UX** - Tab switching via `zellij action go-to-tab-name`

---

## Architecture Overview

### Block Diagram

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                              USER INTERACTION                                │
└─────────────────────────────────────────────────────────────────────────────┘
                                      │
                                      ▼
┌─────────────────────────────────────────────────────────────────────────────┐
│                           aiController TUI                                   │
│  ┌───────────────────────────────────────────────────────────────────────┐  │
│  │                         Task Dashboard                                 │  │
│  │  ┌─────────────────────────────────────────────────────────────────┐  │  │
│  │  │  #  │ Task                      │ Status   │ Tab        │ Age   │  │  │
│  │  ├─────┼───────────────────────────┼──────────┼────────────┼───────┤  │  │
│  │  │  1  │ Fix auth bug in login.ts  │ WAITING  │ task-001   │ 12m   │  │  │
│  │  │  2  │ Add unit tests for API    │ WORKING  │ task-002   │ 8m    │  │  │
│  │  │  3  │ Refactor database layer   │ DONE     │ task-003   │ 45m   │  │  │
│  │  │  4  │ Update README docs        │ PENDING  │ --         │ --    │  │  │
│  │  └─────────────────────────────────────────────────────────────────┘  │  │
│  │                                                                        │  │
│  │  [n]ew task  [s]tart  [j/k]navigate  [enter]jump  [d]elete  [q]uit    │  │
│  └───────────────────────────────────────────────────────────────────────┘  │
│                                                                              │
│  ┌─────────────────┐  ┌─────────────────┐  ┌─────────────────────────────┐  │
│  │  Task Manager   │  │  Status Watcher │  │  Zellij Controller          │  │
│  │  - CRUD tasks   │  │  - Watch files  │  │  - new-tab --layout        │  │
│  │  - Persist JSON │  │  - Parse status │  │  - go-to-tab-name          │  │
│  │  - Queue work   │  │  - Update UI    │  │  - write (send commands)   │  │
│  └────────┬────────┘  └────────┬────────┘  └─────────────┬───────────────┘  │
└───────────┼────────────────────┼─────────────────────────┼──────────────────┘
            │                    │                         │
            ▼                    ▼                         ▼
┌───────────────────┐  ┌─────────────────────┐  ┌─────────────────────────────┐
│  ~/.aicontroller/ │  │ /tmp/aicontroller/  │  │        Zellij               │
│  ├── tasks.json   │  │ ├── task-001.status │  │  ┌───────────────────────┐  │
│  ├── config.toml  │  │ ├── task-002.status │  │  │ Tab: controller       │  │
│  └── history.log  │  │ └── task-003.status │  │  │ (aiController TUI)    │  │
└───────────────────┘  └──────────┬──────────┘  │  ├───────────────────────┤  │
                                  │             │  │ Tab: task-001         │  │
                                  │             │  │ ┌───────────────────┐ │  │
                                  │             │  │ │ $ claude "Fix..." │ │  │
                                  │             │  │ │ > Waiting for     │ │  │
                                  │             │  │ │   user input...   │ │  │
                    ┌─────────────┘             │  │ └───────────────────┘ │  │
                    │                           │  ├───────────────────────┤  │
                    ▼                           │  │ Tab: task-002         │  │
┌─────────────────────────────────────┐        │  │ ┌───────────────────┐ │  │
│  Claude Code (in each AI tab)       │        │  │ │ $ claude "Add..." │ │  │
│  ┌───────────────────────────────┐  │        │  │ │ > Writing tests   │ │  │
│  │  Hooks (hooks.json)           │  │        │  │ │   for endpoint... │ │  │
│  │  ├── on_prompt_start ─────────┼──┼───┐    │  │ └───────────────────┘ │  │
│  │  │    → write WAITING         │  │   │    │  └───────────────────────┘  │
│  │  ├── on_response_start ───────┼──┼───┤    └─────────────────────────────┘
│  │  │    → write WORKING         │  │   │
│  │  └── on_stop ─────────────────┼──┼───┤
│  │       → write DONE            │  │   │
│  └───────────────────────────────┘  │   │
└─────────────────────────────────────┘   │
                                          │
                    ┌─────────────────────┘
                    │  Status file writes
                    ▼
        ┌───────────────────────┐
        │  Status File Format   │
        │  /tmp/aicontroller/   │
        │  task-XXX.status      │
        │  ┌─────────────────┐  │
        │  │ status: WAITING │  │
        │  │ updated: <ts>   │  │
        │  │ task_id: 001    │  │
        │  └─────────────────┘  │
        └───────────────────────┘
```

### Workflow Sequence

```
┌──────────┐     ┌─────────────┐     ┌────────┐     ┌─────────────┐
│   User   │     │ aiController│     │ Zellij │     │ Claude Code │
└────┬─────┘     └──────┬──────┘     └───┬────┘     └──────┬──────┘
     │                  │                │                 │
     │  1. Create task  │                │                 │
     │─────────────────>│                │                 │
     │                  │                │                 │
     │  2. Start task   │                │                 │
     │─────────────────>│                │                 │
     │                  │  3. new-tab    │                 │
     │                  │  --layout      │                 │
     │                  │───────────────>│                 │
     │                  │                │  4. Spawn tab   │
     │                  │                │────────────────>│
     │                  │                │                 │
     │                  │                │  5. Run claude  │
     │                  │                │  with task      │
     │                  │                │────────────────>│
     │                  │                │                 │
     │                  │  6. Status: WORKING              │
     │                  │<─────────────────────────────────│
     │                  │                │                 │
     │  7. UI updates   │                │                 │
     │<─────────────────│                │                 │
     │                  │                │                 │
     │                  │  8. Status: WAITING              │
     │                  │<─────────────────────────────────│
     │                  │                │                 │
     │  9. "Needs       │                │                 │
     │     attention"   │                │                 │
     │<─────────────────│                │                 │
     │                  │                │                 │
     │  10. Jump to tab │                │                 │
     │─────────────────>│                │                 │
     │                  │  11. go-to-tab │                 │
     │                  │───────────────>│                 │
     │                  │                │                 │
     │  12. User now in Claude session   │                 │
     │<──────────────────────────────────│                 │
     │                  │                │                 │
     │  13. Provide input               │                 │
     │─────────────────────────────────────────────────────>
     │                  │                │                 │
     │                  │  14. Status: WORKING             │
     │                  │<─────────────────────────────────│
     │                  │                │                 │
     │  15. Return to   │                │                 │
     │      controller  │                │                 │
     │─────────────────>│                │                 │
     │                  │  16. go-to-tab │                 │
     │                  │  "controller"  │                 │
     │                  │───────────────>│                 │
     │                  │                │                 │
```

### Component Responsibilities

| Component | Responsibility |
|-----------|---------------|
| **TUI (main)** | Render dashboard, handle keyboard input, orchestrate other components |
| **Task Manager** | CRUD operations on tasks, persist to `tasks.json`, manage task lifecycle |
| **Status Watcher** | Watch `/tmp/aicontroller/` directory, parse status files, emit events |
| **Zellij Controller** | Wrapper around `zellij action` commands, manage tab lifecycle |
| **Claude Code Hooks** | Report AI state changes to status files |

### Tech Stack

**TUI Framework: [Charm](https://charm.sh/) libraries**
- **[Bubble Tea](https://github.com/charmbracelet/bubbletea)** - Elm-inspired TUI framework for Go
  - Model-View-Update architecture
  - Built-in support for commands and subscriptions (good for file watching)
  - Clean keyboard/mouse input handling
- **[Lip Gloss](https://github.com/charmbracelet/lipgloss)** - Declarative styling for terminal UIs
  - CSS-like styling API
  - Color profiles and adaptive colors
  - Borders, padding, alignment
- **[Bubbles](https://github.com/charmbracelet/bubbles)** - Pre-built components (optional, useful)
  - Table, list, text input, spinner, viewport components
  - Can accelerate MVP development

**Other dependencies:**
- `fsnotify` - File system watching for status files
- `os/exec` - Zellij command execution

### File Structure (Proposed)

```
aiController/
├── cmd/
│   └── aicontroller/
│       └── main.go
├── internal/
│   ├── tui/
│   │   ├── app.go           # Main TUI application
│   │   ├── dashboard.go     # Task list view
│   │   └── input.go         # Keyboard handling
│   ├── task/
│   │   ├── manager.go       # Task CRUD operations
│   │   ├── task.go          # Task model
│   │   └── store.go         # JSON persistence
│   ├── status/
│   │   ├── watcher.go       # File system watcher
│   │   └── parser.go        # Status file parsing
│   └── zellij/
│       ├── controller.go    # Zellij action wrapper
│       └── layout.go        # Layout template handling
├── configs/
│   └── ai-session.kdl       # Zellij layout template
├── docs/
│   └── idea.md
├── go.mod
└── go.sum
```

### Status File Format

```
# /tmp/aicontroller/task-001.status
status=WAITING
task_id=001
updated=1704672000
tab_name=task-001
```

### Task States

```
PENDING  ──▶  WORKING  ──▶  WAITING  ──▶  WORKING  ──▶  DONE
   │              │            │              │           │
   │              │            │              │           │
   └── Task       └── Claude   └── Claude     └── User    └── Task
       created        starts       needs          gave        complete
                      work         input          input
```

### Zellij Layout Template (ai-session.kdl)

```kdl
layout {
    tab name="task-${TASK_ID}" {
        pane {
            command "claude"
            args "${TASK_PROMPT}"
            cwd "${TASK_CWD}"
        }
    }
}
```

---

## MVP Scope

### Phase 1: Core Functionality
- [ ] Basic TUI with task list display
- [ ] Task creation (manual entry)
- [ ] Zellij tab spawning with layout
- [ ] Status file watching
- [ ] Jump to tab functionality

### Phase 2: Polish
- [ ] Task persistence across restarts
- [ ] Better status indicators (colors, icons)
- [ ] Task history/logging
- [ ] Configuration file support

### Phase 3: Advanced (Future)
- [ ] Standup mode (batch task review)
- [ ] Task templates
- [ ] Multiple project support
- [ ] tmux adapter (optional)

