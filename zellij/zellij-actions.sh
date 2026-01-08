#!/bin/bash

ZELLIJ_LAYOUTS="${ZELLIJ_LAYOUTS:-$(dirname "$0")/layouts}"

#  zellij action new-tab --layout zellij/ai_with_editor_layout.kdl --name ai_agent_0
zellij_new_ai_tab() {
    zellij action new-tab -n "ai_agent_$1" \
        --layout "$ZELLIJ_LAYOUTS/ai_with_editor.kdl"
}


