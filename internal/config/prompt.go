package config

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/DeprecatedLuar/agentctl/internal/shell"
)

const (
	// File names
	promptFile = "prompt"

	// Section markers
	inputSectionPrefix  = "[>>"
	staticSectionPrefix = "[>"
	sectionSuffix       = "]"

	// Variable placeholder markers
	varPlaceholderPrefix = "{{"
	varPlaceholderSuffix = "}}"

	// Binary content check
	binaryCheckLimit = 512
	nullByte         = 0

	// Home directory prefix
	homeDirPrefix = "~/"
)


type ParsedPrompt struct {
	Static []Message
	Input  *Message
}

type Message struct {
	Role    string
	Content string
}

func Parse(agentPath string, vars map[string]string) (*ParsedPrompt, []ValidationIssue) {
	var issues []ValidationIssue
	promptPath := filepath.Join(agentPath, promptFile)

	file, err := os.Open(promptPath)
	if err != nil {
		issues = append(issues, ValidationIssue{
			Type:    IssueWarning,
			Message: fmt.Sprintf("prompt: %v", err),
		})
		return nil, issues
	}
	defer file.Close()

	var result ParsedPrompt
	var currentRole string
	var currentContent strings.Builder
	var isInput bool

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()

		// Check for section headers
		if strings.HasPrefix(line, inputSectionPrefix) && strings.HasSuffix(line, sectionSuffix) {
			// Save previous section if exists
			if currentRole != "" {
				sectionIssues, err := saveMessage(&result, currentRole, currentContent.String(), isInput, agentPath, vars)
				issues = append(issues, sectionIssues...)
				if err != nil {
					issues = append(issues, ValidationIssue{
						Type:    IssueError,
						Message: fmt.Sprintf("prompt: %v", err),
					})
					return nil, issues
				}
			}

			// Start new input section
			if result.Input != nil {
				issues = append(issues, ValidationIssue{
					Type:    IssueError,
					Message: fmt.Sprintf("prompt: multiple %s sections found, only one allowed", inputSectionPrefix),
				})
				return nil, issues
			}
			currentRole = strings.TrimSpace(line[len(inputSectionPrefix) : len(line)-len(sectionSuffix)])
			currentContent.Reset()
			isInput = true

		} else if strings.HasPrefix(line, staticSectionPrefix) && strings.HasSuffix(line, sectionSuffix) {
			// Save previous section if exists
			if currentRole != "" {
				sectionIssues, err := saveMessage(&result, currentRole, currentContent.String(), isInput, agentPath, vars)
				issues = append(issues, sectionIssues...)
				if err != nil {
					issues = append(issues, ValidationIssue{
						Type:    IssueError,
						Message: fmt.Sprintf("prompt: %v", err),
					})
					return nil, issues
				}
			}

			// Start new static section
			currentRole = strings.TrimSpace(line[len(staticSectionPrefix) : len(line)-len(sectionSuffix)])
			currentContent.Reset()
			isInput = false

		} else {
			// Content line
			if currentContent.Len() > 0 {
				currentContent.WriteString("\n")
			}
			currentContent.WriteString(line)
		}
	}

	// Save last section
	if currentRole != "" {
		sectionIssues, err := saveMessage(&result, currentRole, currentContent.String(), isInput, agentPath, vars)
		issues = append(issues, sectionIssues...)
		if err != nil {
			issues = append(issues, ValidationIssue{
				Type:    IssueError,
				Message: fmt.Sprintf("prompt: %v", err),
			})
			return nil, issues
		}
	}

	if err := scanner.Err(); err != nil {
		issues = append(issues, ValidationIssue{
			Type:    IssueError,
			Message: fmt.Sprintf("prompt: error reading file: %v", err),
		})
		return nil, issues
	}

	// Warn if no input section
	if result.Input == nil {
		issues = append(issues, ValidationIssue{
			Type:    IssueWarning,
			Message: "prompt: no [>>] section found — memory injection disabled",
		})
	}

	return &result, issues
}

func saveMessage(result *ParsedPrompt, role, content string, isInput bool, agentPath string, vars map[string]string) ([]ValidationIssue, error) {
	// Process content lines
	processedContent, issues, err := processContent(content, agentPath, vars, !isInput)
	if err != nil {
		return issues, err
	}

	msg := Message{
		Role:    role,
		Content: processedContent,
	}

	if isInput {
		result.Input = &msg
	} else {
		result.Static = append(result.Static, msg)
	}

	return issues, nil
}

func processContent(content, agentPath string, vars map[string]string, substituteVars bool) (string, []ValidationIssue, error) {
	var issues []ValidationIssue

	// First pass: handle {{...}} directives
	processedContent, directiveIssues, err := processDirectives(content, agentPath)
	if err != nil {
		return "", issues, err
	}
	issues = append(issues, directiveIssues...)

	// Second pass: apply variable substitution if requested
	if substituteVars {
		processedContent = substituteVariables(processedContent, vars)
	}

	return processedContent, issues, nil
}

