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

func LoadTools(agentPath string, toolNames []string) ([]ToolConfig, error) {
	toolsPath := filepath.Join(agentPath, toolsDir)

	var filesToLoad []string

	if len(toolNames) == 0 {
		// Auto-discover: load all .toml files except example.toml
		entries, err := os.ReadDir(toolsPath)
		if err != nil {
			return nil, fmt.Errorf("failed to read %s directory: %w", toolsDir, err)
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
		// Load specified tools
		for _, name := range toolNames {
			filename := name
			if !strings.HasSuffix(filename, tomlExtension) {
				filename = name + tomlExtension
			}
			filesToLoad = append(filesToLoad, filename)
		}
	}

	var tools []ToolConfig
	for _, filename := range filesToLoad {
		toolPath := filepath.Join(toolsPath, filename)

		tool, err := loadTool(toolPath, filename)
		if err != nil {
			return nil, err
		}

		tools = append(tools, tool)
	}

	return tools, nil
}

func loadTool(path string, filename string) (ToolConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return ToolConfig{}, fmt.Errorf("failed to read %s: %w", filename, err)
	}

	// Parse as generic map to handle dynamic parameter sections
	var raw map[string]interface{}
	if err := toml.Unmarshal(data, &raw); err != nil {
		return ToolConfig{}, fmt.Errorf("failed to parse %s: %w", filename, err)
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
		return ToolConfig{}, fmt.Errorf("%s is required in %s", keyCommand, filename)
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
		}
		if typ, ok := paramMap["type"].(string); ok {
			param.Type = typ
		}
		if req, ok := paramMap["required"].(bool); ok {
			param.Required = req
		}

		tool.Parameters[key] = param
	}

	return tool, nil
}
