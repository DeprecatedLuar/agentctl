# agentctl

![Status](https://img.shields.io/badge/status-experimental-orange)
![License](https://img.shields.io/badge/license-MIT-blue)
![Go](https://img.shields.io/badge/go-1.21+-00ADD8)

Framework for composable, portable AI agents and harnesses. Agents and tools are folders: copy either to any machine with agentctl installed and it runs. Tools are bash scripts with variable substitution. Agents call tools. Tools execute shell commands. Commands can call other agents.

> [!CAUTION]
> **Experimental software** - Under active development. Build from source. Expect breaking changes between commits.

## Installation

**Requires:** Go 1.21+

```bash
# Clone and build from source
git clone https://github.com/DeprecatedLuar/agentctl
cd agentctl
go build -o agentctl ./cmd/agentctl

# Install to PATH
cp agentctl ~/bin/  # or /usr/local/bin
```

## Quick Start

```bash
# 1. Initialize agent
agentctl init my-agent
cd my-agent

# 2. Add API key to .env
echo "OPENROUTER_API_KEY=your_key_here" >> .env

# 3. Start daemon (Terminal 1)
agentctl serve

# 4. Chat with agent (Terminal 2)
agentctl chat "hello"
```

> [!TIP]
> Default config uses `openrouter/free` model. List free models with tool support: `agentctl models openrouter --free --tools`

## Commands

| Command | Description | Example |
|---------|-------------|---------|
| `init [path]` | Create agent folder with templates | `agentctl init my-agent` |
| `serve [path]` | Start daemon with configured gateways + routine scheduler (`run`/`up` still work as aliases) | `agentctl serve my-agent` |
| `chat` | Send message, one-shot and daemonless | `agentctl chat -m "analyze logs" -a my-agent` |
| `inject [role]` | Inject turn into session without running agent | `agentctl inject assistant -m "response" --session 20260614_abc` |
| `deliver <channel>` | Deliver literal text to channels without running the agent | `agentctl deliver telegram -m "Reminder: standup" --inject` |
| `toolrun <name>` | Execute tool manually with parameters | `agentctl toolrun weather --city=London` |
| `getagent` | Print current agent name from registry | `agentctl getagent` |
| `models [provider]` | List available models | `agentctl models openrouter --free --tools` |
| `help [topic]` | Show detailed help | `agentctl help tools` |

### Flags

**`serve` command:**
- `--debug` - Debug logging with message previews
- `--log` - Enable file logging (`.data/logs/`)
- `-v, --verbose` - Verbose logging

**`chat` command:**
- `-m, --message <text>` - Message to send (required; falls back to `AGENTCTL_MESSAGE`)
- `-a, --agent <path>` - Agent folder path (default: current dir)
- `-u, --user <id>` - User ID for session (default: system username)
- `-s, --session <id>` - Session ID (default: last session or new)
- `--deliver <list>` - Deliver response to channels (comma-separated: `telegram,cli`)
- `--inject [role]` - Also inject the delivered response into the target session(s); role is `assistant` (default), `user`, or `system`
- `--tools <list>` - Whitelist tools for this run (comma-separated, overrides agent.toml)
- `--debug` - Show session file path in output
- Env vars: `AGENTCTL_USER`, `AGENTCTL_SESSION`, `AGENTCTL_MESSAGE` (flags/args take priority)

**`inject` command:**
- `[role]` - Positional; role of the injected turn: `assistant` (default), `user`, or `system`
- `-m, --message <text>` - Content to inject (required; falls back to `AGENTCTL_MESSAGE`)
- `--session <id>` - Session ID to inject into (required)
- `-a, --agent <path>` - Agent folder path (default: current dir)

**`deliver` command:**
- `<channel[,channel...]>` - Positional; channels to deliver the literal message to (required)
- `-m, --message <text>` - Message to deliver (required; falls back to `AGENTCTL_MESSAGE`)
- `--inject [role]` - Also inject the message into the target session(s); role is `assistant` (default), `user`, or `system`
- `--note <text>` - System-role turn injected immediately before the delivered turn, never sent to the channel itself (requires `--inject`)
- `-u, --user <id>` - User ID for bare-gateway channel resolution
- `-a, --agent <path>` - Agent folder path (default: current dir)
- Env vars: `AGENTCTL_USER`, `AGENTCTL_MESSAGE` (flags/args take priority)

**`models` command:**
- `--free` - Show only free models
- `--tools` - Show only models with tool support
- `--stt` - Show speech-to-text models
- `--all` - Include all providers (default: popular + free only)

**`toolrun` command:**
- `-a, --agent <path>` - Agent folder path (default: current dir)
- `--<param>=<value>` - Tool parameters (e.g., `--city=London --units=metric`)

## Agent Folder Structure

```
my-agent/
├── config/
│   └── agent.toml          # Provider, model, gateways, memory config
├── prompts/
│   └── chat_template       # Message template with sections (chat_template.md also accepted)
├── tools/                  # Tool definitions (*.toml)
│   └── example.toml        # Example tool (excluded from auto-load)
├── routines/               # Scheduled commands (*.toml), fired by the serve daemon
│   └── example.toml        # Syntax reference (excluded from scheduling)
├── .prerun.sh              # Optional: runs before each agent execution (non-fatal)
├── .env                    # API keys (OPENAI_API_KEY, OPENROUTER_API_KEY, TELEGRAM_BOT_TOKEN)
└── .data/                  # Runtime data (auto-created)
    ├── contacts.toml       # Identities (user-edited) + contacts (auto-generated)
    ├── logs/               # Structured logs (if logging=true)
    └── sessions/           # Conversation history (per-user, per-session)
        └── {userID}/
            ├── {sessionID}.jsonl
            ├── .last_session
            └── last-call.json  # Last request+response, overwritten each call (if logging="debug")
```

## Configuration

### agent.toml

<details>
<summary><b>Full specification</b></summary>

```toml
[agent]
provider = "openrouter"              # "openai", "openrouter", or http/https URL for custom endpoints
model = "openrouter/free"            # Model name (provider-specific)
tools = []                           # Empty = auto-discover all .toml in tools/, or ["name1", "name2"]
logging = true                       # false | true | "debug" - controls logging output

[access]
allow_by_default = true              # Default access policy for new contacts
interfaces = ["cli"]                 # ["cli", "telegram"] or ["cli"] only

[memory]
max_messages = 100                   # Keep last N messages per session (0 = no persistence)

[audio]                              # Optional: speech-to-text for voice messages
provider = "whisper"                 # "whisper" or http://localhost:9000/v1
model = "whisper-1"

[environment]                        # Optional: non-secret config defaults
# KEY = "value"                      # Supports directives {{file:}}, {{exec:}} and variables
# API_ENDPOINT = "https://api.example.com"
# DATA_PATH = "{{$agentpath}}/data"
```

**Providers:**
- `openai` - Requires `OPENAI_API_KEY` in .env
- `openrouter` - Requires `OPENROUTER_API_KEY` in .env
- `http://...` or `https://...` - OpenAI-compatible custom endpoints (Ollama, LM Studio, vLLM)

**Logging:**
- `false` - Stdout only, no file logging
- `true` - Stdout + basic file logging to `.data/logs/` (default)
- `"debug"` - Stdout + file logging + full API request/response JSON written to `.data/sessions/{userID}/last-call.json` (overwritten on every call, per user)

**Tool loading:**
- `tools = []` - Auto-discover: recursively loads all `.toml` files in `tools/` (except `example.toml`)
- `tools = ["name1", "name2"]` - Explicit: loads only `tools/name1.toml` and `tools/name2.toml` (no subdirectory support)

**Access control:**
- `allow_by_default` - Default access policy for new contacts (true = auto-allow, false = require explicit approval)
- `interfaces` - Daemon-hosted gateways to enable (key name frozen on-disk): `["cli"]` for CLI only, or `["cli", "telegram"]` for both — note `cli` is never actually daemon-hosted, only `telegram` runs a persistent listener

**Environment variables:**
- Optional `[environment]` section for non-secret config defaults
- Supports directives ({{file:}}, {{exec:}}) and system variables ({{$agent}}, {{$agentpath}}, etc.)
- Values are resolved at config load time
- Override via `.env` for secrets or local development

</details>

> [!IMPORTANT]
> **Hot-reload enabled** - Edit any config file while daemon runs. Changes apply on next message. No restart needed.

### Prompt File

Prompt sections define conversation structure. Two types:

```
[>role]       # Static section: processed once at parse time
[>>role]      # Input section: {{$input}} replaced per message (only one allowed)
```

**Example:**

```
[>system]
You are a helpful assistant with access to shell tools.
Agent: {{$agent}}
User: {{$user}}
Session: {{$session}}
Memory: {{file:.data/tools/memory/{{$user}}/memory.md}}
Context: {{file:./docs/context.md}}
Timestamp: {{$timestamp}}

[>user]
Analyze the latest logs.

[>assistant]
I'll check the logs for you.

[>>user]
{{$input}}
```

**Directives in prompts:**

<details>
<summary><b>Directive syntax</b></summary>

Directives are processed at parse time (once per request, due to hot-reload):

- `{{file:path}}` - Load file content
  - Relative: `{{file:./docs/readme.md}}` (from agent folder)
  - Home: `{{file:~/.config/app.conf}}`
  - Absolute: `{{file:/etc/config}}`

- `{{exec:command}}` - Execute shell command, inject stdout
  - Scripts: `{{exec:./scripts/generate-context.sh}}`
  - PATH commands: `{{exec:date -Iseconds}}`
  - Pipes: `{{exec:cat logs.txt | grep ERROR}}`

- `{{var}}` - Variable placeholder (no colon = not a directive, preserved for runtime substitution)

**Features:**
- Recursive: files loaded via `{{file:}}` can contain `{{exec:}}` directives (10-level depth limit)
- Nested: variables in directive paths: `{{file:data/{{$user}}/notes.md}}`
- Inline: `Current time: {{exec:date}} - processing...`
- Escape: `\{{literal}}` becomes literal `{{literal}}`
- Unknown directives cause parse errors (fail fast)

**System variables** (available in all contexts):
- `{{$agent}}` - Agent folder basename
- `{{$agentpath}}` - Absolute agent folder path
- `{{$user}}` - User identity ID
- `{{$username}}` - User display name
- `{{$session}}` - Current session ID
- `{{$gateway}}` - Gateway name (cli, telegram); `{{$interface}}` still works as a deprecated alias
- `{{$model}}` - LLM model name
- `{{$provider}}` - LLM provider name
- `{{$timestamp}}` - RFC3339 timestamp
- `{{$date}}` - ISO date (YYYY-MM-DD)
- `{{$input}}` - User input (only in `[>>role]` input sections)

**Resolution phases:**
1. **Directives first**: `{{file:}}` and `{{exec:}}` processed, supports nested directives with variables in paths (e.g., `{{file:users/{{$user}}/notes.txt}}`)
2. **Variables second**: System variables (`{{$var}}`) and user variables (`{{var}}`) substituted

</details>

<details>
<summary><b>Section types</b></summary>

**Static sections `[>role]`:**
- Processed once at parse time
- Directives executed immediately
- Variables substituted from runtime context
- Used for system prompts, examples, context

**Input section `[>>role]`:**
- Only one allowed per prompt
- `{{$input}}` placeholder replaced with user message
- Directives processed at parse time
- Variables preserved for runtime substitution
- Required for agent to receive messages

</details>

### Tool Definitions

Tools are TOML files in `tools/` directory. Each tool executes a shell command with variable substitution.

**Basic example (`tools/weather.toml`):**

```toml
command = "curl -s wttr.in/{{city}}?format=3"
description = "Get weather for a city"

[city]
description = "City name"
type = "string"
required = true
```

**Tool execution:**
1. AI calls tool with parameters: `{"city": "London"}`
2. Variables substituted inline: `curl -s wttr.in/London?format=3`
3. Parameters injected as environment variables: `$TOOL_CITY=London`
4. Executed via `sh -c` from tool's directory
5. stdout/stderr returned to AI

**Environment variables:**
All resolved parameters are injected as `TOOL_<PARAMNAME>` environment variables (uppercase):
- Safe for multiline values and special characters
- Example: parameter `location` becomes `$TOOL_LOCATION`
- Use env vars instead of `{{param}}` for complex shell scripts
- `$AGENT_PATH` - Always available, contains absolute agent folder path
- `[environment]` section values from agent.toml also injected

> [!WARNING]
> **Shell execution** - Tools run via `sh -c`. AI controls parameters. Validate inputs, use return overrides for sensitive values, disable dangerous tools with `enabled = false`.

<details>
<summary><b>Parameter fields</b></summary>

```toml
[param_name]
description = "Parameter description shown to AI"
type = "string"              # Data type: string, number, boolean, etc.
required = true              # Whether AI must provide this parameter (default: false)
enabled = true               # false = hide from AI, not used even if return is set (default: true)
return = "value"             # Override with literal, file, or command (hides from AI automatically)
```

**Parameter behavior:**
- `enabled = false` - Hard disable, parameter not used
- `return != ""` - Blackbox override, hidden from AI automatically
- AI only sees parameters where `enabled=true` and `return=""`

</details>

<details>
<summary><b>Return field (parameter overrides)</b></summary>

Use `return` to provide values without AI control:

```toml
# Load from file (hidden from AI)
[api_key]
description = "API key"
type = "string"
required = true
return = "{{file:.env.API_KEY}}"    # Loaded at execution time, hidden from AI

# Execute command (hidden from AI)
[timestamp]
description = "Current timestamp"
type = "string"
return = "{{exec:date -Iseconds}}"  # Dynamic value from command

# Literal value (hidden from AI)
[environment]
description = "Environment name"
type = "string"
return = "production"               # Hardcoded value

# Wrap AI value (visible to AI)
[output_file]
description = "Output file path"
type = "string"
return = "{{$agentpath}}/output/{{$completion}}"  # AI provides filename, we add path
```

**Directives in return:**
- `{{file:path}}` - Load file content
- `{{exec:command}}` - Execute shell command
- `{{$completion}}` - AI-provided value for this parameter (keeps parameter visible to AI)
- System variables: `{{$agent}}`, `{{$agentpath}}`, `{{$user}}`, `{{$session}}`, etc.
- Processed at tool execution time (supports hot-reload)
- Escape: `return = '\{{literal}}'` (TOML literal string) or `return = "\\{{literal}}"`

**Behavior:**
- Parameters with `return` are hidden from AI (blackbox) UNLESS they contain `{{$completion}}`
- `{{$completion}}` keeps parameter visible and wraps the AI's value (e.g., `return = "--flag {{$completion}}"`)
- Return values override AI-provided arguments during execution
- Supports same path resolution as prompt directives (relative, ~/, absolute)
- Supports nested directives with variables in paths (e.g., `{{file:{{$agentpath}}/secrets/key}}`)

</details>

**Advanced example (`tools/deploy.toml`):**

```toml
command = "./scripts/deploy.sh {{service}} {{env}} {{api_key}}"
description = "Deploy service to specified environment"

[service]
description = "Service name to deploy"
type = "string"
required = true

[env]
description = "Target environment"
type = "string"
return = "production"              # AI doesn't control this

[api_key]
description = "API key for deployment"
type = "string"
enabled = false                    # Hidden from AI entirely
return = "{{file:.secrets/key}}"   # Loaded from file at execution time
```

## Routines (Scheduled Commands)

Routines are TOML files in the `routines/` directory. Where a **tool** fires when the AI decides to call it, a **routine** fires on a **schedule** — no AI involvement, no arguments from the model, no reply fed back into a conversation. The command runs fire-and-forget and its output is only logged. Use routines for recurring jobs: a proactive daily check-in (where the command is itself an `agentctl deliver`/`agentctl chat` back into a gateway), a periodic scrape, a nightly backup or cleanup.

**How they run:**
- The scheduler lives **inside the `serve` daemon** — there is no system crontab. A routine only ever fires while its owning agent's daemon is running.
- Checked once per minute, so schedules resolve at minute granularity.
- No catch-up: an occurrence whose time already passed when the daemon started is not back-fired.
- Overlap is allowed — a long-running routine won't block its next fire.
- **Hot-reload:** editing/adding/removing a `routines/*.toml` is picked up live. But if the agent had **no `routines/` folder at all** when the daemon started, creating one requires a `serve` restart (the folder is checked once at startup). `init` scaffolds the `routines/` folder up front, so this only bites if you delete it.

`init` drops a `routines/example.toml` as a fill-in syntax reference — copy or rename it to create a real routine. Like `tools/example.toml`, a file named `example.toml` is skipped by the scheduler, so the reference never fires.

**Basic example (`routines/standup.toml`):**

```toml
command = "agentctl deliver telegram -m 'Standup in 5 minutes' -a ."
description = "Weekday standup reminder"   # optional, informational only
enabled = true                             # optional, default true

[schedule]
every = "mon,tue,wed,thu,fri"
time = "08:55"
```

**Schedule field (`[schedule]` is required — pick exactly one shape):**

| Shape | Example | Meaning |
|-------|---------|---------|
| Weekday list | `every = "mon,wed,fri"` + `time = "09:00"` | Those weekdays at HH:MM |
| Day-of-month list | `every = "1,15"` + `time = "09:00"` | Those days (1–31) at HH:MM |
| Every N days | `every = "3d"` + `time = "09:00"` | Every 3rd day at HH:MM |
| Every N hours | `every = "6h"` | Every 6 hours (**no `time`**) |
| RRULE | `rrule = "FREQ=WEEKLY;BYDAY=MO,WE"` | RFC-5545 recurrence rule |

- `time` is 24-hour `HH:MM`. **Required** for weekday / day-of-month / `Nd`; **not allowed** with `Nh`; unused with `rrule`.
- `every` tokens must all be the **same category** — mixing (e.g. `"mon,3d"`) is a hard error. This is deliberate: it avoids cron's ambiguous "OR" semantics.
- `rrule` is mutually exclusive with `every`/`time`.
- A malformed routine is a blocking error at daemon startup, and a logged (non-fatal) warning on live reload.

**Parameters** work like tool parameters — same table syntax — but since no AI supplies arguments, they're only useful with a `return` override. Resolved values inject inline as `{{param}}` and as `$ROUTINE_<PARAM>` environment variables (note the `ROUTINE_` prefix, not `TOOL_`):

```toml
command = "./scripts/report.sh"
description = "Daily report"

[schedule]
every = "1d"
time = "07:00"

[today]
return = "{{exec:date +%F}}"   # exposed to the script as $ROUTINE_TODAY and {{today}}
```

Available `{{$...}}` context in a routine is limited to `{{$agent}}`, `{{$agentpath}}`, and time variables (`{{$timestamp}}`, `{{$date}}`, `{{$now}}`) — a routine has no user/session, so `{{$user}}`/`{{$session}}` are not available. The agent's `[environment]` values are injected the same way as for tools. The command runs from the routine file's own directory.

> [!NOTE]
> There is no `toolrun` equivalent for routines. To test one, run `agentctl serve <agent> --debug`, temporarily set `time` to a minute or two in the future (or a short `rrule`), and watch `.data/logs/` for the fire. Restore the real schedule afterward.

## Gateways

Gateways determine how users interact with your agent. Currently supports CLI and Telegram. Additional gateways planned.

### CLI Gateway

One-shot, in-process access - no daemon or socket required, and not actually daemon-hosted. Every `agentctl chat` call resolves the agent, runs the turn, and exits.

```bash
# Send message (from agent folder)
agentctl chat "analyze logs"

# Specify agent folder
agentctl chat "hello" -a ~/agents/my-agent

# Use specific user and session
agentctl chat "continue conversation" -u alice -s 20260614_150000_abc123

# Debug mode (shows session file path)
agentctl chat "test" --debug
```

**Session management:**
- Sessions are organized by user ID (default: system username)
- Each user can have multiple sessions
- Session storage: `.data/sessions/{userID}/{sessionID}.jsonl`
- `.last_session` tracks most recent session per gateway
- Use `--user` and `--session` flags for explicit control

### Telegram Gateway

The only daemon-hosted gateway - `agentctl serve` starts a persistent listener for it (long polling). Enable in `agent.toml`:

```toml
interfaces = ["cli", "telegram"]
```

Add bot token to `.env`:

```
TELEGRAM_BOT_TOKEN=your_bot_token_here
```

**Features:**
- Per-user sessions (automatic user ID from Telegram)
- Typing indicators during processing
- System commands: `/start`, `/new`, `/sessions` (session switching via ID, not numeric index)
- Voice message transcription (requires `[audio]` config)

Both gateways share the same agent runtime and session storage.

### Identity Linking

Link multiple contact points to a single user identity in `.data/contacts.toml`:

```toml
# ============================================
# IDENTITIES (user-edited)
# ============================================
[[identity]]
id = "alice"
contacts = ["cli:alice", "telegram:123456789"]

[[identity]]
id = "bob"
contacts = ["telegram:987654321"]

# ============================================
# CONTACTS (auto-generated - do not edit)
# ============================================
[[contact]]
interface = "telegram"
id = "123456789"
display_name = "Alice"
first_seen = 2026-06-17T15:04:05Z
```

**File structure:**
- `.data/contacts.toml` contains both user-edited identities and auto-generated contacts
- Created automatically on first message
- Edit `[[identity]]` section to link contacts
- `[[contact]]` section is managed automatically

**Benefits:**
- Unified conversation history across gateways
- CLI and Telegram messages in same session
- Automatic migration from unlinked to linked contacts

**Contact format:** `gateway:platformID` (stored on-disk under the `interface` key; see contacts.toml example above)
- CLI: `cli:username` (system username)
- Telegram: `telegram:userID` (Telegram user ID)

**Migration:**
- Startup: Migrates all unlinked contacts on daemon start
- Lazy: Auto-migrates on first message after adding to identity
- No daemon restart needed when updating contacts.toml
- Sessions automatically consolidated into identity folder

## Advanced Features

### Hot-Reload

Edit any config file while daemon runs. Changes take effect immediately:

```bash
# Terminal 1: daemon running
agentctl serve my-agent

# Terminal 2: edit files
vim agent.toml        # Change model
vim tools/weather.toml # Update tool command
vim prompt            # Modify system prompt

# Next chat message uses new config
agentctl chat "test updated config"
```

**What's hot-reloaded:**
- `agent.toml` (provider, model, tools, gateways, memory)
- `tools/*.toml` (all tool definitions)
- `prompt` (all sections and directives)
- `.env` (API keys loaded fresh each request)

**Behavior:**
- Malformed configs return errors but daemon stays running
- No need to restart daemon for experiments
- Validation happens per request, not at startup

### Prerun Hooks

Execute shell scripts before each agent execution for setup, validation, or dynamic configuration:

```bash
# .prerun.sh (hidden, checked first) or prerun.sh (fallback)
#!/usr/bin/env bash

# Example: Create required directories
mkdir -p .data/custom

# Example: Validate required files
[ -f .data/config.json ] || echo '{}' > .data/config.json

# Example: Source tool-specific prerun scripts
for tool_dir in tools/*/; do
    [ -f "$tool_dir/.prerun.sh" ] && source "$tool_dir/.prerun.sh"
    [ -f "$tool_dir/prerun.sh" ] && source "$tool_dir/prerun.sh"
done
```

**Behavior:**
- Runs from agent root directory before config loading
- Non-fatal: errors logged but agent continues (ideal for self-healing operations)
- Access to `.env` environment variables
- Changes take effect on next message (hot-reload)
- Common uses: mkdir, touch, source tool-specific setup

**Tool-specific prerun** (e.g., `tools/memory/.prerun.sh`):
```bash
#!/usr/bin/env bash
mkdir -p .data/tools/memory
```

Root `.prerun.sh` auto-sources all tool prerun scripts, enabling modular setup.

### Debug Features

**1. Debug logging (`--debug` flag):**

```bash
agentctl serve my-agent --debug
```

Shows message previews, tool names, execution details in structured logs.

**2. Full API call logging (`logging = "debug"` in agent.toml):**

```toml
[agent]
logging = "debug"
```

Writes the merged request+response for a user's most recent API call to
`.data/sessions/{userID}/last-call.json`, overwritten on every call (only the
latest exchange is kept — no accumulating files to clean up). Session-title
generation calls are excluded so they don't overwrite the real conversation's
record. Inspect with:

```bash
cat .data/sessions/{userID}/last-call.json | jq .
```

### Session Management

Conversation history organized by user and session in `.data/sessions/{userID}/{sessionID}.jsonl`:

```jsonl
{"role":"user","content":"hello","timestamp":"2026-06-14T15:04:05Z"}
{"role":"assistant","content":"Hi! How can I help?","timestamp":"2026-06-14T15:04:06Z"}
```

**Configuration:**

```toml
[memory]
max_messages = 100  # Keep last N messages per session (0 = no persistence)
```

**Session organization:**
- User ID: System username (CLI) or Telegram user ID
- Session ID: `YYYYMMDD_HHMMSS_<6-hex>` format
- `.last_session`: Tracks most recent session per gateway
- Identity linking: Multiple contacts share same user folder

**Persistence:**
- History survives daemon restarts
- Sessions isolated by user and session ID
- JSONL format (plain text, one message per line)
- Automatic migration when contacts are linked to identities

**System Commands:**
- `/new` - Create new session and automatically switch to it
- `/sessions` - List all sessions (newest first, shows active session)
- `/sessions attach <number|id>` - Switch to specific session (CLI: supports numeric index; Telegram: use session ID)
- `/start` - Welcome message (Telegram only)

System commands are handled by the gateway layer and don't reach the agent. They provide session management without consuming agent context.

### Cross-Gateway Message Delivery

Send messages from one gateway and deliver to others:

```bash
# Send from CLI, deliver to telegram (doesn't modify telegram session)
agentctl chat -m "update" --deliver telegram

# Deliver to multiple channels
agentctl chat -m "broadcast" --deliver telegram,cli

# Deliver AND inject into target session (preserves history)
agentctl chat -m "update" --deliver telegram --inject

# Inject as a specific role instead of the assistant default
agentctl chat -m "update" --deliver telegram --inject system

# Specify user explicitly (user@gateway format)
agentctl chat -m "message" --deliver alice@telegram
```

**Channel syntax:**
- `telegram` - Deliver to current user's telegram contact
- `alice@telegram` - Deliver to specific user's telegram contact
- Comma-separated lists for multiple targets

**Behavior:**
- `--deliver` alone: Delivers response without modifying target session
- `--deliver` with `--inject [role]`: Delivers AND adds a turn (role defaults to `assistant`) to target session
- Response shown on CLI regardless of delivery targets
- Delivery failures logged as warnings, don't break request

**Raw delivery (no LLM call):**

`chat` always runs an agent turn before delivering. Use `deliver` instead to send literal text straight to a channel, e.g. for scheduled reminders where no model call is needed:

```bash
# Deliver literal text, no agent call
agentctl deliver telegram -m "Reminder: standup in 5 minutes"

# Deliver to multiple channels
agentctl deliver telegram,cli -m "Reminder: standup in 5 minutes"

# Deliver AND inject into target session
agentctl deliver telegram -m "Reminder: standup in 5 minutes" --inject

# Deliver AND inject, with a system-role note attached for model context
# (the note is never sent to the channel, only saved to session history)
agentctl deliver telegram -m "Reminder: standup in 5 minutes" --inject --note "Scheduled message delivered at 09:00 2026-07-08."

# Multi-line content without shell quoting issues
AGENTCTL_MESSAGE="line one
line two" agentctl deliver telegram --user alice
```

Channel is a positional argument here (unlike `chat`'s optional `--deliver` flag) since delivering *somewhere* is the entire point of `deliver`. Same channel syntax and `--inject [role]` semantics as `chat`'s delivery flags — `deliver` just skips the agent call and treats the message as the response verbatim. `--note` requires `--inject` (errors otherwise) since a system note only makes sense alongside an injected turn.

### Tool Whitelisting

Override agent.toml tools for a single run:

```bash
# Only expose specific tools for this request
agentctl chat "task" --tools "web_fetch,file_reader"

# Single tool
agentctl chat "search" --tools "web_search"
```

**Behavior:**
- Comma-separated tool names
- Overrides `tools = []` in agent.toml for this run only
- Other requests use normal tool configuration
- Config file unchanged

### Session Injection

Manually add turns to sessions without running agent:

```bash
# Inject assistant response (role omitted, defaults to assistant)
agentctl inject -m "Here's the data" --session 20260614_150000_abc

# Inject user message
agentctl inject user -m "follow up question" --session 20260614_150000_abc

# Inject a model-only system note
agentctl inject system -m "Scheduled message delivered at 09:00 2026-07-08." --session 20260614_150000_abc

# Specify agent
agentctl inject assistant -m "response" --session 20260614_abc -a my-agent
```

**Use cases:**
- Pre-populate conversations
- Fix conversation history
- Testing and debugging
- Cross-agent coordination

**Behavior:**
- Session must exist (error if not found)
- No agent execution, direct JSONL append
- Preserves conversation flow
- Respects memory.max_messages limit

### Manual Tool Execution

Test tools without running agent:

```bash
# Execute tool with parameters
agentctl toolrun weather --city=London

# Tool with multiple parameters
agentctl toolrun create_schedule --name=backup --cron="0 2 * * *" --message="Daily backup"

# Uses current directory as agent folder (or specify with -a)
agentctl toolrun deploy --service=api -a ~/agents/production-agent
```

**Behavior:**
- Loads tool definition from `tools/<name>.toml`
- Substitutes provided parameters into command
- Executes via `sh -c` from agent folder
- Prints stdout/stderr

## Examples

### Simple Weather Tool

```toml
# tools/weather.toml
command = "curl -s wttr.in/{{city}}?format=3"
description = "Get current weather for a city"

[city]
description = "City name"
type = "string"
required = true
```

### Git Operations Tool

```toml
# tools/git_status.toml
command = "cd {{repo_path}} && git status --short"
description = "Show git status for repository"

[repo_path]
description = "Path to git repository"
type = "string"
required = true
```

### Agent Composition (Agent calling another agent)

```toml
# tools/ask_specialist.toml
command = "agentctl chat '{{question}}' -a {{specialist}} -s {{session}}"
description = "Ask a specialist agent a question"

[question]
description = "Question to ask"
type = "string"
required = true

[specialist]
description = "Specialist agent name"
type = "string"
required = true

[session]
description = "Session key"
type = "string"
return = "{{exec:uuidgen}}"  # Generate unique session ID
```

One agent calls another agent.

### Secure API Tool with Hidden Credentials

```toml
# tools/api_call.toml
command = "curl -X POST {{url}} -H 'Authorization: Bearer {{token}}' -d '{{data}}'"
description = "Make authenticated API call"

[url]
description = "API endpoint URL"
type = "string"
required = true

[data]
description = "JSON payload"
type = "string"
required = true

[token]
description = "API authentication token"
type = "string"
enabled = false
return = "{{file:.secrets/api_token}}"  # AI never sees this
```

### Context-Aware Prompt

```
[>system]
You are a code reviewer with access to the current git state.

Agent: {{$agent}}
User: {{$username}}
Repository: {{exec:basename $(pwd)}}
Branch: {{exec:git branch --show-current}}
Recent commits:
{{file:./docs/recent-commits.txt}}

Guidelines:
{{file:./docs/review-guidelines.md}}

[>>user]
{{$input}}
```

## Architecture

**Daemon + Gateway model:**

```
agentctl chat → one-shot, in-process, no daemon (CLI access)

agentctl serve → daemon starts → loads daemon-hosted gateways from agent.toml
                                └─ Telegram → long polling
                                        ↓
                                Shares the same agent runtime as one-shot chat
```

**Component boundaries:**
- **internal/config** - File loading (agent.toml, tools/*.toml, prompt)
- **internal/resolution** - Two-phase template resolution (directives → variables)
- **internal/providers/llm** - AI provider abstraction (openai, openrouter, generic)
- **internal/agent** - Orchestration (agentic loop, tool execution)
- **internal/gateways** - Gateway abstraction (CLI, Telegram) + outbound dispatcher
- **internal/session** - Session management (identity linking, migration, JSONL persistence, channel resolution)
- **internal/orchestration** - Application layer (message handling, delivery coordination, tool filtering)
- **internal/shell** - Pure shell execution utility
- **internal/hooks** - Prerun hook execution (non-fatal)
- **internal/syscommands** - System command parser (/new, /sessions, etc.)

**Agentic loop:**
1. Build messages: static sections + history + input
2. Send to provider with tool definitions
3. If response has tool_calls: execute tools, append results, loop to step 2
4. Return final text response

**Modularity:**
- Tools are shell commands
- Providers are self-contained files
- Gateways share Runner abstraction
- Directives support file/exec operations

## Help System

Built-in help with topic pages:

```bash
agentctl help              # Overview + command list
agentctl help setup        # Getting started guide
agentctl help config       # agent.toml reference
agentctl help tools        # Tool definition format
agentctl help prompt       # Prompt file format
agentctl help gateways     # CLI and Telegram setup
agentctl help sessions     # Session management and history
agentctl help prerun       # Prerun hook system
```

## Testing

**Use free models to avoid API costs:**

```bash
# List free models with tool support
agentctl models openrouter --free --tools

# Good free options:
# - openrouter/free (meta-router, uses free models automatically)
# - meta-llama/llama-3.3-70b-instruct:free
# - google/gemma-2-27b-it:free
```

**Test workflow:**

```bash
# Create test agent
agentctl init _test-agent
cd _test-agent

# Set free model in agent.toml
echo 'provider = "openrouter"' > agent.toml
echo 'model = "openrouter/free"' >> agent.toml

# Start daemon
agentctl serve

# Test in another terminal
agentctl chat "hello"
```

## License

MIT
