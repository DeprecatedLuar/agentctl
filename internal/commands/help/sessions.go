package help

import gohelp "github.com/DeprecatedLuar/gohelp-luar"

func SessionsPage() *gohelp.Page {
	return gohelp.NewPage("sessions", "session management and history").
		Text("Sessions organize conversation history into isolated contexts. Each session maintains its own message history stored as JSONL files that persist across daemon restarts. Users can create new sessions, switch between them, and link multiple gateway contacts (CLI + Telegram) to share history under a single identity. Sessions receive auto-generated titles after the first exchange.").
		Usage("memory.max_messages = 100").
		Section("Storage",
			gohelp.Item("Format", "JSONL files in .data/sessions/{userID}/{sessionID}.jsonl"),
			gohelp.Item("Session ID", "Format: YYYYMMDD_HHMMSS_<6-hex> (auto-generated)"),
			gohelp.Item("Metadata", "First line contains session metadata (title, creation time)"),
			gohelp.Item("Persistence", "History survives daemon restarts"),
		).
		Section("Identity Linking",
			gohelp.Item("Contacts", "Auto-logged in .data/contacts.toml on first message"),
			gohelp.Item("Identities", "Link multiple contacts to one user (e.g., CLI + Telegram)"),
			gohelp.Item("Format", "Contact: gateway:platformID (e.g., cli:alice, telegram:123456789)"),
			gohelp.Item("Sessions", "All linked contacts share the same session history"),
		).
		Section("Session Management",
			gohelp.Item("/new", "Create new session (auto-switch)"),
			gohelp.Item("/sessions", "List all sessions (newest first, shows active)"),
			gohelp.Item("/sessions attach <id>", "Switch to session (CLI supports numbers)"),
			gohelp.Item("Auto-titles", "Sessions get short LLM-generated titles after first exchange"),
		).
		Section("User Resolution",
			gohelp.Item("CLI", "Defaults to system username as user ID"),
			gohelp.Item("Telegram", "Uses Telegram user ID (automatic per-user isolation)"),
			gohelp.Item("Explicit", "Use --user and --session flags for direct access"),
		).
		Section("Memory Limits",
			gohelp.Item("max_messages", "Keep last N messages per session (0 = no persistence, default: 100)"),
		).
		Text("Sessions are organized by user identity. Multiple gateway contacts can link to one identity for unified history across CLI, Telegram, etc.")
}
