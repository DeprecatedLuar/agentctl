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

// flagRule associates one or more flag spellings (e.g. "--agent"/"-a") with a
// parse function invoked when args[i] matches one of names. parse returns how
// many args were consumed (including the flag itself) and any error.
type flagRule struct {
	names []string
	parse func(args []string, i int) (consumed int, err error)
}

// parseFlags walks args left to right, matching each token against rules in
// order and dispatching to the first match's parse function. Tokens matching
// no rule are passed to onDefault (which consumes exactly one token); a
// non-nil error from either parse or onDefault stops the walk immediately.
// Shared by chat/deliver/inject argument parsing; toolrun and run have
// parsing shapes (generic --key=value, silent positional passthrough) that
// don't fit this and are left as hand-rolled loops.
func parseFlags(args []string, rules []flagRule, onDefault func(arg string) error) error {
	i := 0
	for i < len(args) {
		matched := false
		for _, r := range rules {
			for _, n := range r.names {
				if args[i] == n {
					consumed, err := r.parse(args, i)
					if err != nil {
						return err
					}
					if consumed < 1 {
						consumed = 1
					}
					i += consumed
					matched = true
					break
				}
			}
			if matched {
				break
			}
		}
		if matched {
			continue
		}
		if err := onDefault(args[i]); err != nil {
			return err
		}
		i++
	}
	return nil
}
