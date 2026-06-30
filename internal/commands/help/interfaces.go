package help

import gohelp "github.com/DeprecatedLuar/gohelp-luar"

func InterfacesPage() *gohelp.Page {
	return gohelp.NewPage("interfaces", "interface configuration").
		Text("Interfaces determine how users interact with your agent. The CLI interface uses Unix sockets for command-line access, while the Telegram interface provides a bot frontend. All interfaces share the same agent runtime, memory, and tool access, enabling seamless multi-channel deployment. Configure enabled interfaces in agent.toml.").
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
}
