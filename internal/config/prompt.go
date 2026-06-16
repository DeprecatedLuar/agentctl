package config

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/DeprecatedLuar/agentctl/internal/directives"
)

const (
	// File names
	promptFile = "prompts/default"

	// Section markers
	inputSectionPrefix  = "[>>"
	staticSectionPrefix = "[>"
	sectionSuffix       = "]"

	// Variable placeholder markers
	varPlaceholderPrefix = "{{"
	varPlaceholderSuffix = "}}"
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

	// Validate directive syntax (no execution, just syntax check)
	if err := directives.ValidateSyntax(content); err != nil {
		issues = append(issues, ValidationIssue{
			Type:    IssueError,
			Message: fmt.Sprintf("prompt: %v", err),
		})
		return "", issues, nil
	}

	// First pass: handle {{...}} directives (actual processing happens at runtime)
	processedContent, err := directives.ProcessDirectives(content, agentPath)
	if err != nil {
		// Runtime errors during processing (file not found, etc.)
		issues = append(issues, ValidationIssue{
			Type:    IssueError,
			Message: fmt.Sprintf("prompt: %v", err),
		})
		return "", issues, nil
	}

	// Second pass: apply variable substitution if requested
	if substituteVars {
		processedContent = substituteVariables(processedContent, vars)
	}

	return processedContent, issues, nil
}

func substituteVariables(content string, vars map[string]string) string {
	result := content
	for key, value := range vars {
		placeholder := varPlaceholderPrefix + key + varPlaceholderSuffix
		result = strings.ReplaceAll(result, placeholder, value)
	}
	return result
}
