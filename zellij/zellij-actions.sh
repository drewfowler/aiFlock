#!/bin/bash

ZELLIJ_LAYOUTS="${ZELLIJ_LAYOUTS:-$(dirname "$0")/layouts}"

new-ai-tab() {
    zellij action new-tab -n "ai_agent_$1" \
        --layout "$ZELLIJ_LAYOUTS/ai_with_editor.kdl"
}

$@
