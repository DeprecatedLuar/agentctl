package help

import gohelp "github.com/DeprecatedLuar/gohelp-luar"

func ConfigPage() *gohelp.Page {
	return gohelp.NewPage("config", "agent.toml reference").
		Text("The agent.toml file controls your agent's behavior, model selection, gateway configuration, and memory limits. Edit this file to switch providers or models, enable specific tools, configure gateways, or adjust conversation history retention. Changes take effect on the next message (hot-reload enabled).").
		Usage("agent.toml structure").
		Section("Required Fields",
			gohelp.Item("provider", "AI provider: \"openai\" or \"openrouter\""),
			gohelp.Item("model", "Model name (e.g. \"gpt-4\", \"anthropic/claude-3.5-sonnet\")"),
		).
		Section("Optional Fields",
			gohelp.Item("tools", "Tool names to load (empty array = auto-discover all .toml in tools/)"),
			gohelp.Item("interfaces", "Daemon-hosted gateways to enable: [\"cli\"] or [\"cli\", \"telegram\"] (key name frozen on-disk; cli is never actually daemon-hosted, see 'help gateways')"),
			gohelp.Item("logging", "Enable structured logging (default: true)"),
			gohelp.Item("memory.max_messages", "Keep last N messages per session (0 = no persistence, default: 100)"),
		).
		Text("Example: provider=\"openai\" model=\"gpt-4\" tools=[] interfaces=[\"cli\"]")
}
