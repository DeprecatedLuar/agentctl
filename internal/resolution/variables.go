package resolution

import "strings"

// substituteVariables replaces {{var}} and {{$var}} placeholders with their values.
// System variables ({{$var}}) take precedence over user variables ({{var}}).
//
// Does NOT process directives ({{directive:path}}) - those are handled separately.
// Supports backslash escaping: \{{...}} becomes literal {{...}}.
func substituteVariables(content string, sysVars, userVars map[string]string) string {
	var result strings.Builder
	pos := 0

	for {
		// Find next {{
		start := strings.Index(content[pos:], prefix)
		if start == -1 {
			// No more placeholders - append rest of content
			result.WriteString(content[pos:])
			break
		}
		start += pos // Adjust to absolute position

		// Check for escape sequence \{{
		if start > 0 && content[start-1] == byte(escapeChar) {
			// Remove backslash and keep {{ as literal
			result.WriteString(content[pos : start-1]) // Write up to (but not including) backslash
			result.WriteString(prefix)                  // Write literal {{
			pos = start + len(prefix)
			continue
		}

		// Find matching closing }} (handle nested {{...}})
		endOffset := findMatchingCloseBrace(content[start+len(prefix):])
		if endOffset == -1 {
			// No closing }} - append rest and stop
			result.WriteString(content[pos:])
			break
		}
		end := start + len(prefix) + endOffset // Position of }}

		// Append content before placeholder
		result.WriteString(content[pos:start])

		// Extract variable name
		varName := content[start+len(prefix) : end]

		// Check if this is a directive (contains :) - skip if so
		colonIdx := strings.IndexByte(varName, byte(directiveSep))
		if colonIdx != -1 {
			// This is a directive, not a variable - keep as-is
			result.WriteString(prefix)
			result.WriteString(varName)
			result.WriteString(suffix)
			pos = end + len(suffix)
			continue
		}

		// Try to replace variable (system vars take precedence)
		replaced := false
		if value, exists := sysVars[varName]; exists {
			result.WriteString(value)
			replaced = true
		} else if value, exists := userVars[varName]; exists {
			result.WriteString(value)
			replaced = true
		}

		if !replaced {
			// Variable not found - keep placeholder as-is
			result.WriteString(prefix)
			result.WriteString(varName)
			result.WriteString(suffix)
		}

		pos = end + len(suffix)
	}

	return result.String()
}
