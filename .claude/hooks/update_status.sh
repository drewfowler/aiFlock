#!/bin/bash
# Flock status update hook for Claude Code
# This script updates the status file for a task based on the hook event

# Don't use set -e as we want to handle errors gracefully

# Read input from stdin (JSON from Claude Code)
INPUT=$(cat)

# Get task info from environment variables
TASK_ID="${FLOCK_TASK_ID:-}"
TASK_NAME="${FLOCK_TASK_NAME:-}"
TAB_NAME="${FLOCK_TAB_NAME:-}"
STATUS_DIR="${FLOCK_STATUS_DIR:-/tmp/flock}"

# Exit silently if no task ID is set (not running in flock context)
# This prevents writing invalid status files
if [ -z "$TASK_ID" ]; then
    exit 0
fi

# Validate task ID is not empty or whitespace
TASK_ID=$(echo "$TASK_ID" | tr -d '[:space:]')
if [ -z "$TASK_ID" ]; then
    exit 0
fi

# Extract hook event name from input JSON
# Claude Code sends: {"hook_event_name": "Stop", ...} or similar
HOOK_EVENT=$(echo "$INPUT" | sed -n 's/.*"hook_event_name"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/p')

# If hook_event_name not found, try to get it from CLAUDE_HOOK_EVENT_NAME env var
if [ -z "$HOOK_EVENT" ]; then
    HOOK_EVENT="${CLAUDE_HOOK_EVENT_NAME:-}"
fi

# Map hook event to status
case "$HOOK_EVENT" in
    "UserPromptSubmit")
        # User submitted a prompt, Claude is now working
        STATUS="WORKING"
        ;;
    "PreToolUse")
        STATUS="WORKING"
        ;;
    "PostToolUse")
        STATUS="WORKING"
        ;;
    "Notification")
        # Claude is waiting for user input/permission
        STATUS="WAITING"
        ;;
    "Stop")
        STATUS="DONE"
        ;;
    "SubagentStop")
        # Don't change status for subagent stops
        exit 0
        ;;
    *)
        # Unknown event, don't update
        exit 0
        ;;
esac

# Ensure status directory exists
mkdir -p "$STATUS_DIR"

# Write status file
STATUS_FILE="$STATUS_DIR/$TASK_ID.status"
cat > "$STATUS_FILE" << EOF
status=$STATUS
task_id=$TASK_ID
task_name=$TASK_NAME
updated=$(date +%s)
tab_name=$TAB_NAME
EOF

exit 0
