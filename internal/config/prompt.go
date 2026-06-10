package config

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const (
	// File names
	promptFile = "prompt"

	// Section markers
	inputSectionPrefix  = "[>>"
	staticSectionPrefix = "[>"
	sectionSuffix       = "]"

	// File reference prefix
	fileReferencePrefix = "<"

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
				if err := saveMessage(&result, currentRole, currentContent.String(), isInput, agentPath, vars); err != nil {
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
				if err := saveMessage(&result, currentRole, currentContent.String(), isInput, agentPath, vars); err != nil {
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
		if err := saveMessage(&result, currentRole, currentContent.String(), isInput, agentPath, vars); err != nil {
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

func saveMessage(result *ParsedPrompt, role, content string, isInput bool, agentPath string, vars map[string]string) error {
	// Process content lines
	processedContent, err := processContent(content, agentPath, vars, !isInput)
	if err != nil {
		return err
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

	return nil
}

func processContent(content, agentPath string, vars map[string]string, substituteVars bool) (string, error) {
	var result strings.Builder
	lines := strings.Split(content, "\n")

	for i, line := range lines {
		trimmed := strings.TrimSpace(line)

		// Handle file references
		if strings.HasPrefix(trimmed, fileReferencePrefix) {
			filePath := strings.TrimSpace(trimmed[len(fileReferencePrefix):])
			fileContent, err := loadFile(filePath, agentPath)
			if err != nil {
				return "", err
			}
			if i > 0 {
				result.WriteString("\n")
			}
			result.WriteString(fileContent)
		} else {
			if i > 0 {
				result.WriteString("\n")
			}
			result.WriteString(line)
		}
	}

	finalContent := result.String()

	// Apply variable substitution if requested
	if substituteVars {
		finalContent = substituteVariables(finalContent, vars)
	}

	return finalContent, nil
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
