#!/usr/bin/env bash
# Memory tool prerun - ensure core.md exists, and that default.prompt
# loads it via a [>user] section.

mkdir -p .data/tools/memory/"$USER"

CORE_FILE=".data/tools/memory/$USER/core.md"

if [ ! -f "$CORE_FILE" ]; then
  echo "#PLACEHOLDER" > "$CORE_FILE"
fi

# --- Ensure default.prompt injects memory into a [>user] section ---

PROMPT_FILE="prompts/default.prompt"
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
