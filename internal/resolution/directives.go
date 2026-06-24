package resolution

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"github.com/DeprecatedLuar/agentctl/internal/shell"
)

const (
	// Directive delimiters
	prefix = "{{"
	suffix = "}}"

	// Special characters
	escapeChar       = '\\'
	directiveSep     = ':'
	homeDirPrefix    = "~/"

	// Limits
	maxDepth         = 10  // Prevent infinite recursion
	binaryCheckLimit = 512 // Bytes to check for null bytes
	nullByte         = 0
)

// validateDirectiveSyntax validates directive syntax without executing anything.
// Only checks for syntax errors (unknown directive types, empty paths, invalid escapes).
// Does NOT load files or execute scripts.
//
// Returns error only for syntax issues, not runtime issues.
func validateDirectiveSyntax(content string) error {
	return validateSyntaxWithDepth(content, 0)
}

// processDirectives processes {{file:path}} and {{exec:path}} directives recursively.
// Supports backslash escaping: \{{...}} becomes literal {{...}}.
//
// Directive types:
//   - {{file:path}}: Loads file content (relative to agentPath)
//   - {{exec:path}}: Executes script, returns stdout (or "exit N: stderr" on failure)
//
// Returns processed content and error if directive processing fails.
func processDirectives(content, agentPath string) (string, error) {
	return processDirectivesWithDepth(content, agentPath, 0)
}

// validateSyntaxWithDepth validates directive syntax with depth limit
func validateSyntaxWithDepth(content string, depth int) error {
	if depth > maxDepth {
		return fmt.Errorf("directive nesting too deep (max %d levels)", maxDepth)
	}

	pos := 0
	hasDirectives := false

	// Scan all {{...}} patterns
	for {
		// Find next {{
		start := strings.Index(content[pos:], prefix)
		if start == -1 {
			break
		}
		start += pos

		// Check for escape sequence \{{
		if start > 0 && content[start-1] == byte(escapeChar) {
			pos = start + len(prefix)
			continue
		}

		// Find closing }}
		endOffset := strings.Index(content[start+len(prefix):], suffix)
		if endOffset == -1 {
			break
		}
		end := start + len(prefix) + endOffset

		// Extract inner content
		inner := content[start+len(prefix) : end]

		// Check if this is a directive (contains :)
		colonIdx := strings.IndexByte(inner, byte(directiveSep))
		if colonIdx == -1 {
			// Variable placeholder, skip
			pos = end + len(suffix)
			continue
		}

		// This is a directive - validate it
		hasDirectives = true
		directiveType := inner[:colonIdx]
		directivePath := inner[colonIdx+1:]

		// Validate path syntax
		if len(directivePath) == 0 {
			return fmt.Errorf("{{%s:}}: empty path not allowed", directiveType)
		}
		if directivePath[0] == ' ' {
			return fmt.Errorf("{{%s:%s}}: invalid syntax, space after colon not allowed (use {{%s:%s}})",
				directiveType, directivePath, directiveType, strings.TrimSpace(directivePath))
		}
		if directivePath[len(directivePath)-1] == ' ' {
			return fmt.Errorf("{{%s:%s}}: invalid syntax, space before closing braces not allowed (use {{%s:%s}})",
				directiveType, directivePath, directiveType, strings.TrimSpace(directivePath))
		}

		// Validate directive type (syntax check only, don't execute)
		switch directiveType {
		case "file", "exec":
			// Valid directive types
		default:
			return fmt.Errorf("{{%s:%s}}: unknown directive type '%s' (valid: file, exec)", directiveType, directivePath, directiveType)
		}

		pos = end + len(suffix)
	}

	// If we found directives, we would need to validate nested content too
	// But since we're not actually processing, we can't get the nested content
	// Nested validation happens during processDirectives
	// This is a trade-off: syntax-only validation can't catch nested syntax errors
	// until runtime, but that's acceptable since the first level is validated
	_ = hasDirectives

	return nil
}

