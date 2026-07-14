# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Build & Test

```bash
go build -o agentctl ./cmd/agentctl
go test ./...
go test ./internal/config -v -run TestParse_BasicSections

# Test with local agent (always use _test-agent, not new ones)
./agentctl init _test-agent
./agentctl serve _test-agent          # Terminal 1 (run/up still work as aliases)
./agentctl chat "hello" -a _test-agent  # Terminal 2
./agentctl -m "hello" -a _test-agent    # shortcut: unrecognized/no subcommand falls through to chat
```

**Testing requirement:** Use `_test-agent` with `openrouter/free` model. Never create new test agents.

## Architecture

Hexagonal: I/O adapters → Orchestrator → Domain logic

```
Gateways (cli/, telegram/) → ports.go (MessageHandler)
                            → orchestration.go (sequence calls only)
                            → session/ + agent/ + config/ (domain)
```

**Key packages:**
- `internal/ports.go` - MessageHandler, OutboundDispatcher, Gateway contracts
- `internal/orchestration.go` - Pure orchestration: hooks → resolve → load → agent.Run() → save
- `internal/config/` - agent.toml, tools/*.toml, prompt parser
- `internal/providers/llm/` - One file per provider (openai.go, openrouter.go, generic.go)
- `internal/agent/` - core.go (agentic loop), tools.go (shell execution)
- `internal/shell/` - Execute(cmd, dir) pure function
- `internal/hooks/` - .prerun.sh execution (non-fatal)
- `internal/session/` - resolve.go, identity.go, jsonl.go, migrate.go, title.go
- `internal/gateways/` - cli/ + telegram/ packages, dispatch.go
- `internal/syscommands/` - /new, /sessions, /sessions attach
- `internal/resolution/` - Two-phase: directives ({{file:}}, {{exec:}}) → variables ({{$var}})
- `internal/routines/` - In-process scheduler (scheduler.go): 60s tick + fsnotify hot-reload, fires routines/*.toml via agent.ExecuteRoutine
- `internal/commands/` - init.go, serve.go, chat.go, inject.go

**Agent folder:**
```
.prerun.sh              # Runs before each message (non-fatal)
config/agent.toml       # [agent], [access], [memory], [audio]
prompts/chat_template   # [>role] static, [>>role] input (or chat_template.md fallback)
tools/*.toml            # Command + params (auto-discover or explicit)
routines/*.toml         # Scheduled commands, fired by the serve daemon (no AI); example.toml skipped
.data/
  contacts.toml         # [[identity]] + [[contact]]
  sessions/{userID}/{sessionID}.jsonl
  sessions/{userID}/last-call.json  # If logging="debug" - last request+response, overwritten each call
```

**Flows:**
- Normal: Gateway → HandleMessage(gateway, contactID, displayName, content) → orchestrator
- Explicit: CLI --user/--session → HandleExplicitMessage(userID, sessionID, gateway, content)
- Delivery: --deliver/--inject → HandleMessageWithOptions(MessageOptions)
- Raw delivery: `deliver` command → HandleMessageWithOptions(MessageOptions{Raw: true}) → skips agent call, delivers Content verbatim
- Commands: Gateway detects "/" → syscommands.Parse() → helpers → format → return (no orchestrator)
- CLI dispatch shortcut: no subcommand, or an unrecognized one, forwards straight to `chat` (`cmd/agentctl/main.go`) — e.g. `agentctl -m "hi" -a my-agent` == `agentctl chat -m "hi" -a my-agent`. Chat's own flag parser/`message.Resolve`/`registry.ResolveAgentPath` report errors as normal. `-a`/`--agent` falls back to `$AGENTCTL_AGENT` when omitted, same pattern as `-u`/`$AGENTCTL_USER` and `-s`/`$AGENTCTL_SESSION`.

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

[advanced]
reasoning_carryover = "tools"  # none | tools | all — default "tools"
reasoning_effort = "medium"  # minimal | low | medium | high | max
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
report = ""     # Sent to the originating interface when the tool runs (empty=silent)

[location]
description = "City name"
type = "string"
required = true
enabled = true  # false=hide from AI
return = ""     # Override with literal/directive (auto-hides from AI)
env = ""        # Override injected env var name, replacing $TOOL_VAR (e.g. env = "GH_TOKEN" -> $GH_TOKEN)
```
- Commands run from tool directory (not agent root)
- `tools = []` recursively finds all .toml (including subdirs)
- `tools = ["name"]` only loads top-level tools/ (no subdirs)
- Parameters injected as both `{{var}}` (inline) and `$TOOL_VAR` (env) for safe multiline handling
- Need system context (user, session, etc.)? Declare a hidden param with `return = "{{$var}}"` (still shows up as `$TOOL_VAR`) — no ambient injection, stays declarative
- Need the exact env var name a wrapped script/CLI expects (e.g. `GH_TOKEN`)? Set `env = "GH_TOKEN"` on the param — replaces the `$TOOL_VAR` binding with the given name (must be a valid shell identifier, validated at load)
- `report`: opt-in per-tool line delivered to the originating interface (Telegram/etc.) when the tool runs, so tool usage is visible without reading host logs. Family prefix auto-derived from the tool's folder (`tools/memory/read.toml` -> `memory:`); tools directly in `tools/` emit the bare report. Resolves `{{name}}` (resolved param value, honours return-overrides), `{{$command}}` (fully-resolved shell command), `{{$result}}` (truncated stdout), plus standard directives/system vars. `{{$command}}`/`{{$result}}` are local tokens substituted after `resolution.Process` runs — never part of the global sysvar namespace. Non-fatal: a broken template logs a warning and is skipped.

**routines/*.toml:** (scheduled, AI-less counterpart to tools — parsed in `internal/config/routine.go`, fired by `internal/routines/scheduler.go` inside `serve`)
```toml
command = "agentctl deliver telegram -m 'Standup' -a ."   # required
description = "Weekday standup"   # optional, informational only (no AI sees routines)
enabled = true                    # optional, default true

[schedule]                        # required — exactly one shape (mode detected from `every`'s token type):
# every = "mon,wed,fri" + time = "09:00"   # ModeWeekday
# every = "1,15"        + time = "09:00"   # ModeDayOfMonth (1-31)
# every = "3d"          + time = "09:00"   # ModeDayInterval
# every = "6h"                             # ModeHourInterval (time forbidden)
# rrule = "FREQ=WEEKLY;BYDAY=MO,WE"        # ModeRRule (mutually exclusive with every/time)

[today]                           # params like tools, but populated only by `return` (no AI args)
return = "{{exec:date +%F}}"       # injected as {{today}} and $ROUTINE_TODAY (note ROUTINE_ prefix)
```
- `every` tokens must be one category — mixing (`"mon,3d"`) is a hard error (avoids cron OR-ambiguity)
- Scheduler: 60s tick, no crontab, no boot catch-up, overlap allowed; fires only while daemon runs
- Hot-reload via fsnotify on `routines/` dir, but the dir must exist at daemon startup (checked once)
- Context in `{{$...}}`: `{{$agent}}`, `{{$agentpath}}`, time vars only — no `{{$user}}`/`{{$session}}` (no session). Commands run from the routine file's own dir; `[environment]` injected like tools
- No `toolrun` equivalent — test via `serve --debug` + a near-future `time`, watch `.data/logs/`
- `init` scaffolds `routines/example.toml` (embedded `templates.RoutineExample`) as a fill-in reference; skipped by the shared `example.toml` exclusion in `walkToolsDir`, same as `tools/example.toml`

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
- `.last_session`: plain text `interface=sessionID` (key name frozen on-disk; means gateway)
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
   - System ($ prefix): `{{$agent}}`, `{{$agentpath}}`, `{{$user}}`, `{{$username}}`, `{{$session}}`, `{{$gateway}}` (`{{$interface}}` still works as a deprecated alias), `{{$timestamp}}`, `{{$date}}`, `{{$now}}`, `{{$model}}`, `{{$provider}}`
   - User (no prefix): `{{var}}` (future use)

Used by: prompt parser, tool return values

`resolution.SystemEnv(ctx)` exposes the same system variables as `$AGENTCTL_<NAME>` env vars (e.g. `$AGENTCTL_USER`, `$AGENTCTL_SESSION`) — injected into prerun scripts only, since prerun has no params schema to declare through (unlike tools, which opt in explicitly via `return = "{{$var}}"`; see tools/*.toml above).

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
- Runs from agent root, access to .env vars and `$AGENTCTL_*` context (see Template Resolution) — but since prerun runs before config load, `$AGENTCTL_MODEL`/`$AGENTCTL_PROVIDER` may be empty depending on the caller

## Debug

1. `--debug` flag: Enhanced slog with message previews, tool details (operational log verbosity — separate axis from #2)
2. `logging = "debug"` (agent.toml): Writes merged request+response JSON to `.data/sessions/{userID}/last-call.json`, overwritten on every call. Session-title-generation calls are excluded (they'd clobber the real conversation's record).

## Design Decisions

- **Hexagonal arch** - I/O → orchestration → domain
- **Ports at root** - `internal/ports.go` for MessageHandler, OutboundDispatcher, Gateway
- **SessionStore in domain** - Avoids circular imports
- **Merged contacts.toml** - [[identity]] (user-edited) + [[contact]] (auto-generated)
- **Access control** - Three-tier: Identity.allowed > Contact.allowed > allow_by_default
- **Gateways own UX** - Detect commands, call helpers, format for CLI/Telegram
- **No CLI framework** - Stdlib arg parsing, gohelp-luar for docs only
- **Modular providers** - One file per provider (openai.go, openrouter.go, generic.go)
- **Shell-based tools** - `sh -c` with {{var}} substitution
- **Routines in-process** - Scheduler lives in `serve` daemon, no system crontab; typed `every` shapes over raw cron to dodge OR-ambiguity
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

## Adding Gateways

Package: `internal/gateways/{name}/`
```
interface.go  # Start(), message routing, Sender
commands.go   # handleCommand(), UX formatting
```
1. Implement `internal.Gateway` (Start method)
2. Implement `Sender` (GatewayName, Send)
3. Accept `internal.MessageHandler` in constructor
4. Detect "/" → syscommands.Parse() → helpers → format → return
5. Messages → handler.HandleMessage(gateway, contactID, displayName, username, content)
6. Add to factory in `internal/commands/serve.go`

Responsibility: Pure I/O, detect commands, format output. No session logic.
