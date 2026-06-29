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

**CRITICAL:** Always use the existing `_test-agent` folder for testing. DO NOT create new test agents (e.g., `test-agent`, `my-test-agent`, etc.). The `_test-agent` folder is pre-configured with free models and should be reused for all testing purposes.

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

The `_test-agent` folder uses `openrouter/free` - modify its config if testing requires specific model features, then revert when done.

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
│   internal.Orchestrator             │
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
               └─→ OutboundDispatcher (port)

┌─────────────────────────────────────┐
│   OUTPUT ADAPTERS                   │
│   - JSONLStore (session persistence)│
│   - LLM providers (OpenAI, etc)     │
└─────────────────────────────────────┘
```

### Component Boundaries

**1. internal/config** - All file loading (agent.toml, tools/*.toml, prompt file)
- `agent.go`: AgentConfig with nested sections: [agent], [access], [memory], [audio]
  - AgentSection: provider, model, tools, logging (false | true | "debug")
  - AccessConfig: allow_by_default, interfaces
  - MemoryConfig: max_messages
  - AudioConfig: optional audio provider settings
- `tool.go`: ToolConfig with dynamic parameter sections (all TOML sections except command/description are parameters)
- `prompt.go`: Custom format parser with `[>role]` static sections and `[>>role]` input section
  - Uses resolution.Process() for template resolution (directives + variables)
  - Validates syntax on startup via resolution.ValidateSyntax()

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

**5. internal/** - Application layer (ports & orchestration)
- `ports.go`: Input port interfaces (MessageHandler, OutboundDispatcher, Interface)
  - MessageHandler: Application boundary with three methods:
    - HandleMessage() - auto contact resolution
    - HandleExplicitMessage() - explicit user/session (CLI --user/--session)
    - HandleMessageWithOptions() - delivery options (--channel, --tools)
  - MessageOptions: struct with Channels/ChannelsInject/Tools fields
  - OutboundDispatcher: Cross-interface message delivery abstraction
  - Interface: Adapter contract for CLI/Telegram/etc
- `orchestration.go`: Orchestrator implementation
  - Implements MessageHandler interface
  - HandleMessage(iface, contactID, displayName, content) - auto resolution
  - HandleExplicitMessage(userID, sessionID, iface, content) - explicit resolution (CLI only)
  - HandleMessageWithOptions(opts MessageOptions) - delivery + tool whitelisting
  - Pure orchestration: calls session API → config API → agent → session API
  - No domain logic, only sequencing port calls
  - Loads config/tools/prompt fresh on each request (hot-reload)
  - Triggers async title generation for new sessions
  - deliverToChannels() resolves and dispatches to multiple interfaces

**6. internal/interfaces** - I/O adapters (pure transport layer)
- Organized by interface with nested packages for modularity:
  - `cli/` (package cli) - Unix socket interface
    - `interface.go`: Socket listener, lifecycle, message routing (normal/explicit/options flows)
    - `commands.go`: Command handling + CLI-specific formatting (numbered lists, `/sessions attach <number>`)
  - `telegram/` (package telegram) - Telegram bot interface
    - `interface.go`: Bot polling, typing indicators, voice transcription, lifecycle
    - `commands.go`: Command handling + Telegram-specific formatting (plain lists)
  - `dispatch.go` (package interfaces) - Outbound message dispatcher
    - Sender interface: InterfaceName() and Send(platformID, content)
    - OutboundDispatcher: registers interface senders, routes Send() calls
    - Used for --channel and --channel-inject delivery

**Interface responsibilities:**
- Each interface detects system commands via syscommands.Parse()
- Calls syscommands helpers (NewSession/ListSessions/SwitchSession) for business logic
- Formats CommandResult output for their specific UX (CLI: numbered, Telegram: plain)
- Implements Sender interface for cross-interface message delivery
- CLI exception: Retains minimal session.ResolveExplicit for `--user/--session` flags
- **Key principle**: Interfaces own their UX - detect commands, call helpers, format appropriately

**7. internal/syscommands** - System command framework
- `types.go`: Command, CommandResult, SessionInfo types
- `handler.go`: Command parsing and business logic
  - Parse() detects "/" prefix, returns Command{Name, Args}
  - Helper functions: NewSession(), ListSessions(), SwitchSession()
  - No routing logic - interfaces call helpers directly
- Commands: `/new` (create session), `/sessions` (list), `/sessions attach <arg>` (switch by number or ID)
- Design: Interfaces handle detection/formatting, syscommands provides business logic
- Returns structured data (CommandResult), interfaces format for their UX

**8. internal/session** - Session management domain (port implementation)
- `session.go`: ResolvedSession type, NewSessionID() with `YYYYMMDD_HHMMSS_<6-hex>` format, SessionMeta with title
- `identity.go`: Merged identities and contacts in `.data/contacts.toml` - both [[identity]] and [[contact]] sections in single file
  - ResolveUser() for linking contacts to named identities
  - EnsureContact() for auto-logging contacts with deduplication
  - LoadIdentities() and saveIdentitiesFile() preserve both sections
  - WarnDuplicateContacts() logs startup warnings for duplicate interface contacts
- `jsonl.go`: JSONL storage implementation with per-session mutex for race protection
- `resolve.go`: Session resolution functions
  - Resolve() for auto user/session resolution
  - ResolveExplicit() for explicit CLI flags (--user/--session)
  - ResolveChannel() parses "user@interface" or "interface" for channel delivery
  - LookupPlatformID() reverse lookup from identity ID to platform ID
- `inject.go`: InjectTurn() writes turn to session without running agent (used by inject command and --channel-inject)
- `migrate.go`: MigrateOnStartup() for bulk migration, MigrateContact() for lazy per-message migration
- `title.go`: GenerateTitle() for async LLM-based session title generation
- `store.go`: SessionStore interface + domain types (Message, SessionMeta)
  - SessionStore kept in session package to avoid circular imports
  - Domain types remain with their interface
- Storage: `{agent_folder}/.data/sessions/{userID}/{sessionID}.jsonl`
- Respects `memory.max_messages` limit from agent.toml
- **Thread Safety**: Per-session mutexes prevent race conditions between concurrent Save/Load/SetMeta operations

**9. internal/commands** - CLI command handlers
- `init.go`: Scaffold agent folder from embedded templates (idempotent - skips existing files)
- `run.go`: Start daemon, load config/tools/prompt, spawn interfaces, run migration, create dispatcher
- `chat.go`: Send message to Unix socket
  - Flags: --agent/-a, --user/-u, --session/-s, --channel, --channel-inject, --tools, --debug
  - Supports comma-separated lists for channel and tool flags
- `inject.go`: Manual session injection without running agent (--role, --session, --agent)

**10. internal/resolution** - Template resolution with directives and variables
- `resolution.go`: Process() main pipeline (directives → variables), ValidateSyntax() for startup checks
- `directives.go`: processDirectives() with recursive expansion (10-level depth limit), binary file detection
  - `findMatchingCloseBrace()` properly parses nested `{{}}` for variable substitution in directive paths
  - Validation mode detection: skips variable substitution when UserID is empty (daemon startup)
- `variables.go`: substituteVariables() for {{var}} and {{$var}} placeholders
  - `findMatchingCloseBraceVar()` handles nested braces in variable contexts
- `context.go`: Context struct with runtime info (agent path, user, session, model, timestamp)
- Two-phase processing:
  1. Directives ({{file:path}}, {{exec:command}}) - loads/executes content
     - Variables in directive arguments are substituted first (enables `{{file:path/{{$user}}/file.md}}`)
  2. Variables ({{var}}, {{$var}}) - substitutes runtime values
- Used by prompt parser and tool parameter return values
- System variables: {{$agent}}, {{$user}}, {{$username}}, {{$session}}, {{$interface}}, {{$timestamp}}, {{$date}}, {{$model}}, {{$provider}}
- User variables: {{var}} (future use, currently empty)
- Supports relative paths (agent folder), ~/ expansion, absolute paths

**11. internal/debug** - Debug utilities for inspecting AI requests
- LogRequest/LogResponse/LogToolExecution functions for structured debug logging
- Writes timestamped JSON files to `.data/debug-calls/` when `debug_calls = true`
- Auto-cleanup to prevent disk bloat (keeps last 10 files)

**12. internal/registry** - XDG agent registry for name-based resolution
- Registry file: `~/.local/share/agentctl/agents` (one path per line)
- Register() adds agent to registry, Resolve() looks up by name or path
- Auto-cleanup of dead entries, handles multiple-match ambiguity

**13. internal/templates** - Embedded template files (files/ directory)
- Pure data package, no logic
- Template structure: `files/config/`, `files/prompts/`, `files/tools/`

**14. cmd/agentctl** - Main entry point with gohelp-luar documentation
- `main.go`: Command routing, help system with topic pages (setup, config, tools, prompt, interfaces, memory)

### Agent Folder Structure

```
agent-folder/
  config/
    agent.toml          # [agent], [access], [memory], [audio] sections
  prompts/
    default             # [>role] static, [>>role] input sections
  tools/
    *.toml              # Tool definitions (example.toml excluded from auto-load)
  .env                  # API keys (OPENAI_API_KEY, OPENROUTER_API_KEY, TELEGRAM_BOT_TOKEN)
  .data/
    contacts.toml       # Merged file: [[identity]] + [[contact]] with allowed field
    agent.sock          # Unix socket for CLI interface
    sessions/
      {userID}/
        {sessionID}.jsonl  # Per-session conversation history
        .last_session      # Most recent session per interface
    debug-calls/        # Full request/response JSON (if logging = "debug")
      2026-*.json       # Timestamped debug files
