package help

import gohelp "github.com/DeprecatedLuar/gohelp-luar"

func ToolsPage() *gohelp.Page {
	return gohelp.NewPage("tools", "creating tool definitions").
		Text("Tools extend your agent with custom capabilities by defining shell commands the AI can execute. Each tool is a .toml file that specifies a command template with parameters the AI fills in. Tools can call APIs, manipulate files, query databases, or run any shell operation. The agent auto-discovers tools in the tools/ directory unless you specify an explicit whitelist.").
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
			gohelp.Item("enabled", "Show to AI (default: true, false = hidden)"),
			gohelp.Item("return", "Override value with directive support (hides from AI unless contains {{$completion}})"),
		).
		Section("Special Variables in Return Fields",
			gohelp.Item("{{$completion}}", "AI's value for this parameter (shows param to AI)", "return = \"--flag {{$completion}}\""),
			gohelp.Item("{{file:path}}", "Load file content", "return = \"{{file:.env.API_KEY}}\""),
			gohelp.Item("{{exec:cmd}}", "Execute command", "return = \"{{exec:date -Iseconds}}\""),
		).
		Section("Environment Variables",
			gohelp.Item("TOOL_<PARAM>", "All parameters injected as TOOL_* env vars (uppercase)", "$TOOL_USER, $TOOL_FILE, $TOOL_MESSAGE"),
			gohelp.Item("AGENT_PATH", "Agent folder absolute path (always available)"),
		).
		Text("Tools are executed via 'sh -c' with {{var}} substitution from AI tool calls. Additionally, all resolved parameters are injected as TOOL_<PARAMNAME> environment variables (uppercase), enabling safe handling of multiline values and special characters. Files named example.toml are ignored during auto-discovery.")
}
