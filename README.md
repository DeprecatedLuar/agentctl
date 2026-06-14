# agentctl

Framework for building AI agent harnesses. Tools are bash scripts with variable substitution. Agents can call other agents. All operations are shell commands. Edit configs while daemon runs—changes apply immediately.

## Quick Start

```bash
# 1. Initialize agent
agentctl init my-agent
cd my-agent

# 2. Add API key to .env
echo "OPENROUTER_API_KEY=your_key_here" >> .env

# 3. Start daemon (Terminal 1)
agentctl run

# 4. Chat with agent (Terminal 2)
agentctl chat "hello"
```

Default config uses OpenRouter's free model router.

## Installation

```bash
# Clone and build
git clone https://github.com/DeprecatedLuar/agentctl
cd agentctl
go build -o agentctl ./cmd/agentctl

# Install to PATH
cp agentctl ~/bin/  # or /usr/local/bin
```

## Commands

| Command | Description | Example |
|---------|-------------|---------|
| `init [path]` | Create agent folder with templates | `agentctl init my-agent` |
| `run [path]` | Start daemon with configured interfaces | `agentctl run my-agent` |
| `chat <message>` | Send message to running daemon | `agentctl chat "analyze logs" -a my-agent` |
| `toolrun <name>` | Execute tool manually with parameters | `agentctl toolrun weather --city=London` |
| `getagent` | Print current agent name from registry | `agentctl getagent` |
| `models [provider]` | List available models | `agentctl models openrouter --free --tools` |
| `help [topic]` | Show detailed help | `agentctl help tools` |

### Flags

**`run` command:**
- `--debug` - Debug logging with message previews
- `--log` - Enable file logging (`.data/logs/`)
- `-v, --verbose` - Verbose logging

**`chat` command:**
- `-a, --agent <path>` - Agent folder path (default: current dir)
- `-s, --session <key>` - Session key for memory isolation (default: "default")

**`models` command:**
- `--free` - Show only free models
- `--tools` - Show only models with tool support
- `--all` - Include all providers (default: popular + free only)

**`toolrun` command:**
- `-a, --agent <path>` - Agent folder path (default: current dir)
- `--<param>=<value>` - Tool parameters (e.g., `--city=London --units=metric`)

## Agent Folder Structure

```
my-agent/
├── agent.toml          # Provider, model, interfaces, memory config
├── prompt              # Prompt template with sections
├── tools/              # Tool definitions (*.toml)
│   └── example.toml    # Example tool (excluded from auto-load)
├── .env                # API keys (OPENAI_API_KEY, OPENROUTER_API_KEY, TELEGRAM_BOT_TOKEN)
└── .data/              # Runtime data (auto-created)
    ├── agent.sock      # Unix socket for CLI interface
    ├── memory/         # Conversation history (per-session JSONL)
    └── debug-calls/    # Full request/response JSON (if debug_calls=true)
```

## Configuration

### agent.toml

<details>
<summary><b>Full specification</b></summary>

```toml
provider = "openrouter"              # "openai", "openrouter", or http/https URL for custom endpoints
model = "openrouter/free"            # Model name (provider-specific)
tools = []                           # Empty = auto-discover all .toml in tools/, or ["name1", "name2"]
interfaces = ["cli"]                 # ["cli", "telegram"] or ["cli"] only
logging = true                       # Enable structured logging (default: true)
debug_calls = false                  # Write full API requests/responses to .data/debug-calls/

[memory]
max_messages = 0                     # History limit per session (0 = unlimited)

[audio]                              # Optional: speech-to-text for voice messages
provider = "whisper"                 # "whisper" or http://localhost:9000/v1
model = "whisper-1"
```

**Providers:**
- `openai` - Requires `OPENAI_API_KEY` in .env
- `openrouter` - Requires `OPENROUTER_API_KEY` in .env
- `http://...` or `https://...` - OpenAI-compatible custom endpoints (Ollama, LM Studio, vLLM)

**Tool loading:**
- `tools = []` - Auto-discover: recursively loads all `.toml` files in `tools/` (except `example.toml`)
- `tools = ["name1", "name2"]` - Explicit: loads only `tools/name1.toml` and `tools/name2.toml` (no subdirectory support)

**Interfaces:**
- `cli` - Unix socket at `.data/agent.sock` (use `agentctl chat` to send messages)
- `telegram` - Telegram bot (requires `TELEGRAM_BOT_TOKEN` in .env)

</details>

### Prompt File

Prompt sections define conversation structure. Two types:

```
[>role]       # Static section: processed once at parse time
[>>role]      # Input section: {{input}} replaced per message (only one allowed)
```

**Example:**

```
[>system]
You are a helpful assistant with access to shell tools.
Context: {{file:./docs/context.md}}
Timestamp: {{exec:date -Iseconds}}

[>user]
Analyze the latest logs.

[>assistant]
I'll check the logs for you.

[>>user]
{{input}}
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
- Inline: `Current time: {{exec:date}} - processing...`
- Escape: `\{{literal}}` becomes literal `{{literal}}`
- Unknown directives cause parse errors (fail fast)

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
- `{{input}}` placeholder replaced with user message
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
2. Variables substituted: `curl -s wttr.in/London?format=3`
3. Executed via `sh -c` from agent folder
4. stdout/stderr returned to AI

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
# Load from file
[api_key]
description = "API key"
type = "string"
required = true
return = "{{file:.env.API_KEY}}"    # Loaded at execution time, hidden from AI

# Execute command
[timestamp]
description = "Current timestamp"
type = "string"
return = "{{exec:date -Iseconds}}"  # Dynamic value from command

# Literal value
[environment]
description = "Environment name"
type = "string"
return = "production"               # Hardcoded value
```