// processDirectivesWithDepth handles directives with recursion depth limit
func processDirectivesWithDepth(content, agentPath string, depth int) (string, error) {
	if depth > maxDepth {
		return "", fmt.Errorf("directive nesting too deep (max %d levels)", maxDepth)
	}

	var result strings.Builder
	pos := 0
	hasDirectives := false

	// Process all {{...}} patterns
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

		// Find closing }}
		endOffset := strings.Index(content[start+len(prefix):], suffix)
		if endOffset == -1 {
			// No closing }} - append rest and stop
			result.WriteString(content[pos:])
			break
		}
		end := start + len(prefix) + endOffset // Position of }}

		// Append content before placeholder
		result.WriteString(content[pos:start])

		// Extract inner content
		inner := content[start+len(prefix) : end]

		// Check if this is a directive (contains :) or a variable
		colonIdx := strings.IndexByte(inner, byte(directiveSep))
		if colonIdx == -1 {
			// No colon - this is a variable placeholder, keep as-is
			result.WriteString(prefix)
			result.WriteString(inner)
			result.WriteString(suffix)
			pos = end + len(suffix)
			continue
		}

		// This is a directive - extract type and path
		hasDirectives = true
		directiveType := inner[:colonIdx]
		directivePath := inner[colonIdx+1:]

		// Validate path syntax (strict: no spaces, no empty paths)
		if len(directivePath) == 0 {
			return "", fmt.Errorf("{{%s:}}: empty path not allowed", directiveType)
		}
		if directivePath[0] == ' ' {
			return "", fmt.Errorf("{{%s:%s}}: invalid syntax, space after colon not allowed (use {{%s:%s}})",
				directiveType, directivePath, directiveType, strings.TrimSpace(directivePath))
		}
		if directivePath[len(directivePath)-1] == ' ' {
			return "", fmt.Errorf("{{%s:%s}}: invalid syntax, space before closing braces not allowed (use {{%s:%s}})",
				directiveType, directivePath, directiveType, strings.TrimSpace(directivePath))
		}

		var replacement string
		var err error

		switch directiveType {
		case "file":
			// Load file content
			replacement, err = loadFile(directivePath, agentPath)
			if err != nil {
				return "", fmt.Errorf("{{file:%s}}: %w", directivePath, err)
			}

		case "exec":
			// Execute script and capture output
			replacement, err = execScript(directivePath, agentPath)
			if err != nil {
				return "", fmt.Errorf("{{exec:%s}}: %w", directivePath, err)
			}

		default:
			// Unknown directive type
			return "", fmt.Errorf("{{%s:%s}}: unknown directive type '%s' (valid: file, exec)", directiveType, directivePath, directiveType)
		}

		// Append replacement content
		result.WriteString(replacement)
		pos = end + len(suffix)
	}

	finalResult := result.String()

	// If we processed any directives, recursively process the result
	// to handle nested directives (e.g., {{file:x.md}} where x.md contains {{exec:y.sh}})
	if hasDirectives {
		return processDirectivesWithDepth(finalResult, agentPath, depth+1)
	}

	return finalResult, nil
}

// loadFile loads file content with path resolution
func loadFile(path, agentPath string) (string, error) {
	// Resolve path
	var fullPath string
	if strings.HasPrefix(path, homeDirPrefix) {
		home, err := os.UserHomeDir()
		if err != nil {
			slog.Error("directive {{file:}} failed to get home directory", "path", path, "error", err)
			return "", fmt.Errorf("failed to get home directory: %w", err)
		}
		fullPath = filepath.Join(home, path[len(homeDirPrefix):])
	} else if filepath.IsAbs(path) {
		fullPath = path
	} else {
		fullPath = filepath.Join(agentPath, path)
	}

	// Check if file exists and is not a directory
	info, err := os.Stat(fullPath)
	if err != nil {
		slog.Error("directive {{file:}} failed to load file", "path", path, "full_path", fullPath, "error", err)
		return "", fmt.Errorf("failed to load file %s: %w", path, err)
	}
	if info.IsDir() {
		slog.Error("directive {{file:}} cannot load directory", "path", path, "full_path", fullPath)
		return "", fmt.Errorf("cannot load directory as file: %s", path)
	}

	// Read file
	content, err := os.ReadFile(fullPath)
	if err != nil {
		slog.Error("directive {{file:}} failed to read file", "path", path, "full_path", fullPath, "error", err)
		return "", fmt.Errorf("failed to read file %s: %w", path, err)
	}

	// Check for binary content (simple heuristic: null bytes)
	if hasBinaryContent(content) {
		slog.Error("directive {{file:}} cannot load binary file", "path", path, "full_path", fullPath)
		return "", fmt.Errorf("cannot load binary file: %s", path)
	}

	return string(content), nil
}

// execScript executes a script and returns stdout or formatted error
func execScript(path, agentPath string) (string, error) {
	// Execute command - shell handles all path resolution (./script.sh, /abs/path, commands in PATH, ~/home)
	stdout, stderr, exitCode, execErr := shell.Execute(path, agentPath)

	if execErr != nil {
		// Log the failure for debugging (error text is injected into content)
		slog.Error("directive {{exec:}} command failed",
			"command", path,
			"exit_code", exitCode,
			"stderr", stderr,
			"error", execErr)
		// Format error same as tool errors: "exit N: stderr"
		return fmt.Sprintf("exit %d: %s", exitCode, stderr), nil
	}

	return stdout, nil
}

// hasBinaryContent checks if data contains null bytes (simple binary detection)
func hasBinaryContent(data []byte) bool {
	for i := 0; i < len(data) && i < binaryCheckLimit; i++ {
		if data[i] == nullByte {
			return true
		}
	}
	return false
}
