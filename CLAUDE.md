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

### Daemon + Interface Model

The system runs as a daemon that loads multiple interfaces based on agent config. All interfaces share the same agent runtime and memory:

```
agentctl run → daemon starts → loads interfaces from config.Interfaces
                               ├─ CLI → Unix socket at .data/agent.sock
                               └─ Telegram → long polling (if configured)

Both → Runner.Run() → agent.Run() → provider.SendMessages()
```

### Component Boundaries

1. **internal/config** - All file loading (agent.toml, tools/*.toml, prompt file)
   - `agent.go`: AgentConfig with provider, model, tools, interfaces, memory settings
   - `tool.go`: ToolConfig with dynamic parameter sections (all TOML sections except command/description are parameters)
   - `prompt.go`: Custom format with `[>role]` static sections and `[>>role]` input section

2. **internal/providers/llm** - AI provider abstraction (modular, one file per provider)
   - `provider.go`: Interface contract + routing factory
   - `openai.go`: OpenAI provider (self-contained)
   - `openrouter.go`: OpenRouter provider (self-contained, separate for single responsibility)
   - `generic.go`: OpenAI-compatible custom endpoints (Ollama, LM Studio, vLLM)
   - Each provider loads API keys from `.env` via godotenv, falls back to env vars

3. **internal/agent** - Agent runtime orchestration
   - `core.go`: Run() builds messages (static + history + input), calls provider in agentic loop
   - `tools.go`: Shell command execution with `{{var}}` substitution, captures stdout/stderr

4. **internal/shell** - Pure shell execution utility
   - `execute.go`: Execute(cmd, dir) returns stdout, stderr, exitCode, err
   - Used by both ExecuteTool and prompt directive processor
   - No logging or formatting - pure function

5. **internal/interfaces** - Interface abstraction for daemon
   - `interface.go`: Interface contract + Runner (wraps agent execution with runtime config loading and memory load/save)
   - `cli.go`: Unix socket listener, JSON protocol, session key defaults to "default"
   - `telegram.go`: Telegram bot with long polling, typing indicators, session key = user ID
   - **Important:** Runner loads config/tools/prompt fresh on each Run() call for hot-reload

6. **internal/memory** - JSONL persistence (per-session conversation history)
   - Load/Save functions for message history (role, content, timestamp)
   - Storage: `{agent_folder}/.data/memory/{sessionKey}.jsonl`
   - Respects `memory.max_messages` limit from agent.toml

7. **internal/commands** - CLI command handlers
   - `init.go`: Scaffold agent folder from embedded templates
   - `run.go`: Start daemon, load config/tools/prompt, spawn interfaces
   - `chat.go`: Send message to Unix socket (defaults to current dir, flags: --agent/-a, --session/-s)

8. **internal/debug** - Debug utilities for inspecting AI requests
   - LogRequest/LogResponse/LogToolExecution functions for structured debug logging
   - Writes timestamped JSON files to `.data/debug-calls/` when `debug_calls = true`
   - Auto-cleanup to prevent disk bloat (keeps last 10 files)

9. **internal/templates** - Embedded template files (files/ directory)
   - Pure data package, no logic

10. **cmd/agentctl** - Main entry point with gohelp-luar documentation
   - `main.go`: Command routing, help system with 7 topic pages (setup, config, tools, prompt, interfaces, memory)

### Agent Folder Structure

```
agent-folder/
  agent.toml          # provider, model, tools, interfaces, memory, debug_calls config
  prompt              # [>role] static, [>>role] input sections
  tools/
    *.toml            # Tool definitions (example.toml excluded from auto-load)
  .env                # API keys (OPENAI_API_KEY, OPENROUTER_API_KEY, TELEGRAM_BOT_TOKEN)
  .data/
    agent.sock        # Unix socket for CLI interface
    memory/
      {session}.jsonl # Per-session conversation history
    debug-calls/      # Full request/response JSON (if debug_calls = true)
      2026-*.json     # Timestamped debug files
```

**Tool Loading:** `tools = []` (auto-discovery) recursively finds all `.toml` files including subdirectories. `tools = ["name"]` only loads from top-level `tools/` directory. Subdirectories work only in auto-discovery mode.

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

### Memory System

Memory is session-isolated JSONL storage:
- Each session has a separate `.data/memory/{sessionKey}.jsonl` file
- CLI sessions default to "default" (override with `--session` flag)
- Telegram sessions use user ID as key (automatic per-user isolation)
- `memory.max_messages` in agent.toml controls history limit (0 = unlimited)
- Runner loads history before agent execution, saves user+assistant messages after

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
- Runner.Run() loads fresh config/tools/prompt on each call (no caching)
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

- **No CLI framework** - Simple stdlib arg parsing (KISS principle), gohelp-luar for documentation only
- **Modular providers** - Each provider in separate file (openai.go, openrouter.go) for single responsibility
- **Daemon architecture** - Single process, config-driven interfaces (agent.toml: `interfaces = ["cli", "telegram"]`)
- **Unix socket for CLI** - JSON protocol, session isolation via session keys
- **Shell-based tools** - Execute via `sh -c` with {{var}} substitution
- **JSONL not SQLite** - Simple append-only files for memory (easier debugging, no DB overhead)
- **Flat package structure** - internal/interfaces/ not internal/interfaces/cli/ (keep simple until growth demands nesting)
- **Runtime config loading** - Hot-reload on every request, daemon survives malformed configs

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
2. Implement `Start(ctx context.Context, runner *Runner) error`
3. Call `runner.Run(agent.Input{SessionKey, Content})` for each message
4. Add interface name to factory in `internal/commands/run.go`
5. Update agent.toml template if needed
