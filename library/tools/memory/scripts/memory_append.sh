#!/usr/bin/env bash
set -e

USER_ID="$TOOL_USER"
CONTENT="$TOOL_CONTENT"
IMPORTANCE="$TOOL_IMPORTANCE"
TYPE="$TOOL_TYPE"
TIMESTAMP="$TOOL_TIMESTAMP"
SESSION_REF="$TOOL_SESSION_REF"

LOG_DIR="${AGENT_PATH}/.data/tools/memory/${USER_ID}"
LOG_FILE="${LOG_DIR}/log.jsonl"

if ! [[ "$IMPORTANCE" =~ ^([1-9]|10)$ ]]; then
  echo "ERROR: importance must be an integer 1-10, got '${IMPORTANCE}'"
  exit 1
fi

mkdir -p "$LOG_DIR"

LINE=$(jq -nc \
  --arg timestamp "$TIMESTAMP" \
  --arg type "$TYPE" \
  --argjson importance "$IMPORTANCE" \
  --arg content "$CONTENT" \
  --arg session_ref "$SESSION_REF" \
  '{timestamp: $timestamp, type: $type, importance: $importance, content: $content, session_ref: $session_ref}')

printf '%s\n' "$LINE" >> "$LOG_FILE"

echo "OK: logged (importance ${IMPORTANCE})"
