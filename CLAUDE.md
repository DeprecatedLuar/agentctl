# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

`agentctl` is a CLI tool for building agentic workflows. It creates agent folders with configuration files, manages tool definitions, and runs agents via daemon architecture with multiple interfaces (CLI via Unix socket, Telegram bot).

## Build & Test

```bash
# Build binary
go build -o agentctl ./cmd/agentctl

# Run tests
go test ./...

# Run specific test
go test ./internal/config -v -run TestParse_BasicSections

# Test with local agent
./agentctl init _test-agent
./agentctl run _test-agent          # Start daemon (Terminal 1)
./agentctl chat "hello" -a _test-agent  # Send message (Terminal 2)

# View help documentation
./agentctl help                     # Overview + topic list
./agentctl help setup               # Getting started guide
./agentctl help tools               # Tool definition format
```

## Testing Best Practices

**IMPORTANT:** Always use free models for testing to avoid unnecessary API costs.

```bash
# Use OpenRouter's free model router for testing
provider = "openrouter"
model = "openrouter/free"

# List all free models with tool support
./agentctl models openrouter --free --tools

# Good free options with tool support:
# - openrouter/free (meta-router, automatically uses free models)
# - meta-llama/llama-3.3-70b-instruct:free
# - google/gemma-4-26b-a4b-it:free
```

The `_test-agent` folder uses `openrouter/free` - update this if testing requires specific model features.

## Architecture

### Hexagonal Architecture (Ports & Adapters)

The system follows hexagonal architecture with clear separation between I/O adapters, application orchestration, and domain logic:

```
┌─────────────────────────────────────┐
│   INPUT ADAPTERS (I/O Layer)        │
│   - CLI (Unix socket)                │
│   - Telegram (bot polling)           │
│   Pure transport, no business logic  │
└──────────────┬──────────────────────┘
               │ MessageHandler (port)
               ↓
┌─────────────────────────────────────┐
│   APPLICATION (Orchestrator)        │
│   ConversationService               │
│   - session.Resolve()               │
│   - config/tools/prompt loading     │
│   - session.Load() history          │
│   - agent.Run()                     │
│   - session.Save()                  │
│   - session.GenerateTitle() (async) │
└──────────────┬──────────────────────┘
               │
               ├─→ SessionStore (port)
               ├─→ agent.Run() (direct)
               └─→ Dispatcher (port, future)

┌─────────────────────────────────────┐
│   OUTPUT ADAPTERS                   │
│   - JSONLStore (session persistence)│
│   - LLM providers (OpenAI, etc)     │
└─────────────────────────────────────┘
```

### Component Boundaries

