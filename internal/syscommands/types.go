package syscommands

// Command represents a parsed system command
type Command struct {
	Name string
	Args []string
}

// CommandResult contains structured data returned by command execution
type CommandResult struct {
	Type string
	Data interface{}
}

// SessionInfo contains session metadata for display
type SessionInfo struct {
	SessionID string
	Title     string
	CreatedAt int64
	IsActive  bool
}

// Result type constants
const (
	ResultTypeNewSession      = "new_session"
	ResultTypeSessionList     = "session_list"
	ResultTypeSessionSwitched = "session_switched"
)
