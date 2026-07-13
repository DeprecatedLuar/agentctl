#!/usr/bin/env bash
set -e

WHEN="$TOOL_WHEN"
MESSAGE="$TOOL_MESSAGE"
USER_ID="$TOOL_USER"
AGENT="$TOOL_AGENT"
CONFIG_FILE="${AGENT_PATH}/config/tools/schedule.conf"

source "./scripts/_common.sh"

if [ ! -f "$CONFIG_FILE" ]; then
  echo "ERROR: schedule.conf not found at ${CONFIG_FILE}"
  exit 1
fi

CHANNEL=$(grep -E '^[[:space:]]*CHANNEL=' "$CONFIG_FILE" | head -1 | cut -d= -f2- | xargs)
if [ -z "$CHANNEL" ]; then
  echo "ERROR: CHANNEL not set in schedule.conf"
  exit 1
fi

EXISTING=$(list_agent_jobs "$AGENT" "$USER_ID" | awk -F'\t' -v w="$WHEN" '$2 == w {print $1; exit}')
if [ -n "$EXISTING" ]; then
  echo "ERROR: a message is already scheduled for ${USER_ID} at ${WHEN} (job ${EXISTING}). Use list_scheduled_messages to review it, or cancel_scheduled_message to replace it."
  exit 1
fi

CREATED_AT=$(date '+%H:%M %Y-%m-%d')
NOTE="This message was scheduled on ${CREATED_AT} for delivery at ${WHEN}."
CMD=$(printf 'agentctl deliver %q -m %q --inject --note %q --user %q -a %q' "$CHANNEL" "$MESSAGE" "$NOTE" "$USER_ID" "$AGENT")

MARKER=$(printf '# AGENTCTL_SCHEDULE agent_b64=%s user_b64=%s when_b64=%s message_b64=%s' \
  "$(b64enc "$AGENT")" "$(b64enc "$USER_ID")" "$(b64enc "$WHEN")" "$(b64enc "$MESSAGE")")
JOB_BODY=$(printf '%s\n%s\n' "$MARKER" "$CMD")

# $WHEN is intentionally unquoted: at expects two words ("HH:MM" "YYYY-MM-DD")
OUTPUT=$(printf '%s\n' "$JOB_BODY" | at $WHEN 2>&1) || {
  echo "ERROR: failed to schedule job"
  echo "$OUTPUT"
  exit 1
}

echo "Scheduled for ${WHEN}: ${MESSAGE}"
