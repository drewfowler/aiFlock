# Remote/Mobile Access for Flock

This document outlines the architecture analysis and implementation plan for adding remote access capabilities to Flock, enabling interaction from mobile devices or web browsers.

## Current Architecture Limitations

Flock is currently a **terminal-only TUI** with these hard dependencies:

| Dependency | Description |
|------------|-------------|
| Zellij | Must run inside a Zellij terminal session |
| Local filesystem | Status files in `/tmp/flock/`, tasks in `~/.flock/tasks.json` |
| Direct process spawning | `exec.Command()` calls to Zellij, Claude, editors |
| No network layer | Zero HTTP/WebSocket/API infrastructure |
| Desktop notifications | Linux-specific `notify-send` |

## Architecture Strengths

The codebase is well-structured for adding remote access:

- **Clean separation**: Task management, status watching, and UI are modular
- **Task Manager** (`internal/task/manager.go`) already has all CRUD operations
- **Status updates** are channel-based internally (easy to convert to WebSocket)
- **Configuration** is JSON-based (direct API mapping)

## API Design

### Task Endpoints

```
POST   /api/tasks              - Create task
GET    /api/tasks              - List all tasks
GET    /api/tasks/{id}         - Get task details
PUT    /api/tasks/{id}         - Update task
DELETE /api/tasks/{id}         - Delete task
GET    /api/tasks/{id}/prompt  - Get prompt content
PUT    /api/tasks/{id}/prompt  - Update prompt content
```

### Control Endpoints

```
POST /api/tasks/{id}/start    - Start task (spawn Claude session)
POST /api/tasks/{id}/navigate - Jump to task tab (local TUI only)
POST /api/tasks/{id}/input    - Send input to waiting task
```

### Status Endpoints

```
GET /api/tasks/{id}/status    - Current status
WS  /ws/status                - Real-time status stream (WebSocket)
```

### Git/Worktree Endpoints

```
GET  /api/tasks/{id}/worktree - Worktree info
POST /api/tasks/{id}/merge    - Merge branch to main
GET  /api/tasks/{id}/diff     - View uncommitted changes
```

### Configuration Endpoints

```
GET  /api/config              - Get settings
PUT  /api/config              - Update settings
GET  /api/stats               - Overall statistics
```

## Implementation Options

### Option A: REST API + WebSocket (Recommended Start)

Add an HTTP server alongside the existing TUI:

```
┌─────────────────┐         ┌──────────────────┐
│  Mobile/Web UI  │◄───────►│   API Server     │
│   (React/Vue)   │  HTTP   │  (Go/Gin/Echo)   │
└─────────────────┘  WSS    └──────────────────┘
                                      │
                                      ▼
                            ┌──────────────────┐
                            │  Flock Core      │
                            │  (Task Manager,  │
                            │   Status Watcher)│
                            └──────────────────┘
                                      │
                                      ▼
                            ┌──────────────────┐
                            │  Zellij/Claude   │
                            │  (Local Backend) │
                            └──────────────────┘
```

**Pros:**
- Minimal code changes to core logic
- Reuse existing task manager, store, config
- Web UI can be simple React/Vue app

**Cons:**
- Terminal interaction still requires complex proxying
- Still tied to local Zellij instance

**Effort:** 1-2 weeks for basic monitoring, +1 week for task creation

### Option B: Headless Backend + Web Terminal (Full Remote)

Replace Zellij with web-based terminal emulation:

```
┌─────────────────┐         ┌──────────────────┐
│  Mobile/Web UI  │◄───────►│   API Gateway    │
│  + xterm.js     │  HTTP   │   (Go)           │
└─────────────────┘  WSS    └──────────────────┘
                                      │
                    ┌─────────────────┼─────────────────┐
                    ▼                 ▼                 ▼
            ┌───────────┐     ┌───────────┐   ┌───────────┐
            │ Task API  │     │ Status    │   │ Terminal  │
            │ Service   │     │ Service   │   │ Proxy     │
            └───────────┘     └───────────┘   └───────────┘
                    │                 │                 │
                    └─────────────────┼─────────────────┘
                                      ▼
                            ┌──────────────────┐
                            │  Session Manager │
                            │  (PTY per task)  │
                            └──────────────────┘
                                      │
                                      ▼
                            ┌──────────────────┐
                            │  Claude Instances│
                            │  (tmux/screen)   │
                            └──────────────────┘
```

**Pros:**
- Full remote access from anywhere
- No local Zellij requirement
- Can run on cloud server
- Better scalability

**Cons:**
- Significant refactoring required
- Need to replace Zellij with alternative session manager
- Terminal emulation complexity

**Effort:** 4-6 weeks

## Difficulty Assessment

| Component | Difficulty | Notes |
|-----------|------------|-------|
| Task CRUD API | Easy | Manager already has clean interfaces in `internal/task/manager.go` |
| Status streaming | Easy | Already channel-based, convert to WebSocket |
| Configuration API | Easy | JSON-based, direct mapping |
| Statistics API | Easy | Manager has `Count()`, `ActiveCount()`, `WaitingCount()` |
| Prompt editing | Medium | File-based, convert editor to web textarea |
| Authentication | Medium | Currently none exists, add JWT or session-based |
| File browser | Medium | Replace fzf with web file picker |
| Terminal emulation | Hard | PTY proxying, xterm.js integration |
| Zellij abstraction | Hard | Tightly coupled, needs interface extraction |
| Multi-user support | Hard | Conflict resolution, permissions |