```

**Tool Loading:** `tools = []` (auto-discovery) recursively finds all `.toml` files including subdirectories. `tools = ["name"]` only loads from top-level `tools/` directory. Subdirectories work only in auto-discovery mode.

**Config Paths:**
- Agent config: `config/agent.toml`
- Identities & Contacts: `.data/contacts.toml` (merged file, created on first message)
- Prompt: `prompts/default.prompt`
- Registry: `~/.local/share/agentctl/agents`

### Prompt File Format

- `[>role]` - Static sections (directives + variable substitution at parse time)
- `[>>role]` - Input section (directives processed, variables preserved for runtime, {{input}} replaced per message)
- `{{file:path}}` - Load file content (supports relative, ~/, absolute paths)
- `{{exec:command}}` - Execute shell command and inject stdout (supports scripts, PATH commands, pipes)
- `{{var}}` - User variable placeholders (no colon = variable, not directive)
- `{{$var}}` - System variable placeholders ($ prefix for built-in variables)
- Directives are recursive (10-level depth limit): files loaded via {{file:}} can contain {{exec:}} directives

**Resolution Pipeline (Two-Phase):**
1. **Directives** - Happens first, loads/executes external content
   - `{{file:path}}` - loads file content (with binary file detection)
   - `{{exec:command}}` - runs shell command via `sh -c` from agent folder
   - Recursive expansion up to 10 levels deep
   - Unknown directives (e.g., `{{unknown:path}}`) cause parse errors (fail fast)
   - Directives can appear inline: `Current time: {{exec:date}} - processing...`
   - **Nested directives** - Variables in directive paths are substituted before processing:
     - `{{file:path/{{$user}}/notes.md}}` - `{{$user}}` resolves first, then file loads
     - `{{exec:scripts/{{$date}}/run.sh}}` - enables per-user/date/context file organization
     - During validation mode (daemon startup), directives with variables are preserved as-is
2. **Variables** - Happens after directives, substitutes runtime values
   - Detection rule: `{{...}}` with `:` = directive, without `:` = variable
   - System variables take precedence over user variables
   - Built-in system variables ($ prefix): `{{$agent}}`, `{{$user}}`, `{{$username}}`, `{{$session}}`, `{{$interface}}`, `{{$timestamp}}`, `{{$date}}`, `{{$model}}`, `{{$provider}}`
   - User variables (no prefix): `{{var}}` (future use, currently empty)

**Example:**
```
[>system]
Context: {{file:./docs/context.md}}
Current time: {{$timestamp}}
Agent: {{$agent}}
User: {{$username}} (ID: {{$user}})
Interface: {{$interface}}
Model: {{$model}} ({{$provider}})

