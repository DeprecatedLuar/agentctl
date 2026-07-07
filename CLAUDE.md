# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Build & Test

```bash
go build -o agentctl ./cmd/agentctl
go test ./...
go test ./internal/config -v -run TestParse_BasicSections

# Test with local agent (always use _test-agent, not new ones)
./agentctl init _test-agent
./agentctl run _test-agent          # Terminal 1
./agentctl chat "hello" -a _test-agent  # Terminal 2
```

**Testing requirement:** Use `_test-agent` with `openrouter/free` model. Never create new test agents.

## Architecture

Hexagonal: I/O adapters → Orchestrator → Domain logic

```
Interfaces (cli/, telegram/) → ports.go (MessageHandler)
                              → orchestration.go (sequence calls only)
                              → session/ + agent/ + config/ (domain)
```

**Key packages:**
- `internal/ports.go` - MessageHandler, OutboundDispatcher, Interface contracts
- `internal/orchestration.go` - Pure orchestration: hooks → resolve → load → agent.Run() → save
- `internal/config/` - agent.toml, tools/*.toml, prompt parser
- `internal/providers/llm/` - One file per provider (openai.go, openrouter.go, generic.go)
- `internal/agent/` - core.go (agentic loop), tools.go (shell execution)
- `internal/shell/` - Execute(cmd, dir) pure function
- `internal/hooks/` - .prerun.sh execution (non-fatal)
- `internal/session/` - resolve.go, identity.go, jsonl.go, migrate.go, title.go
- `internal/interfaces/` - cli/ + telegram/ packages, dispatch.go
- `internal/syscommands/` - /new, /sessions, /sessions attach
- `internal/resolution/` - Two-phase: directives ({{file:}}, {{exec:}}) → variables ({{$var}})
- `internal/commands/` - init.go, run.go, chat.go, inject.go

**Agent folder:**
```
.prerun.sh              # Runs before each message (non-fatal)
config/agent.toml       # [agent], [access], [memory], [audio]
prompts/default.prompt  # [>role] static, [>>role] input
tools/*.toml            # Command + params (auto-discover or explicit)
.data/
  contacts.toml         # [[identity]] + [[contact]]
  sessions/{userID}/{sessionID}.jsonl
  sessions/{userID}/last-call.json  # If logging="debug" - last request+response, overwritten each call
```

**Flows:**
- Normal: Interface → HandleMessage(iface, contactID, displayName, content) → orchestrator
- Explicit: CLI --user/--session → HandleExplicitMessage(userID, sessionID, iface, content)
- Delivery: --deliver/--inject → HandleMessageWithOptions(MessageOptions)
- Commands: Interface detects "/" → syscommands.Parse() → helpers → format → return (no orchestrator)

## Config Files

**agent.toml:**
```toml
[agent]
provider = "openrouter"  # or "openai" or http(s):// URL
model = "openrouter/free"
tools = []               # Empty=auto-discover, or ["name1", "name2"]
logging = false | true | "debug"  # "debug" enables .data/sessions/{userID}/last-call.json

[access]
allow_by_default = true
interfaces = ["cli", "telegram"]

[memory]
max_messages = 100  # Keep last N messages (0=no persistence)

[audio]  # Optional
provider = "whisper"
model = "whisper-1"
```

**prompt file:**
- `[>role]` - Static (directives + variables at parse time)
- `[>>role]` - Input ({{input}} replaced per message)
- Directives: `{{file:path}}`, `{{exec:command}}` (recursive, 10-level limit)
- Variables: `{{$agent}}`, `{{$user}}`, `{{$session}}`, `{{$timestamp}}`, `{{$date}}`, `{{$now}}`, etc.
- Nested directives: `{{file:path/{{$user}}/notes.md}}` (variables in paths)

**tools/*.toml:**
```toml
command = "curl wttr.in/{{location}}"
description = "Get weather"

[location]
description = "City name"
type = "string"
required = true
enabled = true  # false=hide from AI
return = ""     # Override with literal/directive (auto-hides from AI)
```
- Commands run from tool directory (not agent root)
- `tools = []` recursively finds all .toml (including subdirs)
- `tools = ["name"]` only loads top-level tools/ (no subdirs)
- Parameters injected as both `{{var}}` (inline) and `$TOOL_VAR` (env) for safe multiline handling

**.data/contacts.toml:**
```toml
[[identity]]
id = "alice"
contacts = ["cli:alice", "telegram:123456789"]
allowed = true  # Optional override

[[contact]]  # Auto-generated
interface = "telegram"
id = "123456789"
display_name = "Alice"
first_seen = 2026-06-17T15:04:05Z
allowed = true
```

## Sessions & Identity

- Path: `.data/sessions/{userID}/{sessionID}.jsonl`
- Session ID: `YYYYMMDD_HHMMSS_<6-hex>`
- `.last_session`: plain text `interface=sessionID`
- Identity linking merges multiple contacts → single user folder
- **Thread safety:** Per-session mutexes (sync.Map) for Load/Save/SetMeta
- **Migration:** Startup bulk + lazy per-message (idempotent, no restart needed)
- **Auto-title:** Async LLM-generated titles after first exchange (non-blocking)

## Template Resolution

Two-phase pipeline in `internal/resolution/`:

1. **Directives** (processDirectives):
   - `{{file:path}}` - Load file (binary detection)
   - `{{exec:command}}` - Run via sh -c from agent folder
   - Variables in directive paths substituted first: `{{file:path/{{$user}}/file.md}}`
   - Recursive (10-level), inline allowed, unknown = error
   - Validation mode (startup): skips variable substitution

2. **Variables** (substituteVariables):
   - Detection: `:` = directive, no `:` = variable
   - System ($ prefix): `{{$agent}}`, `{{$agentpath}}`, `{{$user}}`, `{{$username}}`, `{{$session}}`, `{{$interface}}`, `{{$timestamp}}`, `{{$date}}`, `{{$now}}`, `{{$model}}`, `{{$provider}}`
   - User (no prefix): `{{var}}` (future use)

Used by: prompt parser, tool return values

## Hot-Reload

Orchestrator loads config/tools/prompt on **every request**:
- Edit files while daemon runs → next message uses new config
- Malformed configs return errors, daemon stays running
- .prerun.sh changes apply immediately

## Prerun Hooks

Runs before each agent execution:
- Checks `.prerun.sh` first (hidden), fallback to `prerun.sh`
- Non-fatal: errors logged, agent continues
- Common: source tool-specific `.prerun.sh` files, mkdir -p, touch files
- Runs from agent root, access to .env vars

## Debug

1. `--debug` flag: Enhanced slog with message previews, tool details (operational log verbosity — separate axis from #2)
2. `logging = "debug"` (agent.toml): Writes merged request+response JSON to `.data/sessions/{userID}/last-call.json`, overwritten on every call. Session-title-generation calls are excluded (they'd clobber the real conversation's record).

## Design Decisions

- **Hexagonal arch** - I/O → orchestration → domain
- **Ports at root** - `internal/ports.go` for MessageHandler, OutboundDispatcher, Interface
- **SessionStore in domain** - Avoids circular imports
- **Merged contacts.toml** - [[identity]] (user-edited) + [[contact]] (auto-generated)
- **Access control** - Three-tier: Identity.allowed > Contact.allowed > allow_by_default
- **Interfaces own UX** - Detect commands, call helpers, format for CLI/Telegram
- **No CLI framework** - Stdlib arg parsing, gohelp-luar for docs only
- **Modular providers** - One file per provider (openai.go, openrouter.go, generic.go)
- **Shell-based tools** - `sh -c` with {{var}} substitution
- **JSONL not SQLite** - Append-only, easier debugging
- **Runtime loading** - Hot-reload on every request
- **Dual migration** - Startup bulk + lazy per-message
- **Async title generation** - Non-blocking, in-memory messages
- **Per-session mutexes** - Zero contention between sessions
- **Two-phase resolution** - Directives first, variables second
- **System variables** - $ prefix distinguishes from user vars
- **Prerun hooks** - Per-message self-healing setup

## Adding Providers

File: `internal/providers/llm/{name}.go`
1. Implement `Provider` interface (SendMessages)
2. Load API key from .env (godotenv)
3. Convert Message format
4. Add to `provider.go` factory
5. Factory routes named providers + http(s):// URLs

## Adding Interfaces

Package: `internal/interfaces/{name}/`
```
interface.go  # Start(), message routing, Sender
commands.go   # handleCommand(), UX formatting
```
1. Implement `internal.Interface` (Start method)
2. Implement `Sender` (InterfaceName, Send)
3. Accept `internal.MessageHandler` in constructor
4. Detect "/" → syscommands.Parse() → helpers → format → return
5. Messages → handler.HandleMessage(iface, contactID, displayName, username, content)
6. Add to factory in `internal/commands/run.go`

Responsibility: Pure I/O, detect commands, format output. No session logic.
