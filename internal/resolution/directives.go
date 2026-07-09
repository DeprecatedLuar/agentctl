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
// Supports variable substitution in directive arguments: {{file:path/{{$user}}/file.md}}
//
// Directive types:
//   - {{file:path}}: Loads file content (relative to ctx.BaseDir)
//   - {{exec:path}}: Executes script, returns stdout (or "exit N: stderr" on failure), run from ctx.BaseDir
//
// ctx.BaseDir is the directory relative paths resolve against for directives found in the
// content currently being processed. When a {{file:}} directive loads another file, its content
// is resolved with BaseDir set to that file's own directory, so directives nested inside behave
// as if they "cd"'d into the including file's location (mirrors how tools/*.toml commands run
// from their own directory rather than the agent root).
//
// Returns processed content and error if directive processing fails.
func processDirectives(content string, ctx Context) (string, error) {
	return processDirectivesWithDepth(content, ctx, 0)
}

// validateSyntaxWithDepth validates directive syntax with depth limit.
//
// Note: syntax-only validation can't catch nested syntax errors (e.g. inside a
// {{file:}}'s target) until runtime, since that requires actually loading the
// content. That's acceptable since the first level is always validated up front,
// and full nested validation happens during processDirectives.
func validateSyntaxWithDepth(content string, depth int) error {
	if depth > maxDepth {
		return fmt.Errorf("directive nesting too deep (max %d levels)", maxDepth)
	}

	pos := 0
	for {
		tok := scanBraceToken(content, pos)

		switch tok.kind {
		case tokenNone, tokenUnterminated:
			return nil

		case tokenEscaped:
			pos = tok.start + len(prefix)

		case tokenVariable:
			pos = tok.end + len(suffix)

		case tokenDirective:
			if err := validateDirectivePathSyntax(tok.directiveType, tok.directivePath); err != nil {
				return err
			}
			if err := validateDirectiveType(tok.directiveType, tok.directivePath); err != nil {
				return err
			}
			pos = tok.end + len(suffix)
		}
	}
}

// tokenKind classifies what scanBraceToken found at the next {{ in a string.
type tokenKind int

const (
	tokenNone         tokenKind = iota // no more {{ in the remainder of the string
	tokenEscaped                       // a \{{ escape sequence (literal {{, not a placeholder)
	tokenUnterminated                  // a {{ with no matching }}
	tokenDirective                     // a {{type:path}} directive
	tokenVariable                      // a {{name}} variable placeholder (no ':')
)

// braceToken is one {{...}} construct found while scanning content, along with
// enough position/content info for callers to either validate or substitute it.
type braceToken struct {
	kind          tokenKind
	start         int    // index of "{{" (set for all kinds except tokenNone)
	end           int    // index of "}}" (set for tokenDirective/tokenVariable)
	inner         string // raw content between the braces (tokenDirective/tokenVariable)
	directiveType string // inner before the ':' (tokenDirective only)
	directivePath string // inner after the ':' (tokenDirective only)
}

// scanBraceToken finds the next {{...}} construct in content starting at pos.
// It shares the escape-detection and nested-brace-matching rules used by both
// validateSyntaxWithDepth and processDirectivesWithDepth, and additionally
// extracts the directive type/path split so callers don't repeat that either.
func scanBraceToken(content string, pos int) braceToken {
	start := strings.Index(content[pos:], prefix)
	if start == -1 {
		return braceToken{kind: tokenNone}
	}
	start += pos // Adjust to absolute position

	// Check for escape sequence \{{
	if start > 0 && content[start-1] == byte(escapeChar) {
		return braceToken{kind: tokenEscaped, start: start}
	}

	// Find matching closing }} (handle nested {{...}})
	endOffset := findMatchingCloseBrace(content[start+len(prefix):])
	if endOffset == -1 {
		return braceToken{kind: tokenUnterminated, start: start}
	}
	end := start + len(prefix) + endOffset // Position of }}
	inner := content[start+len(prefix) : end]

	// Check if this is a directive (contains :) or a plain variable
	colonIdx := strings.IndexByte(inner, byte(directiveSep))
	if colonIdx == -1 {
		return braceToken{kind: tokenVariable, start: start, end: end, inner: inner}
	}

	return braceToken{
		kind:          tokenDirective,
		start:         start,
		end:           end,
		inner:         inner,
		directiveType: inner[:colonIdx],
		directivePath: inner[colonIdx+1:],
	}
}

