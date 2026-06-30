package help

import gohelp "github.com/DeprecatedLuar/gohelp-luar"

func SetupPage() *gohelp.Page {
	return gohelp.NewPage("setup", "getting started guide").
		Text("This guide walks you through creating your first agent. You'll initialize an agent folder, configure API credentials, and send your first message. The process takes about 5 minutes and requires an API key from OpenAI or OpenRouter.").
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
}
