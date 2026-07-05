#!/usr/bin/env bash
set -e

CONFIG_FILE="${AGENT_PATH}/config/tools/memory.conf"

if [ ! -f "$CONFIG_FILE" ]; then
  echo "ERROR: memory.conf not found"
  exit 1
fi

grep -v '^#' "$CONFIG_FILE" | grep -v '^[[:space:]]*$'
