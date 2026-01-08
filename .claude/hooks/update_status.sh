#!/bin/bash
# Flock status update hook for Claude Code
# This script updates the status file for a task based on the hook event

set -e

# Read input from stdin (JSON from Claude Code)
INPUT=$(cat)

# Get task info from environment variables
TASK_ID="${FLOCK_TASK_ID:-}"
TAB_NAME="${FLOCK_TAB_NAME:-}"
STATUS_DIR="${FLOCK_STATUS_DIR:-/tmp/flock}"

# Exit if no task ID is set (not running in flock context)
if [ -z "$TASK_ID" ]; then
    exit 0
fi

# Extract hook event name from input
HOOK_EVENT=$(echo "$INPUT" | grep -o '"hook_event_name"[[:space:]]*:[[:space:]]*"[^"]*"' | sed 's/.*"\([^"]*\)"$/\1/' || echo "")

# Map hook event to status
case "$HOOK_EVENT" in
    "UserPromptSubmit")
        STATUS="WAITING"
        ;;
    "PreToolUse")
        STATUS="WORKING"
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
updated=$(date +%s)
tab_name=$TAB_NAME
EOF

exit 0