**1. internal/config** - All file loading (agent.toml, tools/*.toml, prompt file)
- `agent.go`: AgentConfig with provider, model, tools, interfaces, memory settings
- `tool.go`: ToolConfig with dynamic parameter sections (all TOML sections except command/description are parameters)
- `prompt.go`: Custom format with `[>role]` static sections and `[>>role]` input section

**2. internal/providers/llm** - AI provider abstraction (modular, one file per provider)
- `provider.go`: Interface contract + routing factory
- `openai.go`: OpenAI provider (self-contained)
- `openrouter.go`: OpenRouter provider (self-contained, separate for single responsibility)
- `generic.go`: OpenAI-compatible custom endpoints (Ollama, LM Studio, vLLM)
- Each provider loads API keys from `.env` via godotenv, falls back to env vars

**3. internal/agent** - Agent runtime execution
- `core.go`: Run() builds messages (static + history + input), calls provider in agentic loop
- `tools.go`: Shell command execution with `{{var}}` substitution, captures stdout/stderr
- Pure runtime - no business logic, no session management

**4. internal/shell** - Pure shell execution utility
- `execute.go`: Execute(cmd, dir) returns stdout, stderr, exitCode, err
- Used by both ExecuteTool and prompt directive processor
- No logging or formatting - pure function

**5. internal/interfaces** - I/O adapters (pure transport layer)
- `interface.go`: MessageHandler port (application contract), Dispatcher port (future)
- `cli.go`: Unix socket listener, JSON protocol
  - Normal flow: pure I/O adapter (passes contact ID to handler)
  - Exception: Explicit resolution for `--user/--session` flags (uses session.ResolveExplicit)
- `telegram.go`: Telegram bot with long polling, typing indicators
  - Pure I/O adapter (no session imports, no resolution logic)
- **Key principle**: Interfaces are thin I/O adapters that call MessageHandler, nothing more

**6. internal/session** - Session management domain (port implementation)
- `session.go`: ResolvedSession type, NewSessionID() with `YYYYMMDD_HHMMSS_<6-hex>` format, SessionMeta with title
- `identity.go`: identities.toml parsing, ResolveUser() for linking contacts to named identities
- `contacts.go`: Contact logging with deduplication
- `last.go`: .last_session tracking (plain text `interface=sessionID` format)
- `jsonl.go`: JSONL storage implementation with per-session mutex for race protection
- `resolve.go`: Resolve() for auto user/session resolution, ResolveExplicit() for explicit CLI flags
- `migrate.go`: MigrateOnStartup() for bulk migration, MigrateContact() for lazy per-message migration
- `title.go`: GenerateTitle() for async LLM-based session title generation
- `store.go`: SessionStore interface (port contract)
- Storage: `{agent_folder}/.data/sessions/{userID}/{sessionID}.jsonl`
- Respects `memory.max_messages` limit from agent.toml
- **Thread Safety**: Per-session mutexes prevent race conditions between concurrent Save/Load/SetMeta operations

**7. internal/service** - Application orchestration layer
- `conversation.go`: ConversationService orchestrates full message flow
  - Implements MessageHandler interface
  - HandleMessage(iface, contactID, displayName, content) - auto resolution
  - HandleExplicitMessage(userID, sessionID, iface, content) - explicit resolution (CLI only)
  - Pure orchestration: calls session API → config API → agent → session API
  - No domain logic, only sequencing port calls
  - Loads config/tools/prompt fresh on each request (hot-reload)
  - Triggers async title generation for new sessions

**8. internal/commands** - CLI command handlers
- `init.go`: Scaffold agent folder from embedded templates
- `run.go`: Start daemon, load config/tools/prompt, spawn interfaces, run migration
- `chat.go`: Send message to Unix socket (defaults to current dir, flags: --agent/-a, --user/-u, --session/-s)

**9. internal/directives** - Directive processor for {{file:}} and {{exec:}} syntax
- `process.go`: ProcessDirectives() with recursive expansion (10-level depth limit)
- Used by both prompt parser and tool parameter return values
- Supports relative paths (agent folder), ~/ expansion, absolute paths

**10. internal/debug** - Debug utilities for inspecting AI requests
- LogRequest/LogResponse/LogToolExecution functions for structured debug logging
- Writes timestamped JSON files to `.data/debug-calls/` when `debug_calls = true`
- Auto-cleanup to prevent disk bloat (keeps last 10 files)

**11. internal/registry** - XDG agent registry for name-based resolution
- Registry file: `~/.local/share/agentctl/agents` (one path per line)
- Register() adds agent to registry, Resolve() looks up by name or path
- Auto-cleanup of dead entries, handles multiple-match ambiguity

**12. internal/templates** - Embedded template files (files/ directory)
- Pure data package, no logic
- Template structure: `files/config/`, `files/prompts/`, `files/tools/`

**13. cmd/agentctl** - Main entry point with gohelp-luar documentation
- `main.go`: Command routing, help system with topic pages (setup, config, tools, prompt, interfaces, memory)

### Agent Folder Structure

```
agent-folder/
  config/
    agent.toml          # provider, model, tools, interfaces, memory, debug_calls config
    identities.toml     # Identity linking (multiple contacts per user)
  prompts/
    default             # [>role] static, [>>role] input sections
  tools/
    *.toml              # Tool definitions (example.toml excluded from auto-load)
  .env                  # API keys (OPENAI_API_KEY, OPENROUTER_API_KEY, TELEGRAM_BOT_TOKEN)
  .data/
    agent.sock          # Unix socket for CLI interface
    sessions/
      {userID}/
        {sessionID}.jsonl  # Per-session conversation history
        .last_session      # Most recent session per interface
    debug-calls/        # Full request/response JSON (if debug_calls = true)
      2026-*.json       # Timestamped debug files
```

**Tool Loading:** `tools = []` (auto-discovery) recursively finds all `.toml` files including subdirectories. `tools = ["name"]` only loads from top-level `tools/` directory. Subdirectories work only in auto-discovery mode.

**Config Paths:**
- Agent config: `config/agent.toml`
- Identities: `config/identities.toml`
- Prompt: `prompts/default`
- Registry: `~/.local/share/agentctl/agents`

### Prompt File Format

- `[>role]` - Static sections (directives + variable substitution at parse time)
- `[>>role]` - Input section (directives processed, variables preserved for runtime, {{input}} replaced per message)
- `{{file:path}}` - Load file content (supports relative, ~/, absolute paths)
- `{{exec:command}}` - Execute shell command and inject stdout (supports scripts, PATH commands, pipes)
- `{{var}}` - Variable placeholders (no colon = variable, not directive)
- Directives are recursive (10-level depth limit): files loaded via {{file:}} can contain {{exec:}} directives

**Directive Processing:**
- Happens at parse time (once per request due to hot-reload)
- Detection rule: `{{...}}` with `:` = directive, without `:` = variable
- `{{exec:}}` runs commands via `sh -c` from agent folder (handles ./scripts, PATH commands, pipes)
- Unknown directives (e.g., `{{unknown:path}}`) cause parse errors (fail fast)
- Directives can appear inline: `Current time: {{exec:date}} - processing...`

**Example:**
```
[>system]
Context: {{file:./docs/context.md}}
Timestamp: {{exec:date -Iseconds}}
Agent: {{exec:agentctl getagent}}
User: {{username}}

[>>user]
{{input}}
```

### Tool File Format (TOML)

```toml
command = "curl wttr.in/{{location}}"
description = """
Tool description here
"""

[location]
description = "City name"
type = "string"
required = true
enabled = true    # Optional: false to hide from AI (default: true)
return = "value"  # Optional: override value with directive support (hides from AI)
```

**Parameter Fields:**
- `description` - Parameter description (shown to AI)
- `type` - Data type: string, number, boolean, etc.
- `required` - true/false - whether AI must provide this
- `enabled` - true/false - hide from AI schema (default: true)
- `return` - Override value, supports directives (automatically hides from AI)

**Return Field (Parameter Overrides):**

Use `return` to provide values without AI control:

```toml
[api_key]
description = "API authentication key"
type = "string"
required = true
return = "{{file:.env.API_KEY}}"  # Load from file, hidden from AI

[timestamp]
description = "Current timestamp"
type = "string"
return = "{{exec:./scripts/timestamp.sh}}"  # Dynamic from script

[env]
description = "Environment name"
type = "string"
return = "production"  # Literal hardcoded value
```

**Directives in Return Values:**
- `{{file:path}}` - Load file content (relative to agent folder, or ~/home, or absolute)
- `{{exec:command}}` - Execute shell command (scripts, PATH commands, pipes) from agent folder
- Escape with backslash: `return = '\{{literal}}'` (TOML literal string, single backslash)
- Or double backslash in regular strings: `return = "\\{{literal}}"` (TOML escaping)

**Behavior:**
- Parameters with `return != ""` are hidden from AI (blackbox)
- Return values override AI-provided arguments during execution
- `enabled=false` is hard disable (parameter not used even if `return` is set)
- Directives processed at tool execution time (supports hot-reload)

**Important:** Parameter sections are directly `[paramName]`, NOT nested under `[parameters]`. Tool parser extracts all sections except `command` and `description` as parameters.

### Agentic Loop

1. Build messages: static prompt sections + history + input (with {{input}} substituted)
2. Send to provider (openai/openrouter) with tool definitions
3. If response has tool_calls:
   - Execute each tool via shell with variable substitution
   - Append tool results as user messages
   - Loop back to step 2
4. Return final text response

### Session Management & Identity Linking

Sessions are organized by user ID and session ID, with support for linking multiple contacts to a single identity:

**Storage:**
- Path: `.data/sessions/{userID}/{sessionID}.jsonl`
- User ID: System username (CLI) or platform user ID (Telegram)
- Session ID: `YYYYMMDD_HHMMSS_<6-hex>` format
- `.last_session`: Tracks most recent session per interface (plain text `interface=sessionID`)

**Identity Linking (`config/identities.toml`):**
```toml
[[identity]]
id = "alice"
contacts = ["cli:alice", "telegram:123456789"]
```

Contact format: `interface:platformID` (e.g., `cli:luar`, `telegram:12345678`)

**Migration System:**
- **Startup migration**: `MigrateOnStartup()` called in `run.go` after `.data` directory creation
  - Bulk processes all identities from identities.toml
  - Moves sessions from unlinked folders (e.g., `telegram-12345678`) to identity folders (e.g., `alice`)
  - Merges `.last_session` files by parsing session ID timestamps
- **Lazy migration**: `MigrateContact()` called from `Resolve()` on each message
  - Single `os.Stat()` check if no migration needed (sub-millisecond overhead)
  - Migrates specific contact on first message after adding to identity
  - No daemon restart required for identity changes to take effect
- Both migrations are idempotent and safe to run multiple times

**Session Resolution:**
- `Resolve(store, agentFolder, iface, contactID, displayName)` - Auto resolution (used by ConversationService)
- `ResolveExplicit(store, agentFolder, userID, sessionID, iface)` - Explicit resolution (used by CLI for --user/--session)
- Contact logging in `.data/contacts` file (append-only with deduplication)
- `memory.max_messages` in agent.toml controls history limit (0 = unlimited)

**Auto-Title Generation:**
- Automatically generates short session titles (max 5 words) after first user-assistant exchange
- Runs asynchronously in background goroutine (doesn't block user response)
- Uses same LLM provider/model as agent with fixed prompt
- Title stored in session metadata (first line of JSONL file with `type="meta"`)
- No-op if title already exists (prevents re-generation)

**Thread Safety (Critical):**
- All session file operations (Load/Save/GetMeta/SetMeta) are protected by per-session mutexes
- Prevents race condition where concurrent Save() during async SetMeta() could lose messages
- Each session gets its own mutex via `sync.Map` (zero contention between different sessions)
- Title generation passes messages in-memory (not re-reading from disk) to avoid file I/O race

### Message Flow

**Normal Flow (Telegram, CLI auto-resolution):**
```
Interface → MessageHandler.HandleMessage(iface, contactID, displayName, content)
              ↓
         ConversationService
              ↓
         session.Resolve() → Load config/tools/prompt → session.Load()
              ↓
         agent.Run() → provider.SendMessages() → tool execution
              ↓
         session.Save() → session.GenerateTitle() (async)
              ↓
         return response
```

**Explicit Flow (CLI --user/--session flags):**
```
CLI → session.ResolveExplicit() → MessageHandler.HandleExplicitMessage(userID, sessionID, iface, content)
                                      ↓
                                 ConversationService
                                      ↓
                                 (skip resolution) → Load config/tools/prompt → session.Load()
                                      ↓
                                 agent.Run() → provider.SendMessages() → tool execution
                                      ↓
                                 session.Save() → session.GenerateTitle() (async)
                                      ↓
                                 return response
```

### Help System

Uses `github.com/DeprecatedLuar/gohelp-luar` for formatted CLI help:
- Root page lists all commands with examples
- Topic pages: setup, config, tools, prompt, interfaces, memory
- Routing: `agentctl help <topic>`, `agentctl help --all`
- Fuzzy matching for typos (e.g., "memori" suggests "memory")
- All help content in `cmd/agentctl/main.go` printHelp() function

### Runtime Config Hot-Reload

Config, tools, and prompt files are loaded on **every request**, not at daemon startup:
- Edit `agent.toml`, any tool file, or `prompt` while daemon is running → changes take effect immediately
- Malformed configs return errors to the caller but daemon stays running
- ConversationService loads fresh config/tools/prompt on each HandleMessage() call (no caching)
- Startup validation still happens for fail-fast on boot errors

This enables zero-downtime config changes and experimentation.

### Debug Infrastructure

Two complementary debugging mechanisms:

1. **--debug flag**: Enhanced slog output with message previews, tool names, execution details
   ```bash
   ./agentctl run _test-agent --debug
   ```

2. **debug_calls = true** (agent.toml): Writes full request/response JSON to `.data/debug-calls/`
   - Timestamped files: `2026-06-12T15-04-05-request.json`, `2026-06-12T15-04-05-response.json`
   - Contains: complete messages array, tool definitions, provider/model info
   - Auto-cleanup: keeps last 10 files
   - Use `cat .data/debug-calls/*.json | jq` to inspect what was sent to AI

**internal/debug package**: LogRequest(), LogResponse(), LogToolExecution() for structured logging

### Key Design Decisions

- **Hexagonal architecture** - Clear separation: I/O adapters → orchestration → domain logic
- **Ports & Adapters** - MessageHandler (input port), SessionStore (output port), Dispatcher (future output port)
- **No CLI framework** - Simple stdlib arg parsing (KISS principle), gohelp-luar for documentation only
- **Modular providers** - Each provider in separate file (openai.go, openrouter.go) for single responsibility
- **Daemon architecture** - Single process, config-driven interfaces (agent.toml: `interfaces = ["cli", "telegram"]`)
- **Unix socket for CLI** - JSON protocol, session isolation via user ID + session ID
- **Shell-based tools** - Execute via `sh -c` with {{var}} substitution
- **JSONL not SQLite** - Simple append-only files for sessions (easier debugging, no DB overhead)
- **Flat package structure** - internal/interfaces/ not internal/interfaces/cli/ (keep simple until growth demands nesting)
- **Runtime config loading** - Hot-reload on every request, daemon survives malformed configs
- **Identity-based sessions** - Support multiple contacts per user (CLI + Telegram unified)
- **Dual migration strategy** - Startup bulk migration + lazy per-message migration (no restart needed)
- **XDG agent registry** - Name-based agent resolution via `~/.local/share/agentctl/agents`
- **Async title generation** - LLM-generated titles don't block user responses, run in background
- **Per-session mutexes** - Prevents race conditions in concurrent file operations without global locking

### Adding New Providers

Each provider is self-contained in `internal/providers/llm/{name}.go`:
1. Implement `Provider` interface (SendMessages method)
2. Load API key from `.env` using godotenv
3. Convert between agentctl Message format and provider's SDK format
4. Add provider name constant to `provider.go` and update factory function
5. Factory routing supports both named providers and HTTP/HTTPS URLs (custom endpoints)

### Adding New Interfaces

Each interface implements the `Interface` contract in `internal/interfaces/interface.go`:
1. Create new file in `internal/interfaces/` (e.g., `discord.go`)
2. Implement `Start(ctx context.Context) error`
3. Receive input, call `handler.HandleMessage(iface, contactID, displayName, content)`
4. Keep interface as thin I/O adapter - no session logic, no resolution
5. Add interface name to factory in `internal/commands/run.go`
6. Update agent.toml template and identities.toml contact format if needed

**Interface responsibility**: Pure I/O transport. Extract platform-specific user ID, call MessageHandler, send response back. Nothing more.

**Exception**: CLI interface keeps minimal `session.ResolveExplicit` for `--user/--session` flags (documented architectural exception for explicit resolution feature).
