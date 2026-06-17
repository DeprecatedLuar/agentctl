package main

import (
	"fmt"
	"os"

	gohelp "github.com/DeprecatedLuar/gohelp-luar"

	"github.com/DeprecatedLuar/agentctl/internal/commands"
)

func main() {
	if len(os.Args) < 2 {
		printHelp(nil)
		os.Exit(1)
	}

	cmd := os.Args[1]
	args := os.Args[2:]

	switch cmd {
	case "init":
		if err := commands.HandleInit(args); err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			os.Exit(1)
		}
	case "run":
		if err := commands.HandleRun(args); err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			os.Exit(1)
		}
	case "chat":
		if err := commands.HandleChat(args); err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			os.Exit(1)
		}
	case "getagent":
		if err := commands.HandleGetAgent(args); err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			os.Exit(1)
		}
	case "models":
		if err := commands.HandleModels(args); err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			os.Exit(1)
		}
	case "toolrun":
		if err := commands.HandleToolRun(args); err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			os.Exit(1)
		}
	case "inject":
		if err := commands.HandleInject(args); err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			os.Exit(1)
		}
	case "help":
		printHelp(args)
	default:
		printHelp(nil)
		os.Exit(1)
	}
}

func printHelp(args []string) {
	// Build args for gohelp.Run: prepend "help" if needed
	helpArgs := []string{"help"}
	if len(args) > 0 {
		helpArgs = append(helpArgs, args...)
	}

	root := gohelp.NewPage("agentctl", "agentic workflow tool").
		Usage("agentctl <command> [args]").
		Section("Commands",
			gohelp.Item("init [path]", "Initialize agent folder with config templates", "agentctl init my-agent"),
			gohelp.Item("run [path]", "Start agent daemon with configured interfaces", "agentctl run my-agent"),
			gohelp.Item("chat <message>", "Send message to running agent daemon", "agentctl chat \"hello\" -a my-agent"),
			gohelp.Item("inject <content>", "Inject a turn into a session without running agent", "agentctl inject \"response\" --role assistant --session 20250614_abc -a my-agent"),
			gohelp.Item("toolrun <name>", "Execute a tool manually with parameters", "agentctl toolrun create_schedule --name=test --cron=\"0 * * * *\" --message=\"Test\""),
			gohelp.Item("getagent", "Print current agent name", "agentctl getagent"),
			gohelp.Item("models [provider]", "List available models (openai, openrouter, or both)", "agentctl models openrouter --free"),
			gohelp.Item("help [topic]", "Show help (topics: setup, config, tools, prompt, interfaces, memory)"),
		).
		Section("Chat Flags",
			gohelp.Item("--agent, -a <path>", "Agent folder path (default: current directory)"),
			gohelp.Item("--session, -s <id>", "Session ID for explicit session selection"),
			gohelp.Item("--user, -u <id>", "User ID for explicit user selection"),
			gohelp.Item("--channel <list>", "Deliver response to channels (comma-separated, e.g., telegram,cli)"),
			gohelp.Item("--channel-inject <list>", "Deliver and inject into channel sessions (comma-separated)"),
			gohelp.Item("--tools <list>", "Whitelist tools for this run (comma-separated)"),
			gohelp.Item("--debug", "Show debug information including session file path"),
		).
		Section("Inject Flags",
			gohelp.Item("--role <assistant|user>", "Role of the injected turn (required)"),
			gohelp.Item("--session, -s <id>", "Session ID to inject into (required)"),
			gohelp.Item("--agent, -a <path>", "Agent folder path (default: current directory)"),
		).
		Section("Toolrun Flags",
			gohelp.Item("--agent, -a <path>", "Agent folder path (default: current directory)"),
			gohelp.Item("--<param>=<value>", "Tool parameters (e.g., --name=test --cron=\"0 * * * *\")"),
		).
		Section("Models Flags",
			gohelp.Item("--free", "Show only free models"),
			gohelp.Item("--tools", "Show only models with tool support"),
			gohelp.Item("--stt", "Show speech-to-text models"),
			gohelp.Item("--all", "Show all models including obscure providers (default: popular + free only)"),
		).
		Text("Run 'agentctl help <topic>' for detailed documentation.")

	setup := gohelp.NewPage("setup", "getting started guide").
		Section("Quick Start",
			gohelp.Item("1. Create agent", "Initialize a new agent folder", "agentctl init my-agent"),
			gohelp.Item("2. Configure .env", "Add API key: OPENAI_API_KEY or OPENROUTER_API_KEY"),
			gohelp.Item("3. Edit agent.toml", "Set provider (openai/openrouter) and model name"),
			gohelp.Item("4. Run daemon", "Start agent with configured interfaces", "agentctl run my-agent"),
			gohelp.Item("5. Send messages", "Chat with agent via CLI", "agentctl chat \"hello\" -a my-agent"),
		).
		Section("Provider Setup",
			gohelp.Item("OpenAI", "Set OPENAI_API_KEY in .env, use provider=\"openai\" in agent.toml"),
			gohelp.Item("OpenRouter", "Set OPENROUTER_API_KEY in .env, use provider=\"openrouter\" in agent.toml"),
		).
		Text("Agent folder structure: agent.toml, prompt, tools/, .env, .data/")

	config := gohelp.NewPage("config", "agent.toml reference").
		Usage("agent.toml structure").
		Section("Required Fields",
			gohelp.Item("provider", "AI provider: \"openai\" or \"openrouter\""),
			gohelp.Item("model", "Model name (e.g. \"gpt-4\", \"anthropic/claude-3.5-sonnet\")"),
		).
		Section("Optional Fields",
			gohelp.Item("tools", "Tool names to load (empty array = auto-discover all .toml in tools/)"),
			gohelp.Item("interfaces", "Interfaces to enable: [\"cli\"] or [\"cli\", \"telegram\"]"),
			gohelp.Item("logging", "Enable structured logging (default: true)"),
			gohelp.Item("memory.max_messages", "History limit per session (0 = unlimited, default: 0)"),
		).
		Text("Example: provider=\"openai\" model=\"gpt-4\" tools=[] interfaces=[\"cli\"]")

	tools := gohelp.NewPage("tools", "creating tool definitions").
		Usage("tools/*.toml format").
		Section("Required Fields",
			gohelp.Item("command", "Shell command with {{var}} placeholders", "command = \"curl wttr.in/{{city}}\""),
		).
		Section("Optional Fields",
			gohelp.Item("description", "Tool description for the AI"),
			gohelp.Item("[paramName]", "Parameter sections (not nested under [parameters]!)"),
		).
		Section("Parameter Fields",
			gohelp.Item("description", "Parameter description"),
			gohelp.Item("type", "Parameter type (default: \"string\")"),
			gohelp.Item("required", "Whether required (default: false)"),
		).
		Text("Tools are executed via 'sh -c' with {{var}} substitution from AI tool calls. Files named example.toml are ignored during auto-discovery.")

	prompt := gohelp.NewPage("prompt", "prompt file format").
		Usage("prompt file structure").
		Section("Section Types",
			gohelp.Item("[>role]", "Static section (variables substituted at parse time)"),
			gohelp.Item("[>>role]", "Input section with {{input}} placeholder (only one allowed)"),
		).
		Section("Special Syntax",
			gohelp.Item("<path", "Load file content (relative to agent folder, supports ~/ and absolute paths)"),
			gohelp.Item("{{var}}", "Variable placeholder (substituted in static sections, preserved in input section)"),
		).
		Text("The [>>role] section is required for the agent to receive messages. Static sections build conversation context.")

	interfaces := gohelp.NewPage("interfaces", "interface configuration").
		Usage("interfaces = [\"cli\", \"telegram\"]").
		Section("CLI Interface",
			gohelp.Item("Socket", "Unix socket at .data/agent.sock"),
			gohelp.Item("Protocol", "JSON lines: {\"session\":\"key\",\"message\":\"text\"}"),
			gohelp.Item("Session", "Defaults to \"default\" if not specified"),
			gohelp.Item("Usage", "Use 'agentctl chat' command to send messages"),
		).
		Section("Telegram Interface",
			gohelp.Item("Setup", "Set TELEGRAM_BOT_TOKEN in .env file"),
			gohelp.Item("Sessions", "Session key is Telegram user ID (automatic isolation)"),
			gohelp.Item("Features", "Typing indicators, /start command"),
		).
		Text("All interfaces share the same agent runtime and memory. Enable both for multi-channel access.")

	memory := gohelp.NewPage("memory", "session isolation and history").
		Usage("memory.max_messages = 0").
		Section("Storage",
			gohelp.Item("Format", "JSONL files in .data/memory/{sessionKey}.jsonl"),
			gohelp.Item("Fields", "Each line: {\"role\",\"content\",\"ts\"} (timestamp)"),
			gohelp.Item("Persistence", "History survives daemon restarts"),
		).
		Section("Session Isolation",
			gohelp.Item("CLI", "Use --session flag to specify session key (default: \"default\")"),
			gohelp.Item("Telegram", "Session key is user ID (automatic per-user isolation)"),
		).
		Section("Limits",
			gohelp.Item("max_messages", "Controls history size (0 = unlimited, recommended for development)"),
		).
		Text("Each session has independent conversation history. Sessions are isolated by key.")

	gohelp.Run(helpArgs, root, setup, config, tools, prompt, interfaces, memory)
}