# Nested directive example - per-user memory
User Memory: {{file:.data/memory/{{$user}}/core.md}}

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

**Identity Linking (`.data/contacts.toml`):**

Merged file with two sections:
```toml
# ============================================
# IDENTITIES (user-edited)
# Link multiple contacts to a single identity
# ============================================
[[identity]]
id = "alice"
contacts = ["cli:alice", "telegram:123456789"]
allowed = true  # Optional: overrides contact/default policy

# ============================================
# CONTACTS (auto-generated - do not edit)
# All contacts that have sent messages
# ============================================
[[contact]]
interface = "telegram"
id = "123456789"
display_name = "Alice"
first_seen = 2026-06-17T15:04:05Z
allowed = true  # Auto-set from allow_by_default policy
```

Contact format: `interface:platformID` (e.g., `cli:luar`, `telegram:12345678`)

**File Creation:**
- Created by `init` command with default user identity (`[[identity]]` with system username)
- Auto-updated when first contact sends message (adds `[[contact]]` entry)
- `saveIdentitiesFile()` preserves both sections with header comments

**Migration System:**
- **Startup migration**: `MigrateOnStartup()` called in `run.go` after `.data` directory creation
  - Bulk processes all identities from contacts.toml
  - Moves sessions from unlinked folders (e.g., `telegram-12345678`) to identity folders (e.g., `alice`)
  - Merges `.last_session` files by parsing session ID timestamps
