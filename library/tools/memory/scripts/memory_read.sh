#!/usr/bin/env bash
set -e

USER_ID="$TOOL_USER"
FILE="$TOOL_FILE"

MEM_DIR="${AGENT_PATH}/.data/tools/memory/${USER_ID}"
TARGET_FILE="${MEM_DIR}/${FILE}.md"

if [ ! -f "$TARGET_FILE" ]; then
  echo "ERROR: file not found: ${FILE}"
  exit 1
fi

cat "$TARGET_FILE"
