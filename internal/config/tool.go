package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/BurntSushi/toml"
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
	toolsPath := filepath.Join(agentPath, "tools")

	var filesToLoad []string

	if len(toolNames) == 0 {
		// Auto-discover: load all .toml files except example.toml
		entries, err := os.ReadDir(toolsPath)
		if err != nil {
			return nil, fmt.Errorf("failed to read tools directory: %w", err)
		}

		for _, entry := range entries {
			if entry.IsDir() {
				continue
			}
			name := entry.Name()
			if strings.HasSuffix(name, ".toml") && name != "example.toml" {
				filesToLoad = append(filesToLoad, name)
			}
		}
	} else {
		// Load specified tools
		for _, name := range toolNames {
			filename := name
			if !strings.HasSuffix(filename, ".toml") {
				filename = name + ".toml"
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
		Name:       strings.TrimSuffix(filename, ".toml"),
		Parameters: make(map[string]Parameter),
	}

	// Extract command and description
	if cmd, ok := raw["command"].(string); ok {
		tool.Command = cmd
	}
	if desc, ok := raw["description"].(string); ok {
		tool.Description = desc
	}

	// Validation
	if tool.Command == "" {
		return ToolConfig{}, fmt.Errorf("command is required in %s", filename)
	}

	// Extract parameters (all other sections are parameters)
	for key, value := range raw {
		if key == "command" || key == "description" {
			continue
		}

		paramMap, ok := value.(map[string]interface{})
		if !ok {
			continue
		}

		param := Parameter{
			Type: "string", // default
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
