#!/usr/bin/env bash
# Shared helpers for schedule_message / list_scheduled_messages /
# cancel_scheduled_message. Not itself a tool - sourced by the three scripts
# above. State lives entirely in the OS `at` spool: each job we create embeds
# an "# AGENTCTL_SCHEDULE ..." marker comment (base64 fields, so agent path /
# user id / message can contain anything) that these helpers read back.

b64enc() { printf '%s' "$1" | base64 -w0; }
b64dec() { printf '%s' "$1" | base64 -d; }

# list_agent_jobs <agent> [user]
# Prints one line per pending `at` job carrying an AGENTCTL_SCHEDULE marker
# for <agent> (and, if [user] is non-empty, that user too): "job\twhen\tmessage".
# Jobs without our marker (foreign `at` usage) are silently skipped.
list_agent_jobs() {
  local agent="$1" user="$2" job marker agent_b64 user_b64 when_b64 message_b64

  for job in $(atq | awk '{print $1}'); do
    marker=$(at -c "$job" 2>/dev/null | grep '^# AGENTCTL_SCHEDULE ')
    [ -z "$marker" ] && continue

    agent_b64=$(printf '%s\n' "$marker" | grep -oE 'agent_b64=[^ ]+' | cut -d= -f2)
    user_b64=$(printf '%s\n' "$marker" | grep -oE 'user_b64=[^ ]+' | cut -d= -f2)
    when_b64=$(printf '%s\n' "$marker" | grep -oE 'when_b64=[^ ]+' | cut -d= -f2)
    message_b64=$(printf '%s\n' "$marker" | grep -oE 'message_b64=[^ ]+' | cut -d= -f2)

    [ "$(b64dec "$agent_b64")" = "$agent" ] || continue
    if [ -n "$user" ] && [ "$(b64dec "$user_b64")" != "$user" ]; then
      continue
    fi

    printf '%s\t%s\t%s\n' "$job" "$(b64dec "$when_b64")" "$(b64dec "$message_b64")"
  done
}
