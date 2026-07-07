#!/usr/bin/env bash
set -e

WHEN="$TOOL_WHEN"
MESSAGE="$TOOL_MESSAGE"
USER_ID="$TOOL_USER"
AGENT="$TOOL_AGENT"
CONFIG_FILE="${AGENT_PATH}/config/tools/schedule.conf"

if [ ! -f "$CONFIG_FILE" ]; then
  echo "ERROR: schedule.conf not found at ${CONFIG_FILE}"
  exit 1
fi

CHANNEL=$(grep -E '^[[:space:]]*CHANNEL=' "$CONFIG_FILE" | head -1 | cut -d= -f2- | xargs)
if [ -z "$CHANNEL" ]; then
  echo "ERROR: CHANNEL not set in schedule.conf"
  exit 1
fi

NOTE="Scheduled message delivered at ${WHEN}."
CMD=$(printf 'agentctl deliver %q -m %q --inject --note %q --user %q -a %q' "$CHANNEL" "$MESSAGE" "$NOTE" "$USER_ID" "$AGENT")

# $WHEN is intentionally unquoted: at expects two words ("HH:MM" "YYYY-MM-DD")
OUTPUT=$(echo "$CMD" | at $WHEN 2>&1) || {
  echo "ERROR: failed to schedule job"
  echo "$OUTPUT"
  exit 1
}

echo "Scheduled for ${WHEN}: ${MESSAGE}"