- **Lazy migration**: `MigrateContact()` called from `Resolve()` on each message
  - Single `os.Stat()` check if no migration needed (sub-millisecond overhead)
  - Migrates specific contact on first message after adding to identity
  - No daemon restart required for identity changes to take effect
- Both migrations are idempotent and safe to run multiple times

**Session Resolution:**
- `Resolve(store, agentFolder, iface, contactID, displayName)` - Auto resolution (used by Orchestrator)
- `ResolveExplicit(store, agentFolder, userID, sessionID, iface)` - Explicit resolution (used by CLI for --user/--session)
- `ResolveChannel(store, agentFolder, channelStr, currentUserID)` - Channel resolution for delivery
  - Parses "user@interface" or "interface" syntax
  - Returns userID, platformID, sessionID for delivery
  - Creates new session if none exists (consistent with Resolve)
- `LookupPlatformID(agentFolder, userID, iface)` - Reverse lookup from identity ID to platform ID
  - First match wins when identity has multiple contacts for same interface
- `CheckAccess(agentFolder, iface, platformID)` - Access control check
  - Three-tier precedence: Identity.allowed > Contact.allowed > allow_by_default
  - Returns (bool, error) - false means denial, error means file I/O issue
  - Called by all message handlers before agent execution
- Contact auto-logged to `.data/contacts.toml` (deduplication by interface+id)
- New contacts auto-assigned `allowed` field from `allow_by_default` policy
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

### System Commands

Built-in commands for session management (prefixed with `/`):

**`/new`** - Create new session
- Generates new session ID with auto-switch
- Returns model, provider, memory info
- Works on all interfaces (CLI, Telegram)

**`/sessions`** - List all sessions
- Shows sessions in reverse chronological order (newest first)
- Marks active session with `[active]`
- CLI: numbered list, Telegram: plain list
- Example output:
  ```
  Sessions:
  1. Planning feature X (2026-06-17) [active]
  2. Bug investigation (2026-06-16)
  ```

**`/sessions attach <arg>`** - Switch session
- CLI: accepts number (from list) or literal session ID
  - `/sessions attach 2` - switch to session #2
  - `/sessions attach 20260615_143022_a1b2c3` - switch by ID
- Telegram: only supports literal session ID (no numbering)
- Future: Telegram inline keyboard buttons for one-tap switching

**Command Architecture:**
- Detected by interfaces before calling orchestrator
- Parsed by `syscommands.Parse()` - returns `Command{Name, Args}`
- Interfaces call syscommands helpers, format output for their UX
- No agent execution, no session history - immediate response

