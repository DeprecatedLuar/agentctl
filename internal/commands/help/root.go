package help

import gohelp "github.com/DeprecatedLuar/gohelp-luar"

func RootPage() *gohelp.Page {
	return gohelp.NewPage("agentctl", "agentic workflow tool").
		Text("Agentctl manages AI agents that execute tools via shell commands. Each agent runs as a daemon with configurable interfaces (CLI, Telegram), maintains conversation history across sessions, and can be extended with custom tools. Use 'agentctl help <topic>' to learn about setup, configuration, tools, prompts, interfaces, sessions, and prerun hooks.").
		Usage("agentctl <command> [args]").
		Section("Commands",
			gohelp.Item("init [path]", "Initialize agent folder with config templates", "agentctl init my-agent"),
			gohelp.Item("run [path]", "Start agent daemon with configured interfaces", "agentctl run my-agent"),
			gohelp.Item("chat", "Send message to running agent daemon", "agentctl chat -m \"hello\" -a my-agent"),
			gohelp.Item("inject", "Inject a turn into a session without running agent", "agentctl inject -m \"response\" --role assistant --session 20250614_abc -a my-agent"),
			gohelp.Item("deliver", "Deliver literal text to channels without running the agent", "agentctl deliver -m \"Reminder: standup\" --deliver telegram --inject"),
			gohelp.Item("toolrun <name>", "Execute a tool manually with parameters", "agentctl toolrun create_schedule --name=test --cron=\"0 * * * *\" --message=\"Test\""),
			gohelp.Item("getagent", "Print current agent name", "agentctl getagent"),
			gohelp.Item("models [provider]", "List available models (openai, openrouter, or both)", "agentctl models openrouter --free"),
			gohelp.Item("help [topic]", "Show help (topics: setup, config, tools, prompt, interfaces, sessions, prerun)"),
		).
		Section("Chat Flags",
			gohelp.Item("--message, -m <text>", "Message text (falls back to $AGENTCTL_MESSAGE, then opens $EDITOR on a temp file if interactive)"),
			gohelp.Item("--agent, -a <path>", "Agent folder path (default: current directory)"),
			gohelp.Item("--session, -s <id>", "Session ID for explicit session selection"),
			gohelp.Item("--user, -u <id>", "User ID for explicit user selection"),
			gohelp.Item("--deliver <list>", "Deliver response to channels (comma-separated, e.g., telegram,cli)"),
			gohelp.Item("--inject", "Also inject the delivered response into the target session(s)"),
			gohelp.Item("--tools <list>", "Whitelist tools for this run (comma-separated)"),
			gohelp.Item("--debug", "Show debug information including session file path"),
		).
		Section("Deliver Flags",
			gohelp.Item("--message, -m <text>", "Message text (falls back to $AGENTCTL_MESSAGE, then opens $EDITOR on a temp file if interactive)"),
			gohelp.Item("--deliver <list>", "Channels to deliver the literal message to (comma-separated, required)"),
			gohelp.Item("--inject", "Also inject the message as an assistant turn into the target session(s)"),
			gohelp.Item("--user, -u <id>", "User ID for bare-interface channel resolution (falls back to $AGENTCTL_USER)"),
			gohelp.Item("--agent, -a <path>", "Agent folder path (default: current directory)"),
		).
		Section("Inject Flags",
			gohelp.Item("--message, -m <text>", "Content to inject (falls back to $AGENTCTL_MESSAGE, then opens $EDITOR on a temp file if interactive)"),
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
}
