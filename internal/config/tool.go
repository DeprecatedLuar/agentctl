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
	Path        string               // Absolute path to the .toml file
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
		// Auto-discover: recursively load all .toml files except example.toml
		err := filepath.WalkDir(toolsPath, func(path string, d os.DirEntry, err error) error {
			if err != nil {
				return err
			}
			// Skip directories
			if d.IsDir() {
				return nil
			}
			// Skip example.toml at any depth
			if d.Name() == exampleToolFile {
				return nil
			}
			// Collect .toml files with their full path
			if strings.HasSuffix(d.Name(), tomlExtension) {
				filesToLoad = append(filesToLoad, path)
			}
			return nil
		})
		if err != nil {
			issues = append(issues, ValidationIssue{
				Type:    IssueError,
				Message: fmt.Sprintf("tools/: %v", err),
			})
			return nil, issues
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
			filesToLoad = append(filesToLoad, toolPath)
		}
	}

	for _, toolPath := range filesToLoad {
		tool, toolIssues := loadTool(toolPath, toolsPath)
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

func loadTool(path string, toolsBasePath string) (ToolConfig, []ValidationIssue) {
	var issues []ValidationIssue

	// Extract filename and compute relative path for error messages
	filename := filepath.Base(path)
	relPath := strings.TrimPrefix(path, toolsBasePath)
	relPath = strings.TrimPrefix(relPath, string(filepath.Separator)) // Remove leading separator
	if relPath == "" {
		relPath = filename
	}

	data, err := os.ReadFile(path)
	if err != nil {
		issues = append(issues, ValidationIssue{
			Type:    IssueError,
			Message: fmt.Sprintf("tools/%s: %v", relPath, err),
		})
		return ToolConfig{}, issues
	}

	// Parse as generic map to handle dynamic parameter sections
	var raw map[string]interface{}
	if err := toml.Unmarshal(data, &raw); err != nil {
		issues = append(issues, ValidationIssue{
			Type:    IssueError,
			Message: fmt.Sprintf("tools/%s: failed to parse TOML: %v", relPath, err),
		})
		return ToolConfig{}, issues
	}

	tool := ToolConfig{
		Name:       strings.TrimSuffix(filename, tomlExtension),
		Path:       path, // Store absolute path
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
			Message: fmt.Sprintf("tools/%s: command field is required", relPath),
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
				Message: fmt.Sprintf("tools/%s: parameter '%s' missing description", relPath, key),
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
