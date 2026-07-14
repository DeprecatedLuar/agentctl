package help

import gohelp "github.com/DeprecatedLuar/gohelp-luar"

func RootPage() *gohelp.Page {
	return gohelp.NewPage("agentctl", "agentic workflow tool").
		Text("Agentctl manages AI agents that execute tools via shell commands. `chat`/`deliver` run in-process (no daemon needed); `serve` starts a long-running daemon for persistent gateways (Telegram) and scheduled routines. Each agent maintains conversation history across sessions and can be extended with custom tools. Use 'agentctl help <topic>' to learn about setup, configuration, tools, prompts, gateways, sessions, and prerun hooks.").
		Usage("agentctl <command> [args]").
		Section("Commands",
			gohelp.Item("init [path]", "Initialize agent folder with config templates", "agentctl init my-agent"),
			gohelp.Item("serve [path]", "Start the daemon (persistent gateways + routines); `run`/`up` are hidden aliases", "agentctl serve my-agent"),
			gohelp.Item("stop", "Stop a running agent daemon (graceful, escalates to SIGKILL after 5s)", "agentctl stop -a my-agent"),
			gohelp.Item("chat", "Run a single conversation turn in-process, no daemon required (default when no/unrecognized command given)", "agentctl chat -m \"hello\" -a my-agent  (or just: agentctl -m \"hello\" -a my-agent)"),
			gohelp.Item("inject [role]", "Inject a turn into a session without running agent", "agentctl inject assistant -m \"response\" --session 20250614_abc -a my-agent"),
			gohelp.Item("deliver", "Deliver literal text to channels without running the agent", "agentctl deliver -m \"Reminder: standup\" --deliver telegram --inject"),
			gohelp.Item("monitor <agent>", "Follow an agent's log file live", "agentctl monitor my-agent"),
			gohelp.Item("toolrun <name>", "Execute a tool manually with parameters", "agentctl toolrun create_schedule --name=test --cron=\"0 * * * *\" --message=\"Test\""),
			gohelp.Item("getagent", "Print current agent name", "agentctl getagent"),
			gohelp.Item("models [provider]", "List available models (openai, openrouter, or both)", "agentctl models openrouter --vendor anthropic --sort price"),
			gohelp.Item("help [topic]", "Show help (topics: setup, config, tools, prompt, gateways, sessions, prerun)"),
		).
		Section("Chat Flags",
			gohelp.Item("--message, -m <text>", "Message text (falls back to $AGENTCTL_MESSAGE, then opens $EDITOR on a temp file if interactive)"),
			gohelp.Item("--agent, -a <path>", "Agent folder path (default: current directory, falls back to $AGENTCTL_AGENT)"),
			gohelp.Item("--session, -s <id>", "Session ID for explicit session selection"),
			gohelp.Item("--user, -u <id>", "User ID for explicit user selection"),
			gohelp.Item("--deliver <list>", "Deliver response to channels (comma-separated, e.g., telegram,cli)"),
			gohelp.Item("--inject [role]", "Also inject the delivered response into the target session(s); role defaults to assistant (assistant|user|system)"),
			gohelp.Item("--tools <list>", "Whitelist tools for this run (comma-separated)"),
			gohelp.Item("--model <name>", "Override agent.toml's model for this call (falls back to $AGENTCTL_MODEL)"),
			gohelp.Item("--provider <name>", "Override agent.toml's provider for this call (falls back to $AGENTCTL_PROVIDER)"),
			gohelp.Item("--debug", "Show debug information (stderr only) including session file path"),
		).
		Section("Deliver Flags",
			gohelp.Item("--message, -m <text>", "Message text (falls back to $AGENTCTL_MESSAGE, then opens $EDITOR on a temp file if interactive)"),
			gohelp.Item("--deliver <list>", "Channels to deliver the literal message to (comma-separated, required)"),
			gohelp.Item("--inject [role]", "Also inject the message into the target session(s); role defaults to assistant (assistant|user|system)"),
				gohelp.Item("--note <text>", "System-role note injected immediately before the delivered turn (requires --inject)"),
			gohelp.Item("--user, -u <id>", "User ID for bare-gateway channel resolution (falls back to $AGENTCTL_USER)"),
			gohelp.Item("--agent, -a <path>", "Agent folder path (default: current directory)"),
		).
		Section("Inject Flags",
			gohelp.Item("--message, -m <text>", "Content to inject (falls back to $AGENTCTL_MESSAGE, then opens $EDITOR on a temp file if interactive)"),
			gohelp.Item("inject [role]", "Role of the injected turn: assistant|user|system (default: assistant)"),
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
				gohelp.Item("--vendor <name>", "Show only models from this vendor (e.g. anthropic, google) — not the same as the openai/openrouter provider arg"),
				gohelp.Item("--sort <key>", "Order results by provider|price|context (default: provider)"),
		).
		Text("Run 'agentctl help <topic>' for detailed documentation.")
}
