#!/usr/bin/env bash
set -e

USER_ID="$TOOL_USER"
AGENT="$TOOL_AGENT"

source "./scripts/_common.sh"

FOUND=0
while IFS=$'\t' read -r job when message; do
  [ -z "$job" ] && continue
  FOUND=1
  echo "job=${job} when=\"${when}\" message=\"${message}\""
done < <(list_agent_jobs "$AGENT" "$USER_ID")

if [ "$FOUND" -eq 0 ]; then
  echo "No scheduled messages pending for ${USER_ID}."
fi
