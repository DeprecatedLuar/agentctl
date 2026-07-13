#!/usr/bin/env bash
set -e

JOB_ID="$TOOL_JOB_ID"
USER_ID="$TOOL_USER"
AGENT="$TOOL_AGENT"

source "./scripts/_common.sh"

MATCH=$(list_agent_jobs "$AGENT" "$USER_ID" | awk -F'\t' -v j="$JOB_ID" '$1 == j {print; exit}')
if [ -z "$MATCH" ]; then
  echo "ERROR: no scheduled message with job_id ${JOB_ID} belongs to ${USER_ID}."
  exit 1
fi

OUTPUT=$(atrm "$JOB_ID" 2>&1) || {
  echo "ERROR: failed to cancel job ${JOB_ID}"
  echo "$OUTPUT"
  exit 1
}

WHEN=$(printf '%s' "$MATCH" | cut -f2)
MESSAGE=$(printf '%s' "$MATCH" | cut -f3)
echo "Cancelled scheduled message (job ${JOB_ID}) for ${WHEN}: ${MESSAGE}"
