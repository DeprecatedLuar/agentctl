package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/BurntSushi/toml"
)

const (
	// Directory and file names
	toolsDir        = "tools"
	exampleToolFile = "example.toml"
	tomlExtension   = ".toml"

	// Reserved TOML keys (not parameters)
	keyCommand     = "command"
	keyDescription = "description"

	// Default parameter type
	defaultParameterType = "string"
)

type ToolConfig struct {
	Name        string
	Command     string               `toml:"command"`
	Description string               `toml:"description"`
	Parameters  map[string]Parameter `toml:",inline"`
}

type Parameter struct {
	Description string `toml:"description"`
	Type        string `toml:"type"`
	Required    bool   `toml:"required"`
}

func LoadTools(agentPath string, toolNames []string) ([]ToolConfig, []ValidationIssue) {
	var issues []ValidationIssue
	var tools []ToolConfig

	toolsPath := filepath.Join(agentPath, toolsDir)

	var filesToLoad []string

	if len(toolNames) == 0 {
		// Auto-discover: load all .toml files except example.toml
		entries, err := os.ReadDir(toolsPath)
		if err != nil {
			issues = append(issues, ValidationIssue{
				Type:    IssueError,
				Message: fmt.Sprintf("tools/: %v", err),
			})
			return nil, issues
		}

		for _, entry := range entries {
			if entry.IsDir() {
				continue
			}
			name := entry.Name()
			if strings.HasSuffix(name, tomlExtension) && name != exampleToolFile {
				filesToLoad = append(filesToLoad, name)
			}
		}
	} else {
		// Load specified tools - validate they exist
		for _, name := range toolNames {
			filename := name
			if !strings.HasSuffix(filename, tomlExtension) {
				filename = name + tomlExtension
			}
			toolPath := filepath.Join(toolsPath, filename)
			if _, err := os.Stat(toolPath); err != nil {
				issues = append(issues, ValidationIssue{
					Type:    IssueError,
					Message: fmt.Sprintf("tools/%s: declared in agent.toml but file not found", filename),
				})
				continue
			}
			filesToLoad = append(filesToLoad, filename)
		}
	}

	for _, filename := range filesToLoad {
		toolPath := filepath.Join(toolsPath, filename)

		tool, toolIssues := loadTool(toolPath, filename)
		issues = append(issues, toolIssues...)

		// Only add tool if no errors occurred
		hasError := false
		for _, issue := range toolIssues {
			if issue.Type == IssueError {
				hasError = true
				break
			}
		}
		if !hasError {
			tools = append(tools, tool)
		}
	}

	return tools, issues
}

func loadTool(path string, filename string) (ToolConfig, []ValidationIssue) {
	var issues []ValidationIssue

	data, err := os.ReadFile(path)
	if err != nil {
		issues = append(issues, ValidationIssue{
			Type:    IssueError,
			Message: fmt.Sprintf("tools/%s: %v", filename, err),
		})
		return ToolConfig{}, issues
	}

	// Parse as generic map to handle dynamic parameter sections
	var raw map[string]interface{}
	if err := toml.Unmarshal(data, &raw); err != nil {
		issues = append(issues, ValidationIssue{
			Type:    IssueError,
			Message: fmt.Sprintf("tools/%s: failed to parse TOML: %v", filename, err),
		})
		return ToolConfig{}, issues
	}

	tool := ToolConfig{
		Name:       strings.TrimSuffix(filename, tomlExtension),
		Parameters: make(map[string]Parameter),
	}

	// Extract command and description
	if cmd, ok := raw[keyCommand].(string); ok {
		tool.Command = cmd
	}
	if desc, ok := raw[keyDescription].(string); ok {
		tool.Description = desc
	}

	// Validation
	if tool.Command == "" {
		issues = append(issues, ValidationIssue{
			Type:    IssueError,
			Message: fmt.Sprintf("tools/%s: command field is required", filename),
		})
	}

	// Extract parameters (all other sections are parameters)
	for key, value := range raw {
		if key == keyCommand || key == keyDescription {
			continue
		}

		paramMap, ok := value.(map[string]interface{})
		if !ok {
			continue
		}

		param := Parameter{
			Type: defaultParameterType, // default
		}

		if desc, ok := paramMap["description"].(string); ok {
			param.Description = desc
		} else {
			// Warn if parameter missing description
			issues = append(issues, ValidationIssue{
				Type:    IssueWarning,
				Message: fmt.Sprintf("tools/%s: parameter '%s' missing description", filename, key),
			})
		}

		if typ, ok := paramMap["type"].(string); ok {
			param.Type = typ
		}
		if req, ok := paramMap["required"].(bool); ok {
			param.Required = req
		}

		tool.Parameters[key] = param
	}

	return tool, issues
}
