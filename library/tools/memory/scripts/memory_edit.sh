#!/usr/bin/env bash
set -e

USER_ID="$TOOL_USER"
FILE="$TOOL_FILE"
OLD_STR="$TOOL_OLD"
NEW_STR="$TOOL_NEW"
CONFIG_FILE="${AGENT_PATH}/config/tools/memory.conf"

MEM_DIR="${AGENT_PATH}/.data/tools/memory/${USER_ID}"
TARGET_FILE="${MEM_DIR}/${FILE}.md"

# Look up this file's char limit from memory.config (0 or missing = unlimited)
MAX_CHARS=$(grep -E "^${FILE}[[:space:]]*:" "$CONFIG_FILE" 2>/dev/null | head -1 | awk -F: '{print $2}' | xargs)
[ -z "$MAX_CHARS" ] && MAX_CHARS=0

# Check file exists
if [ ! -f "$TARGET_FILE" ]; then
  echo "ERROR: file not found: ${FILE}"
  exit 1
fi

CURRENT=$(cat "$TARGET_FILE")

if [[ "$CURRENT" != *"$OLD_STR"* ]]; then
  echo "ERROR: string not found in ${FILE}"
  exit 1
fi

NEW_CONTENT="${CURRENT//$OLD_STR/$NEW_STR}"
NEW_SIZE=${#NEW_CONTENT}

if [ "$MAX_CHARS" -gt 0 ] && [ "$NEW_SIZE" -gt "$MAX_CHARS" ]; then
  echo "ERROR: would be ${NEW_SIZE} chars (cap ${MAX_CHARS}). Compress by making it objective and removing redundancy or verbose explanations."
  exit 1
fi

printf '%s' "$NEW_CONTENT" > "$TARGET_FILE"
echo "OK (${NEW_SIZE}/$( [ "$MAX_CHARS" -gt 0 ] && echo "${MAX_CHARS}" || echo "∞" ))"
echo "---"
echo "$NEW_CONTENT"
