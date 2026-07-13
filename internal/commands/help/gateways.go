package help

import gohelp "github.com/DeprecatedLuar/gohelp-luar"

func GatewaysPage() *gohelp.Page {
	return gohelp.NewPage("gateways", "gateway configuration").
		Text("Gateways determine how users interact with your agent. The CLI is a one-shot, daemonless identity (no listener process; every 'agentctl chat' call runs in-process), while the Telegram gateway provides a persistent bot frontend hosted by 'agentctl serve'. All gateways share the same agent runtime, memory, and tool access, enabling seamless multi-channel deployment. Configure enabled gateways in agent.toml.").
		Usage("interfaces = [\"cli\", \"telegram\"]  # key name frozen on-disk, still called \"interfaces\"").
		Section("CLI Gateway",
			gohelp.Item("Model", "One-shot, in-process - no daemon required"),
			gohelp.Item("Session", "Defaults to \"default\" if not specified"),
			gohelp.Item("Usage", "Use 'agentctl chat' command to send messages"),
		).
		Section("Telegram Gateway",
			gohelp.Item("Setup", "Set TELEGRAM_BOT_TOKEN in .env file"),
			gohelp.Item("Sessions", "Session key is Telegram user ID (automatic isolation)"),
			gohelp.Item("Features", "Typing indicators, /start command"),
		).
		Text("All gateways share the same agent runtime and memory. Enable both for multi-channel access.")
}
