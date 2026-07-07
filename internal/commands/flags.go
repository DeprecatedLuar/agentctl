package commands

// Turn roles shared by inject, deliver, and chat.
const (
	roleAssistant = "assistant"
	roleUser      = "user"
	roleSystem    = "system"
)

// isValidRole reports whether r is one of the known turn roles.
func isValidRole(r string) bool {
	return r == roleAssistant || r == roleUser || r == roleSystem
}

// parseInjectArg handles the optional-value --inject flag at args[i]
// (args[i] must equal flagInject). Bare "--inject" injects as assistant;
// "--inject <role>" overrides the role when the next arg is a valid role.
// Returns how many args were consumed (1 or 2) and the resolved role.
func parseInjectArg(args []string, i int) (consumed int, role string) {
	if i+1 < len(args) && isValidRole(args[i+1]) {
		return 2, args[i+1]
	}
	return 1, roleAssistant
}
