#!/usr/bin/env bash
# Memory tool prerun - self-heal every file declared in memory.config for
# this user, and ensure chat_template loads core.md via a [>user] section.

mkdir -p .data/tools/memory/"$USER"
mkdir -p config/tools

CONFIG_FILE="config/tools/memory.conf"

if [ ! -f "$CONFIG_FILE" ]; then
  cat > "$CONFIG_FILE" <<'EOF'
# memory.conf — declares every memory file for this agent.
# .prerun.sh self-heals these into existence per user (as <name>.md).
# format: <name>: <max_chars, 0=unlimited>: <description>

core: 2000: your always-loaded memory snapshot
EOF
fi

while IFS=':' read -r name limit desc; do
  name="$(echo "$name" | xargs)"
  [ -z "$name" ] && continue
  case "$name" in \#*) continue ;; esac

  case "$name" in
    *[!a-zA-Z0-9_-]*)
      echo "WARN: skipping invalid entry in config/tools/memory.conf: '${name}' (name must be alphanumeric/underscore/hyphen, no spaces — likely a missing ':')" >&2
      continue
      ;;
  esac

  MEM_FILE=".data/tools/memory/$USER/${name}.md"
  if [ ! -f "$MEM_FILE" ]; then
    echo "#PLACEHOLDER" > "$MEM_FILE"
  fi
done < "$CONFIG_FILE"

# --- Ensure chat_template injects memory into a [>user] section ---

# chat_template (no extension) takes priority over chat_template.md, mirroring
# the loader in internal/config/prompt.go.
PROMPT_FILE="prompts/chat_template"
[ -f "$PROMPT_FILE" ] || PROMPT_FILE="prompts/chat_template.md"
MEMORY_DIRECTIVE='{{file:{{$agentpath}}/.data/tools/memory/{{$user}}/core.md}}'

# Line number of the next section header ("[>role]" or "[>>role]") after $1
next_header_after() {
  awk -v start="$1" 'NR > start && $0 ~ /^\[>{1,2}[a-zA-Z]+\]$/ { print NR; exit }' "$PROMPT_FILE"
}

# Drop trailing blank lines from stdin
rstrip_blank() {
  awk 'NF{last=NR} {a[NR]=$0} END{for(i=1;i<=last;i++) print a[i]}'
}

if [ -f "$PROMPT_FILE" ] && ! grep -qF "$MEMORY_DIRECTIVE" "$PROMPT_FILE"; then
  SYS_LINE=$(grep -nx '\[>system\]' "$PROMPT_FILE" | head -1 | cut -d: -f1)

  if [ -n "$SYS_LINE" ]; then
    USER_LINE=$(grep -nx '\[>user\]' "$PROMPT_FILE" | head -1 | cut -d: -f1)
    TOTAL=$(wc -l < "$PROMPT_FILE")
    TMP=$(mktemp)

    if [ -n "$USER_LINE" ]; then
      # [>user] exists: append the block to the end of its content
      END=$(next_header_after "$USER_LINE")
      [ -n "$END" ] || END=$((TOTAL + 1))

      head -n $((END - 1)) "$PROMPT_FILE" | rstrip_blank > "$TMP"
      printf '\n<memory>\n%s\n</memory>\n\n' "$MEMORY_DIRECTIVE" >> "$TMP"
      tail -n +"$END" "$PROMPT_FILE" >> "$TMP"
    else
      # [>user] missing: insert a new section right after [>system]'s content
      END=$(next_header_after "$SYS_LINE")
      [ -n "$END" ] || END=$((TOTAL + 1))

      head -n $((END - 1)) "$PROMPT_FILE" | rstrip_blank > "$TMP"
      printf '\n[>user]\n<memory>\n%s\n</memory>\n\n' "$MEMORY_DIRECTIVE" >> "$TMP"
      tail -n +"$END" "$PROMPT_FILE" >> "$TMP"
    fi

    mv "$TMP" "$PROMPT_FILE"
  fi
fi