// validateDirectivePathSyntax checks a directive path for the strict syntax rules
// shared by validation and processing: no empty paths, no leading/trailing spaces.
func validateDirectivePathSyntax(directiveType, directivePath string) error {
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
	return nil
}

// validateDirectiveType checks that directiveType is a known directive (syntax check
// only - it does not execute anything).
func validateDirectiveType(directiveType, directivePath string) error {
	switch directiveType {
	case "file", "exec":
		return nil
	default:
		return fmt.Errorf("{{%s:%s}}: unknown directive type '%s' (valid: file, exec)", directiveType, directivePath, directiveType)
	}
}

// findMatchingCloseBrace finds the position of the matching }} for nested {{...}}
// Returns the offset from the start of the search string, or -1 if not found
func findMatchingCloseBrace(content string) int {
	depth := 1 // We already consumed one {{
	i := 0

	for i < len(content)-1 {
		// Check for opening {{
		if i < len(content)-1 && content[i:i+2] == prefix {
			depth++
			i += 2
			continue
		}

		// Check for closing }}
		if i < len(content)-1 && content[i:i+2] == suffix {
			depth--
			if depth == 0 {
				return i // Found matching close
			}
			i += 2
			continue
		}

		i++
	}

	return -1 // No matching close found
}

// processDirectivesWithDepth handles directives with recursion depth limit
func processDirectivesWithDepth(content string, ctx Context, depth int) (string, error) {
	if depth > maxDepth {
		return "", fmt.Errorf("directive nesting too deep (max %d levels)", maxDepth)
	}

	// Build system variables for substitution in directive arguments
	// Skip variable substitution if in validation mode (empty UserID means minimal context)
	isValidationMode := ctx.UserID == ""
	var sysVars map[string]string
	var userVars map[string]string

	if !isValidationMode {
		sysVars = buildSystemVariables(ctx)
		userVars = ctx.UserVars
		if userVars == nil {
			userVars = make(map[string]string)
		}
	}

	var result strings.Builder
	pos := 0

	// Process all {{...}} patterns
	for {
		tok := scanBraceToken(content, pos)

		switch tok.kind {
		case tokenNone, tokenUnterminated:
			// No more placeholders (or an unterminated one) - append rest and stop
			result.WriteString(content[pos:])
			return result.String(), nil

		case tokenEscaped:
			// Remove backslash and keep {{ as literal
			result.WriteString(content[pos : tok.start-1]) // Write up to (but not including) backslash
			result.WriteString(prefix)                       // Write literal {{
			pos = tok.start + len(prefix)
			continue

		case tokenVariable:
			// No colon - this is a variable placeholder, keep as-is
			result.WriteString(content[pos:tok.start])
			result.WriteString(prefix)
			result.WriteString(tok.inner)
			result.WriteString(suffix)
			pos = tok.end + len(suffix)
			continue
		}

		// tokenDirective - extract type and path
		result.WriteString(content[pos:tok.start])
		directiveType := tok.directiveType
		directivePath := tok.directivePath

		// Substitute variables in directive path (e.g., {{file:path/{{$user}}/file.md}})
		// Skip during validation mode to preserve variables
		if !isValidationMode {
			directivePath = substituteVariables(directivePath, sysVars, userVars)
		}

		// Skip directives with variables during validation mode
		if isValidationMode && strings.Contains(directivePath, "{{") {
			// Can't process directives with runtime variables during validation
			// Keep as-is and continue (don't mark as processed to avoid recursion)
			result.WriteString(prefix)
			result.WriteString(tok.inner)
			result.WriteString(suffix)
			pos = tok.end + len(suffix)
			continue
		}

		// Validate path syntax (strict: no spaces, no empty paths)
		if err := validateDirectivePathSyntax(directiveType, directivePath); err != nil {
			return "", err
		}

		var replacement string
		var err error

		switch directiveType {
		case "file":
			// Load file content, then resolve any directives inside it relative to
			// its own directory (so it behaves as if we "cd"'d into it).
			fileContent, fileDir, loadErr := loadFile(directivePath, ctx.BaseDir)
			if loadErr != nil {
				return "", fmt.Errorf("{{file:%s}}: %w", directivePath, loadErr)
			}
			fileCtx := ctx
			fileCtx.BaseDir = fileDir
			replacement, err = processDirectivesWithDepth(fileContent, fileCtx, depth+1)
			if err != nil {
				return "", err
			}

		case "exec":
			// Execute script from ctx.BaseDir, then resolve any directives in its output
			// with the same base dir (the script wasn't loaded from a file of its own).
			execOutput, execErr := execScript(directivePath, ctx.BaseDir, ctx.AgentPath, ctx.ConfigEnv)
			if execErr != nil {
				return "", fmt.Errorf("{{exec:%s}}: %w", directivePath, execErr)
			}
			replacement, err = processDirectivesWithDepth(execOutput, ctx, depth+1)
			if err != nil {
				return "", err
			}

		default:
			// Unknown directive type
			return "", validateDirectiveType(directiveType, directivePath)
		}

		// Append replacement content
		result.WriteString(replacement)
		pos = tok.end + len(suffix)
	}
}