**Directives in return:**
- `{{file:path}}` - Load file content
- `{{exec:command}}` - Execute shell command
- Processed at tool execution time (supports hot-reload)
- Escape: `return = '\{{literal}}'` (TOML literal string) or `return = "\\{{literal}}"`

**Behavior:**
- Parameters with `return` are automatically hidden from AI (blackbox)
- Return values override AI-provided arguments during execution
- Supports same path resolution as prompt directives (relative, ~/, absolute)

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

## Interfaces

### CLI Interface

Default interface using Unix socket at `.data/agent.sock`.

```bash
# Send message (from agent folder)
agentctl chat "analyze logs"

# Specify agent folder
agentctl chat "hello" -a ~/agents/my-agent

# Use specific session
agentctl chat "continue conversation" -a my-agent -s session-123
```

**Session isolation:**
- Each session has independent conversation history
- Default session key: "default"
- Memory stored in `.data/memory/{session}.jsonl`

### Telegram Interface

Enable in `agent.toml`:

```toml
interfaces = ["cli", "telegram"]
```

Add bot token to `.env`:

```
TELEGRAM_BOT_TOKEN=your_bot_token_here
```

**Features:**
- Per-user sessions (session key = user ID)
- Typing indicators during processing
- `/start` command support
- Voice message transcription (requires `[audio]` config)

Both interfaces share the same agent runtime and memory.

## Advanced Features

### Hot-Reload

Edit any config file while daemon runs—changes take effect immediately:

```bash
# Terminal 1: daemon running
agentctl run my-agent

# Terminal 2: edit files
vim agent.toml        # Change model
vim tools/weather.toml # Update tool command
vim prompt            # Modify system prompt

# Next chat message uses new config
agentctl chat "test updated config"
```

**What's hot-reloaded:**
- `agent.toml` (provider, model, tools, interfaces, memory)
- `tools/*.toml` (all tool definitions)
- `prompt` (all sections and directives)
- `.env` (API keys loaded fresh each request)

**Behavior:**
- Malformed configs return errors but daemon stays running
- No need to restart daemon for experiments
- Validation happens per request, not at startup

### Debug Features

**1. Debug logging (`--debug` flag):**

```bash
agentctl run my-agent --debug
```

Shows message previews, tool names, execution details in structured logs.

**2. Full API call logging (`debug_calls = true` in agent.toml):**

```toml
debug_calls = true
```

Writes request/response JSON to `.data/debug-calls/`:
- `2026-06-14T15-04-05-request.json` - Full messages array, tool definitions, provider/model
- `2026-06-14T15-04-05-response.json` - Full AI response

Auto-cleanup keeps last 10 files. Inspect with:

```bash
cat .data/debug-calls/*.json | jq .
```

### Memory System

Conversation history stored per-session in `.data/memory/{session}.jsonl`:

```jsonl
{"role":"user","content":"hello","ts":"2026-06-14T15:04:05Z"}
{"role":"assistant","content":"Hi! How can I help?","ts":"2026-06-14T15:04:06Z"}
```

**Configuration:**

```toml
[memory]
max_messages = 0  # 0 = unlimited (recommended for development)
                  # N = keep last N messages per session
```

**Session keys:**
- CLI: "default" (or specify with `--session` flag)
- Telegram: User ID

**Persistence:**
- History survives daemon restarts
- Each session isolated
- JSONL format (plain text, one message per line)

### Manual Tool Execution

Test tools without running the agent:

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

Repository: {{exec:basename $(pwd)}}
Branch: {{exec:git branch --show-current}}
Recent commits:
{{file:./docs/recent-commits.txt}}

Guidelines:
{{file:./docs/review-guidelines.md}}

[>>user]
{{input}}
```

## Architecture

**Daemon + Interface model:**

```
agentctl run → daemon starts → loads interfaces from agent.toml
                               ├─ CLI → Unix socket
                               └─ Telegram → long polling
                                       ↓
                               Both share same agent runtime
```

**Component boundaries:**
- **internal/config** - File loading (agent.toml, tools/*.toml, prompt)
- **internal/providers/llm** - AI provider abstraction (openai, openrouter, generic)
- **internal/agent** - Orchestration (agentic loop, tool execution)
- **internal/interfaces** - Interface abstraction (CLI, Telegram)
- **internal/memory** - JSONL persistence (per-session history)
- **internal/shell** - Pure shell execution utility
- **internal/directives** - Directive processor ({{file:}}, {{exec:}})

**Agentic loop:**
1. Build messages: static sections + history + input
2. Send to provider with tool definitions
3. If response has tool_calls: execute tools, append results, loop to step 2
4. Return final text response

**Modularity:**
- Tools are shell commands (no Go code changes needed)
- Providers are self-contained files (add new providers by creating a file)
- Interfaces share Runner abstraction (add new interfaces by implementing Interface)
- Directives support file/exec operations (extensible)

## Help System

Built-in help with topic pages:

```bash
agentctl help              # Overview + command list
agentctl help setup        # Getting started guide
agentctl help config       # agent.toml reference
agentctl help tools        # Tool definition format
agentctl help prompt       # Prompt file format
agentctl help interfaces   # CLI and Telegram setup
agentctl help memory       # Session isolation and history
```

Fuzzy matching for typos: `agentctl help memori` suggests "memory".

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
agentctl run

# Test in another terminal
agentctl chat "hello"
```

## License

MIT
