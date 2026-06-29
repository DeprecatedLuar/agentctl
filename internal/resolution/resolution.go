package resolution

// Process is the main pipeline for template processing.
// Executes in order:
//  1. Process directives ({{file:path}}, {{exec:command}}) with recursive expansion
//     - Variables in directive arguments are substituted before processing
//  2. Substitute variables ({{var}}, {{$var}})
//
// Context provides runtime values for system variables and directive processing.
// Returns processed content and error if any step fails.
func Process(content string, ctx Context) (string, error) {
	// Step 1: Process directives (may contain nested directives)
	// Variables in directive paths are substituted during this phase
	processed, err := processDirectives(content, ctx)
	if err != nil {
		return "", err
	}

	// Step 2: Build system variables map
	sysVars := buildSystemVariables(ctx)

	// Step 3: Substitute all variables (system + user)
	// System variables take precedence over user variables
	result := substituteVariables(processed, sysVars, ctx.UserVars)

	return result, nil
}

// ValidateSyntax validates template syntax without executing directives or substituting variables.
// Only checks for syntax errors (unknown directives, empty paths, invalid escapes).
//
// Use this at daemon startup to fail fast on malformed templates.
// Returns error only for syntax issues, not runtime issues.
func ValidateSyntax(content string) error {
	return validateDirectiveSyntax(content)
}