## Key Files for Implementation

### Direct API Mapping (Easy)

These files already have the interfaces needed for API exposure:

- `internal/task/manager.go` - All CRUD operations ready
- `internal/task/task.go` - Task model (JSON-serializable)
- `internal/task/store.go` - Persistence layer
- `internal/config/config.go` - Config management

### Status System

- `internal/status/watcher.go` - File watcher (convert events to WebSocket)
- `internal/status/parser.go` - Status parsing

### Requires Abstraction

- `internal/zellij/controller.go` - Tab management needs interface extraction

## Recommended Implementation Phases

### Phase 1: Read-Only API (1 week)

1. Add HTTP server (Gin or Echo framework)
2. Expose `/api/tasks` endpoint
3. Add WebSocket `/ws/status` for real-time updates
4. Basic web UI showing task list and status

**Deliverables:**
- Monitor all tasks from phone
- See real-time status changes
- View task details and prompts

### Phase 2: Task Management (1 week)

1. Add task creation endpoint
2. Add task update/delete endpoints
3. Prompt editing via web form
4. Start task endpoint (triggers local Zellij)

**Deliverables:**
- Create tasks from phone
- Edit prompts remotely
- Trigger task start (must be near local machine)

### Phase 3: Authentication (1 week)

1. Add JWT-based authentication
2. API key support for programmatic access
3. HTTPS configuration

**Deliverables:**
- Secure remote access
- Multi-device support

### Phase 4: Terminal Access (2-3 weeks)

1. Abstract Zellij controller behind interface
2. Implement PTY-based session manager
3. Add xterm.js frontend
4. WebSocket terminal proxy

**Deliverables:**
- Full interactive Claude sessions from browser
- No local terminal required

## Technology Recommendations

### Backend

- **HTTP Framework:** [Gin](https://github.com/gin-gonic/gin) or [Echo](https://echo.labstack.com/) - lightweight, fast
- **WebSocket:** [gorilla/websocket](https://github.com/gorilla/websocket) - standard Go WebSocket library
- **Authentication:** [golang-jwt/jwt](https://github.com/golang-jwt/jwt) - JWT handling

### Frontend

- **Framework:** React or Vue (whichever you prefer)
- **Terminal:** [xterm.js](https://xtermjs.org/) - browser terminal emulator
- **Mobile:** Progressive Web App (PWA) for mobile support

### Alternative: Existing Solutions

Consider using existing tools as shortcuts:

- **[ttyd](https://github.com/tsl0922/ttyd)** - Share terminal over web
- **[Gotty](https://github.com/yudai/gotty)** - Terminal as web app
- **[code-server](https://github.com/coder/code-server)** - VS Code in browser (for prompt editing)

## Security Considerations

1. **Authentication required** - Never expose without auth
2. **HTTPS only** - Encrypt all traffic
3. **Rate limiting** - Prevent abuse
4. **Input sanitization** - Especially for terminal input
5. **Network binding** - Consider localhost-only + tunnel (ngrok, tailscale)
6. **Token expiration** - Short-lived JWTs

## Example: Minimal API Server

```go
// internal/api/server.go
package api

import (
    "github.com/gin-gonic/gin"
    "github.com/gorilla/websocket"
    "your/project/internal/task"
    "your/project/internal/status"
)

type Server struct {
    manager *task.Manager
    watcher *status.Watcher
    upgrader websocket.Upgrader
}

func NewServer(manager *task.Manager, watcher *status.Watcher) *Server {
    return &Server{
        manager: manager,
        watcher: watcher,
        upgrader: websocket.Upgrader{
            CheckOrigin: func(r *http.Request) bool { return true },
        },
    }
}

func (s *Server) Run(addr string) error {
    r := gin.Default()

    // Task endpoints
    r.GET("/api/tasks", s.listTasks)
    r.GET("/api/tasks/:id", s.getTask)
    r.POST("/api/tasks", s.createTask)
    r.PUT("/api/tasks/:id", s.updateTask)
    r.DELETE("/api/tasks/:id", s.deleteTask)

    // Status WebSocket
    r.GET("/ws/status", s.statusWebSocket)

    return r.Run(addr)
}

func (s *Server) listTasks(c *gin.Context) {
    tasks := s.manager.List()
    c.JSON(200, tasks)
}

func (s *Server) statusWebSocket(c *gin.Context) {
    conn, err := s.upgrader.Upgrade(c.Writer, c.Request, nil)
    if err != nil {
        return
    }
    defer conn.Close()

    // Subscribe to status updates and forward to WebSocket
    updates := s.watcher.Subscribe()
    for update := range updates {
        conn.WriteJSON(update)
    }
}
```

## Conclusion

Adding remote/mobile access to Flock is feasible due to the well-structured codebase. Start with Phase 1 (read-only API) to get immediate value, then iterate based on usage patterns.

The main complexity lies in terminal emulation (Phase 4). Consider whether full terminal access is needed, or if task monitoring + creation is sufficient for mobile use cases.