// loadFile loads file content with path resolution relative to baseDir.
// Also returns the directory containing the loaded file, so callers can resolve
// any directives nested inside its content relative to that file's own location.
func loadFile(path, baseDir string) (content string, fileDir string, err error) {
	// Resolve path
	var fullPath string
	if strings.HasPrefix(path, homeDirPrefix) {
		home, homeErr := os.UserHomeDir()
		if homeErr != nil {
			slog.Error("directive {{file:}} failed to get home directory", "path", path, "error", homeErr)
			return "", "", fmt.Errorf("failed to get home directory: %w", homeErr)
		}
		fullPath = filepath.Join(home, path[len(homeDirPrefix):])
	} else if filepath.IsAbs(path) {
		fullPath = path
	} else {
		fullPath = filepath.Join(baseDir, path)
	}

	// Check if file exists and is not a directory
	info, err := os.Stat(fullPath)
	if err != nil {
		slog.Error("directive {{file:}} failed to load file", "path", path, "full_path", fullPath, "error", err)
		return "", "", fmt.Errorf("failed to load file %s: %w", path, err)
	}
	if info.IsDir() {
		slog.Error("directive {{file:}} cannot load directory", "path", path, "full_path", fullPath)
		return "", "", fmt.Errorf("cannot load directory as file: %s", path)
	}

	// Read file
	fileContent, err := os.ReadFile(fullPath)
	if err != nil {
		slog.Error("directive {{file:}} failed to read file", "path", path, "full_path", fullPath, "error", err)
		return "", "", fmt.Errorf("failed to read file %s: %w", path, err)
	}

	// Check for binary content (simple heuristic: null bytes)
	if hasBinaryContent(fileContent) {
		slog.Error("directive {{file:}} cannot load binary file", "path", path, "full_path", fullPath)
		return "", "", fmt.Errorf("cannot load binary file: %s", path)
	}

	return string(fileContent), filepath.Dir(fullPath), nil
}

// execScript executes a script from workDir and returns stdout or formatted error.
// agentFolder (the agent root) is passed separately for .env/config loading, mirroring
// how tool commands separate their working directory from the agent's env context.
func execScript(path, workDir, agentFolder string, configEnv map[string]string) (string, error) {
	// Execute command - shell handles all path resolution (./script.sh, /abs/path, commands in PATH, ~/home)
	stdout, stderr, exitCode, execErr := shell.Execute(path, workDir, agentFolder, configEnv, nil)

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