### Message Flow

**Command Flow (System Commands: /new, /sessions, /sessions attach):**
```
Interface (CLI/Telegram) → syscommands.Parse(content)
              ↓
         Detect "/" prefix → Command{Name, Args}
              ↓
         Interface.handleCommand()
              ↓
         Call syscommands helpers (NewSession/ListSessions/SwitchSession)
              ↓
         Format CommandResult for interface UX
              ↓
         return formatted response (no orchestrator, no agent execution)
```

**Normal Flow (Telegram, CLI auto-resolution):**
```
Interface → internal.MessageHandler.HandleMessage(iface, contactID, displayName, content)
              ↓
         internal.Orchestrator
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
CLI → session.ResolveExplicit() → internal.MessageHandler.HandleExplicitMessage(userID, sessionID, iface, content)
                                      ↓
                                 internal.Orchestrator
                                      ↓
                                 (skip resolution) → Load config/tools/prompt → session.Load()
                                      ↓
                                 agent.Run() → provider.SendMessages() → tool execution
                                      ↓
                                 session.Save() → session.GenerateTitle() (async)
                                      ↓
                                 return response
```

**Delivery Flow (--channel/--channel-inject/--tools flags):**
```
CLI → internal.MessageHandler.HandleMessageWithOptions(MessageOptions)
         ↓
    internal.Orchestrator
         ↓
    session.Resolve() or ResolveExplicit() → filterTools() if --tools
         ↓
    agent.Run() with whitelisted tools
         ↓
    session.Save()
         ↓
    deliverToChannels():
      - Parse each channel string (user@interface or interface)
      - session.ResolveChannel() → userID, platformID, sessionID
      - dispatcher.Send(interface, platformID, response)
      - If --channel-inject: session.InjectTurn() into target session
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
- Orchestrator loads fresh config/tools/prompt on each HandleMessage() call (no caching)
- Startup validation still happens for fail-fast on boot errors

This enables zero-downtime config changes and experimentation.

### Debug Infrastructure

Two complementary debugging mechanisms:

1. **--debug flag**: Enhanced slog output with message previews, tool names, execution details
   ```bash
   ./agentctl run _test-agent --debug
   ```

2. **logging = "debug"** (agent.toml): Writes full request/response JSON to `.data/debug-calls/`
   - Timestamped files: `2026-06-12T15-04-05-request.json`, `2026-06-12T15-04-05-response.json`
   - Contains: complete messages array, tool definitions, provider/model info
   - Auto-cleanup: keeps last 10 files
   - Use `cat .data/debug-calls/*.json | jq` to inspect what was sent to AI

**Logging levels in agent.toml:**
- `logging = false` - No file logging (stdout only)
- `logging = true` - Basic operational logs (default)
- `logging = "debug"` - Full debug logging + auto-enables debug_calls JSON dumps

**internal/debug package**: LogRequest(), LogResponse(), LogToolExecution() for structured logging

### Key Design Decisions

- **Hexagonal architecture** - Clear separation: I/O adapters → orchestration → domain logic
- **Ports at package root** - All input ports in `internal/ports.go` (MessageHandler, OutboundDispatcher, Interface)
- **Orchestrator at top-level** - `internal/orchestration.go` for shorter imports and architectural clarity
- **SessionStore in domain** - Kept in `session/` package to avoid circular imports (domain types stay with interface)
- **Merged identities & contacts** - Single `.data/contacts.toml` file with both user-edited identities and auto-generated contacts
- **Access control** - Three-tier precedence (Identity.allowed > Contact.allowed > allow_by_default) enforced at orchestrator level
- **Outbound dispatcher** - Cross-interface message delivery via Sender interface registration
- **Command handling in interfaces** - Each interface detects commands, calls syscommands helpers, formats output for their UX (no centralized routing)
- **No CLI framework** - Simple stdlib arg parsing (KISS principle), gohelp-luar for documentation only
- **Modular providers** - Each provider in separate file (openai.go, openrouter.go) for single responsibility
- **Daemon architecture** - Single process, config-driven interfaces (access.toml: `interfaces = ["cli", "telegram"]`)
- **Unix socket for CLI** - JSON protocol, session isolation via user ID + session ID
- **Shell-based tools** - Execute via `sh -c` with {{var}} substitution
- **JSONL not SQLite** - Simple append-only files for sessions (easier debugging, no DB overhead)
- **Nested interface packages** - Each interface (cli/, telegram/) is a separate package with interface.go + commands.go for modularity
- **Runtime config loading** - Hot-reload on every request, daemon survives malformed configs
- **Identity-based sessions** - Support multiple contacts per user (CLI + Telegram unified)
- **Dual migration strategy** - Startup bulk migration + lazy per-message migration (no restart needed)
- **XDG agent registry** - Name-based agent resolution via `~/.local/share/agentctl/agents`
- **Async title generation** - LLM-generated titles don't block user responses, run in background
- **Per-session mutexes** - Prevents race conditions in concurrent file operations without global locking
- **Channel delivery** - ResolveChannel receives identity ID (not raw username) for consistent semantics
- **Tool whitelisting** - Runtime --tools flag filters available tools without config changes
- **Two-phase template resolution** - Directives first ({{file:}}, {{exec:}}), then variables ({{var}}, {{$var}}) for clean separation
- **System variables** - Built-in runtime context ({{$agent}}, {{$user}}, {{$timestamp}}, etc.) with $ prefix to distinguish from user vars
- **Unified logging config** - Single `logging` field (false | true | "debug") replaces separate logging + debug_calls fields
- **Semantic config sections** - Grouped settings under [agent], [access], [memory], [audio] for clarity

### Adding New Providers

Each provider is self-contained in `internal/providers/llm/{name}.go`:
1. Implement `Provider` interface (SendMessages method)
2. Load API key from `.env` using godotenv
3. Convert between agentctl Message format and provider's SDK format
4. Add provider name constant to `provider.go` and update factory function
5. Factory routing supports both named providers and HTTP/HTTPS URLs (custom endpoints)

### Adding New Interfaces

Each interface is a nested package under `internal/interfaces/`:

**Structure:**
```
internal/interfaces/
  newinterface/          # package newinterface
    interface.go         # Core interface logic + lifecycle
    commands.go          # Command handling + UX formatting
```

**Implementation steps:**
1. Create `internal/interfaces/newinterface/` directory
2. Create `interface.go` with package name matching directory
3. Import `github.com/DeprecatedLuar/agentctl/internal` for port interfaces
4. Implement `internal.Interface` contract: `Start(ctx context.Context) error`
5. Implement `Sender` interface: `InterfaceName() string` and `Send(platformID, content string) error`
6. Accept `internal.MessageHandler` in constructor (e.g., `NewNewInterface()`)
7. In message handler:
   - Extract platform-specific user ID, display name, username
   - Detect system commands via `syscommands.Parse(content)`
   - If command: call syscommands helpers, format output, return (skip orchestrator)
   - If message: call `handler.HandleMessage(iface, contactID, displayName, username, content)`
8. Create `commands.go` for command handling and UX-specific formatting
9. Add interface to factory in `internal/commands/run.go` with imports:
   ```go
   import "github.com/DeprecatedLuar/agentctl/internal/interfaces/newinterface"

   case "newinterface":
       iface := newinterface.NewNewInterface(absPath, orch, store, lg, verbose)
       dispatcher.Register(iface)
       interfaceInstances = append(interfaceInstances, iface)
   ```
10. Update agent.toml template and contacts.toml contact format if needed

**Interface responsibilities:**
- Pure I/O transport - extract platform IDs, call MessageHandler, send response
- Detect and handle system commands locally (no orchestrator involvement)
- Format command output for interface-specific UX
- Keep thin - no session logic, no resolution (except CLI's documented exception)

**File organization:**
- `interface.go`: ~250-350 lines - socket/polling/lifecycle, message routing, Sender implementation
- `commands.go`: ~80-100 lines - handleCommand(), format functions for /new, /sessions, etc.
- Minimal split prevents over-fragmentation while keeping concerns separated
