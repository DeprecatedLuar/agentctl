#!/usr/bin/env bash
# Schedule tool prerun - self-heal config/tools/schedule.conf into existence

mkdir -p config/tools

CONFIG_FILE="config/tools/schedule.conf"

if [ ! -f "$CONFIG_FILE" ]; then
  cat > "$CONFIG_FILE" <<'EOF'
# schedule.conf — where scheduled messages get delivered.
# format: CHANNEL=<comma-separated channel list, e.g. telegram,cli>
# CHANNEL=telegram,cli
EOF
fi
