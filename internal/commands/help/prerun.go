package help

import gohelp "github.com/DeprecatedLuar/gohelp-luar"

func PrerunPage() *gohelp.Page {
	return gohelp.NewPage("prerun", "prerun hook system").
		Text("Prerun hooks execute shell scripts before each agent execution to perform setup, validation, or dynamic configuration. The system checks for .prerun.sh (hidden) first, then falls back to prerun.sh. Errors are logged but don't block agent execution, making hooks ideal for self-healing operations like creating directories or initializing files.").
		Usage(".prerun.sh or prerun.sh").
		Section("Overview",
			gohelp.Item("Purpose", "Run setup/validation before each agent execution"),
			gohelp.Item("Execution", "Runs before config loading, non-fatal (warns but continues)"),
			gohelp.Item("Precedence", "Checks .prerun.sh first (hidden), falls back to prerun.sh"),
			gohelp.Item("Hot reload", "Changes take effect on next message"),
		).
		Section("Default Template",
			gohelp.Item("Auto-source tools", "Loops through tools/*/ and sources .prerun.sh or prerun.sh"),
			gohelp.Item("Custom logic", "Add setup commands below the tool-sourcing loop"),
		).
		Section("Common Use Cases",
			gohelp.Item("Create directories", "mkdir -p .data/custom"),
			gohelp.Item("Validate files", "[ -f .data/required ] || touch .data/required"),
			gohelp.Item("Tool setup", "Per-tool prerun in tools/memory/.prerun.sh"),
		).
		Section("Example: Tool-Specific Prerun",
			gohelp.Item("Location", "tools/memory/.prerun.sh"),
			gohelp.Item("Content", "#!/usr/bin/env bash\nmkdir -p .data/tools/memory"),
			gohelp.Item("Sourced by", "Root .prerun.sh auto-sources all tool prerun scripts"),
		).
		Text("Prerun scripts run from agent root directory with access to .env variables. Errors are logged but don't block agent execution.")
}
