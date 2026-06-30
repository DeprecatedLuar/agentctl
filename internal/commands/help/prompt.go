package help

import gohelp "github.com/DeprecatedLuar/gohelp-luar"

func PromptPage() *gohelp.Page {
	return gohelp.NewPage("prompt", "prompt file format").
		Text("The prompt file defines your agent's system instructions and how user messages are incorporated. It supports two section types: static sections for context that's processed once at startup, and input sections that receive each user message. Use directives to inject file contents or command output, and variables to access runtime context like user IDs and session information.").
		Usage("prompt file structure").
		Section("Section Types",
			gohelp.Item("[>role]", "Static section (directives and variables processed at parse time)"),
			gohelp.Item("[>>role]", "Input section with {{$input}} placeholder (only one allowed)"),
		).
		Section("Directives (processed at parse time)",
			gohelp.Item("{{file:path}}", "Load file content (supports ./, ~/, absolute paths)"),
			gohelp.Item("{{exec:cmd}}", "Execute command and inject stdout"),
			gohelp.Item("Nested directives", "Variables in paths: {{file:.data/memory/{{$user}}/notes.md}}"),
		).
		Section("System Variables",
			gohelp.Item("{{$input}}", "User's message (input section only)"),
			gohelp.Item("{{$agent}}", "Agent name"),
			gohelp.Item("{{$agentpath}}", "Agent folder absolute path"),
			gohelp.Item("{{$user}}", "User identity ID"),
			gohelp.Item("{{$username}}", "Display name"),
			gohelp.Item("{{$session}}", "Session ID"),
			gohelp.Item("{{$interface}}", "Interface name (cli, telegram)"),
			gohelp.Item("{{$timestamp}}", "ISO8601 timestamp"),
			gohelp.Item("{{$date}}", "ISO date (YYYY-MM-DD)"),
			gohelp.Item("{{$model}}", "Model name"),
			gohelp.Item("{{$provider}}", "Provider name"),
		).
		Text("The [>>role] section is required for the agent to receive messages. Static sections build conversation context. Directives are recursive (10-level depth limit). Variables in directive paths are substituted before processing, enabling per-user files and dynamic paths.")
}