// processDirectives handles {{file:path}} and {{exec:path}} directives
func processDirectives(content, agentPath string) (string, []ValidationIssue, error) {
	return processDirectivesWithDepth(content, agentPath, 0)
}

// processDirectivesWithDepth handles directives with recursion depth limit
func processDirectivesWithDepth(content, agentPath string, depth int) (string, []ValidationIssue, error) {
	const maxDepth = 10 // Prevent infinite recursion

	if depth > maxDepth {
		return "", nil, fmt.Errorf("directive nesting too deep (max %d levels)", maxDepth)
	}

	var issues []ValidationIssue
	var result strings.Builder
	pos := 0
	hasDirectives := false

	// Process all {{...}} patterns
	for {
		// Find next {{
		start := strings.Index(content[pos:], varPlaceholderPrefix)
		if start == -1 {
			// No more placeholders - append rest of content
			result.WriteString(content[pos:])
			break
		}
		start += pos // Adjust to absolute position

		// Find closing }}
		endOffset := strings.Index(content[start+len(varPlaceholderPrefix):], varPlaceholderSuffix)
		if endOffset == -1 {
			// No closing }} - append rest and stop
			result.WriteString(content[pos:])
			break
		}
		end := start + len(varPlaceholderPrefix) + endOffset // Position of }}

		// Append content before placeholder
		result.WriteString(content[pos:start])

		// Extract inner content
		inner := content[start+len(varPlaceholderPrefix) : end]

		// Check if this is a directive (contains :) or a variable
		colonIdx := strings.Index(inner, ":")
		if colonIdx == -1 {
			// No colon - this is a variable placeholder, keep as-is
			result.WriteString(varPlaceholderPrefix)
			result.WriteString(inner)
			result.WriteString(varPlaceholderSuffix)
			pos = end + len(varPlaceholderSuffix)
			continue
		}

		// This is a directive - extract type and path
		hasDirectives = true
		directiveType := inner[:colonIdx]
		directivePath := inner[colonIdx+1:]

		var replacement string
		var err error

		switch directiveType {
		case "file":
			// Load file content
			replacement, err = loadFile(directivePath, agentPath)
			if err != nil {
				return "", issues, fmt.Errorf("{{file:%s}}: %w", directivePath, err)
			}

		case "exec":
			// Resolve script path
			var scriptPath string
			if strings.HasPrefix(directivePath, homeDirPrefix) {
				home, homeErr := os.UserHomeDir()
				if homeErr != nil {
					return "", issues, fmt.Errorf("{{exec:%s}}: failed to get home directory: %w", directivePath, homeErr)
				}
				scriptPath = filepath.Join(home, directivePath[len(homeDirPrefix):])
			} else if filepath.IsAbs(directivePath) {
				scriptPath = directivePath
			} else {
				scriptPath = filepath.Join(agentPath, directivePath)
			}

			// Execute script
			stdout, stderr, exitCode, execErr := shell.Execute(scriptPath, agentPath)
			if execErr != nil {
				// Format error same as tool errors: "exit N: stderr"
				replacement = fmt.Sprintf("exit %d: %s", exitCode, stderr)
			} else {
				replacement = stdout
			}

		default:
			// Unknown directive type
			return "", issues, fmt.Errorf("{{%s:%s}}: unknown directive type '%s'", directiveType, directivePath, directiveType)
		}

		// Append replacement content
		result.WriteString(replacement)
		pos = end + len(varPlaceholderSuffix)
	}

	finalResult := result.String()

	// If we processed any directives, recursively process the result
	// to handle nested directives (e.g., {{file:x.md}} where x.md contains {{exec:y.sh}})
	if hasDirectives {
		return processDirectivesWithDepth(finalResult, agentPath, depth+1)
	}

	return finalResult, issues, nil
}

func loadFile(path, agentPath string) (string, error) {
	// Resolve path
	var fullPath string
	if strings.HasPrefix(path, homeDirPrefix) {
		home, err := os.UserHomeDir()
		if err != nil {
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
		return "", fmt.Errorf("failed to load file %s: %w", path, err)
	}
	if info.IsDir() {
		return "", fmt.Errorf("cannot load directory as file: %s", path)
	}

	// Read file
	content, err := os.ReadFile(fullPath)
	if err != nil {
		return "", fmt.Errorf("failed to read file %s: %w", path, err)
	}

	// Check for binary content (simple heuristic: null bytes)
	if hasBinaryContent(content) {
		return "", fmt.Errorf("cannot load binary file: %s", path)
	}

	return string(content), nil
}

func hasBinaryContent(data []byte) bool {
	for i := 0; i < len(data) && i < binaryCheckLimit; i++ {
		if data[i] == nullByte {
			return true
		}
	}
	return false
}

func substituteVariables(content string, vars map[string]string) string {
	result := content
	for key, value := range vars {
		placeholder := varPlaceholderPrefix + key + varPlaceholderSuffix
		result = strings.ReplaceAll(result, placeholder, value)
	}
	return result
}
