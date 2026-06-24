package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadTools_RecursiveDiscovery(t *testing.T) {
	// Create a temporary agent folder with nested tools
	tmpDir := t.TempDir()
	toolsDir := filepath.Join(tmpDir, "tools")

	// Create tools directory structure
	if err := os.MkdirAll(filepath.Join(toolsDir, "nested"), 0755); err != nil {
		t.Fatalf("failed to create nested dir: %v", err)
	}

	// Create a flat tool
	flatTool := filepath.Join(toolsDir, "flat.toml")
	if err := os.WriteFile(flatTool, []byte(`command = "echo flat"`), 0644); err != nil {
		t.Fatalf("failed to create flat tool: %v", err)
	}

	// Create a nested tool
	nestedTool := filepath.Join(toolsDir, "nested", "nested.toml")
	if err := os.WriteFile(nestedTool, []byte(`command = "./script.sh"`), 0644); err != nil {
		t.Fatalf("failed to create nested tool: %v", err)
	}

	// Create example.toml (should be skipped)
	exampleTool := filepath.Join(toolsDir, "example.toml")
	if err := os.WriteFile(exampleTool, []byte(`command = "echo example"`), 0644); err != nil {
		t.Fatalf("failed to create example tool: %v", err)
	}

	// Create nested example.toml (should also be skipped)
	nestedExample := filepath.Join(toolsDir, "nested", "example.toml")
	if err := os.WriteFile(nestedExample, []byte(`command = "echo example"`), 0644); err != nil {
		t.Fatalf("failed to create nested example: %v", err)
	}

	// Load tools (auto-discover mode)
	tools, issues := LoadTools(tmpDir, []string{})

	// Check for errors
	for _, issue := range issues {
		if issue.Type == IssueError {
			t.Errorf("unexpected error: %s", issue.Message)
		}
	}

	// Should load exactly 2 tools (flat and nested, skipping both examples)
	if len(tools) != 2 {
		t.Fatalf("expected 2 tools, got %d", len(tools))
	}

	// Find tools by name
	var flatFound, nestedFound bool
	for _, tool := range tools {
		switch tool.Name {
		case "flat":
			flatFound = true
			if tool.Command != "echo flat" {
				t.Errorf("flat tool: expected command 'echo flat', got '%s'", tool.Command)
			}
			if tool.Path == "" {
				t.Error("flat tool: Path should be set")
			}
			if filepath.Base(tool.Path) != "flat.toml" {
				t.Errorf("flat tool: expected path to end with 'flat.toml', got '%s'", tool.Path)
			}

		case "nested":
			nestedFound = true
			if tool.Command != "./script.sh" {
				t.Errorf("nested tool: expected command './script.sh', got '%s'", tool.Command)
			}
			if tool.Path == "" {
				t.Error("nested tool: Path should be set")
			}
			if filepath.Base(tool.Path) != "nested.toml" {
				t.Errorf("nested tool: expected path to end with 'nested.toml', got '%s'", tool.Path)
			}
			expectedDir := filepath.Join(toolsDir, "nested")
			actualDir := filepath.Dir(tool.Path)
			if actualDir != expectedDir {
				t.Errorf("nested tool: expected dir '%s', got '%s'", expectedDir, actualDir)
			}
		}
	}

	if !flatFound {
		t.Error("flat tool not found")
	}
	if !nestedFound {
		t.Error("nested tool not found")
	}
}

func TestLoadTools_ExplicitTools(t *testing.T) {
	// Create a temporary agent folder
	tmpDir := t.TempDir()
	toolsDir := filepath.Join(tmpDir, "tools")

	if err := os.MkdirAll(toolsDir, 0755); err != nil {
		t.Fatalf("failed to create tools dir: %v", err)
	}

	// Create a tool
	toolPath := filepath.Join(toolsDir, "mytool.toml")
	if err := os.WriteFile(toolPath, []byte(`command = "echo test"`), 0644); err != nil {
		t.Fatalf("failed to create tool: %v", err)
	}

	// Load specific tool by name
	tools, issues := LoadTools(tmpDir, []string{"mytool"})

	// Check for errors
	for _, issue := range issues {
		if issue.Type == IssueError {
			t.Errorf("unexpected error: %s", issue.Message)
		}
	}

	if len(tools) != 1 {
		t.Fatalf("expected 1 tool, got %d", len(tools))
	}

	if tools[0].Name != "mytool" {
		t.Errorf("expected tool name 'mytool', got '%s'", tools[0].Name)
	}

	if tools[0].Path == "" {
		t.Error("tool Path should be set")
	}
}

func TestLoadTools_MissingTool(t *testing.T) {
	tmpDir := t.TempDir()
	toolsDir := filepath.Join(tmpDir, "tools")

	if err := os.MkdirAll(toolsDir, 0755); err != nil {
		t.Fatalf("failed to create tools dir: %v", err)
	}

	// Try to load a tool that doesn't exist
	_, issues := LoadTools(tmpDir, []string{"nonexistent"})

	// Should have an error
	hasError := false
	for _, issue := range issues {
		if issue.Type == IssueError {
			hasError = true
			break
		}
	}

	if !hasError {
		t.Error("expected error for missing tool, got none")
	}
}

func TestLoadTools_EnabledParameter(t *testing.T) {
	tmpDir := t.TempDir()
	toolsDir := filepath.Join(tmpDir, "tools")

	if err := os.MkdirAll(toolsDir, 0755); err != nil {
		t.Fatalf("failed to create tools dir: %v", err)
	}

	// Create tool with enabled, disabled, and default (missing) parameters
	toolContent := `
command = "test {{enabled_param}} {{disabled_param}} {{default_param}}"
description = "Test tool with enabled field"

[enabled_param]
description = "Explicitly enabled parameter"
type = "string"
required = true
enabled = true

[disabled_param]
description = "Disabled parameter (hidden from AI)"
type = "string"
required = false
enabled = false

[default_param]
description = "Default parameter (no enabled field)"
type = "string"
required = false
`

	toolPath := filepath.Join(toolsDir, "test.toml")
	if err := os.WriteFile(toolPath, []byte(toolContent), 0644); err != nil {
		t.Fatalf("failed to create test tool: %v", err)
	}

	// Load the tool
	tools, issues := LoadTools(tmpDir, []string{})

	// Check for errors
	for _, issue := range issues {
		if issue.Type == IssueError {
			t.Errorf("unexpected error: %s", issue.Message)
		}
	}

	if len(tools) != 1 {
		t.Fatalf("expected 1 tool, got %d", len(tools))
	}

	tool := tools[0]

	// Verify all three parameters exist
	if len(tool.Parameters) != 3 {
		t.Fatalf("expected 3 parameters, got %d", len(tool.Parameters))
	}

	// Check enabled_param (explicitly true)
	enabledParam, ok := tool.Parameters["enabled_param"]
	if !ok {
		t.Fatal("enabled_param not found")
	}
	if !enabledParam.Enabled {
		t.Error("enabled_param should be enabled (explicitly set to true)")
	}

	// Check disabled_param (explicitly false)
	disabledParam, ok := tool.Parameters["disabled_param"]
	if !ok {
		t.Fatal("disabled_param not found")
	}
	if disabledParam.Enabled {
		t.Error("disabled_param should be disabled (explicitly set to false)")
	}

	// Check default_param (missing field, should default to true)
	defaultParam, ok := tool.Parameters["default_param"]
	if !ok {
		t.Fatal("default_param not found")
	}
	if !defaultParam.Enabled {
		t.Error("default_param should be enabled (default value when field is missing)")
	}
}

func TestLoadTools_ReturnField(t *testing.T) {
	tmpDir := t.TempDir()
	toolsDir := filepath.Join(tmpDir, "tools")

	if err := os.MkdirAll(toolsDir, 0755); err != nil {
		t.Fatalf("failed to create tools dir: %v", err)
	}

	// Create a test file for {{file:}} directive
	testFile := filepath.Join(tmpDir, "test.txt")
	if err := os.WriteFile(testFile, []byte("file-content"), 0644); err != nil {
		t.Fatalf("failed to create test file: %v", err)
	}

	// Create tool with various return field configurations
	toolContent := `
command = "echo {{literal}} {{with_file}} {{with_exec}} {{ai_controlled}}"
description = "Test tool with return field"

[literal]
description = "Literal return value"
type = "string"
required = true
return = "hardcoded-value"

[with_file]
description = "Return with file directive"
type = "string"
required = false
return = "{{file:./test.txt}}"

[with_exec]
description = "Return with exec directive"
type = "string"
required = false
return = "{{exec:echo hello}}"

[ai_controlled]
description = "AI-controlled parameter (no return)"
type = "string"
required = false

[disabled_with_return]
description = "Disabled parameter with return (should not be used)"
type = "string"
enabled = false
return = "should-not-appear"
`

	toolPath := filepath.Join(toolsDir, "test.toml")
	if err := os.WriteFile(toolPath, []byte(toolContent), 0644); err != nil {
		t.Fatalf("failed to create test tool: %v", err)
	}

	// Load the tool
	tools, issues := LoadTools(tmpDir, []string{})

	// Check for errors
	for _, issue := range issues {
		if issue.Type == IssueError {
			t.Errorf("unexpected error: %s", issue.Message)
		}
	}

	if len(tools) != 1 {
		t.Fatalf("expected 1 tool, got %d", len(tools))
	}

	tool := tools[0]

	// Verify all parameters parsed correctly
	if len(tool.Parameters) != 5 {
		t.Fatalf("expected 5 parameters, got %d", len(tool.Parameters))
	}

	// Check literal return
	if param, ok := tool.Parameters["literal"]; !ok {
		t.Error("literal parameter not found")
	} else if param.Return != "hardcoded-value" {
		t.Errorf("literal return: expected 'hardcoded-value', got '%s'", param.Return)
	}

	// Check file directive return
	if param, ok := tool.Parameters["with_file"]; !ok {
		t.Error("with_file parameter not found")
	} else if param.Return != "{{file:./test.txt}}" {
		t.Errorf("with_file return: expected '{{file:./test.txt}}', got '%s'", param.Return)
	}

	// Check exec directive return
	if param, ok := tool.Parameters["with_exec"]; !ok {
		t.Error("with_exec parameter not found")
	} else if param.Return != "{{exec:echo hello}}" {
		t.Errorf("with_exec return: expected '{{exec:echo hello}}', got '%s'", param.Return)
	}

	// Check AI-controlled parameter (no return)
	if param, ok := tool.Parameters["ai_controlled"]; !ok {
		t.Error("ai_controlled parameter not found")
	} else if param.Return != "" {
		t.Errorf("ai_controlled return: expected empty string, got '%s'", param.Return)
	}

	// Check disabled with return
	if param, ok := tool.Parameters["disabled_with_return"]; !ok {
		t.Error("disabled_with_return parameter not found")
	} else {
		if param.Enabled {
			t.Error("disabled_with_return should be disabled")
		}
		if param.Return != "should-not-appear" {
			t.Errorf("disabled_with_return return: expected 'should-not-appear', got '%s'", param.Return)
		}
	}
}

func TestLoadTools_ReturnFieldInvalidDirective(t *testing.T) {
	tmpDir := t.TempDir()
	toolsDir := filepath.Join(tmpDir, "tools")

	if err := os.MkdirAll(toolsDir, 0755); err != nil {
		t.Fatalf("failed to create tools dir: %v", err)
	}

	// Create tool with invalid directive in return field
	toolContent := `
command = "echo {{test}}"
description = "Test tool"

[test]
description = "Parameter with invalid directive"
type = "string"
return = "{{unknown:./file.txt}}"
`

	toolPath := filepath.Join(toolsDir, "test.toml")
	if err := os.WriteFile(toolPath, []byte(toolContent), 0644); err != nil {
		t.Fatalf("failed to create test tool: %v", err)
	}

	// Load the tool
	_, issues := LoadTools(tmpDir, []string{})

	// Should have an error for unknown directive
	hasError := false
	for _, issue := range issues {
		if issue.Type == IssueError && strings.Contains(issue.Message, "unknown directive") {
			hasError = true
			break
		}
	}

	if !hasError {
		t.Error("expected error for unknown directive type, got none")
	}
}

func TestConvertToolParameters(t *testing.T) {
	tests := []struct {
		name           string
		tool           ToolConfig
		expectVisible  []string // Parameters that should be visible to AI
		expectHidden   []string // Parameters that should be hidden from AI
		expectRequired []string // Parameters that should be required
	}{
		{
			name: "default enabled behavior",
			tool: ToolConfig{
				Parameters: map[string]Parameter{
					"normal": {
						Description: "Normal param",
						Type:        "string",
						Required:    true,
						Enabled:     true, // default
					},
					"with_default": {
						Description: "Param with enabled not specified (defaults to true)",
						Type:        "string",
						Required:    false,
						Enabled:     true, // This is what we expect from the parser
					},
				},
			},
			expectVisible:  []string{"normal", "with_default"},
			expectHidden:   []string{},
			expectRequired: []string{"normal"},
		},
		{
			name: "explicitly disabled parameter",
			tool: ToolConfig{
				Parameters: map[string]Parameter{
					"visible": {
						Description: "Visible param",
						Type:        "string",
						Enabled:     true,
					},
					"hidden": {
						Description: "Hidden param",
						Type:        "string",
						Enabled:     false, // explicitly disabled
					},
				},
			},
			expectVisible:  []string{"visible"},
			expectHidden:   []string{"hidden"},
			expectRequired: []string{},
		},
		{
			name: "return field without completion (blackbox)",
			tool: ToolConfig{
				Parameters: map[string]Parameter{
					"api_key": {
						Description: "API key from file",
						Type:        "string",
						Enabled:     true,
						Return:      "{{file:.env.KEY}}", // Has return, no {{$completion}}
					},
					"normal": {
						Description: "Normal param",
						Type:        "string",
						Enabled:     true,
					},
				},
			},
			expectVisible:  []string{"normal"},
			expectHidden:   []string{"api_key"}, // Hidden because return without {{$completion}}
			expectRequired: []string{},
		},
		{
			name: "return field WITH completion (AI provides value)",
			tool: ToolConfig{
				Parameters: map[string]Parameter{
					"flag": {
						Description: "Optional flag",
						Type:        "string",
						Required:    false,
						Enabled:     true, // Default when not specified
						Return:      "--flag {{$completion}}",
					},
					"another": {
						Description: "Another param",
						Type:        "string",
						Required:    true,
						Enabled:     true,
					},
				},
			},
			expectVisible:  []string{"flag", "another"}, // Both visible
			expectHidden:   []string{},
			expectRequired: []string{"another"},
		},
		{
			name: "disabled with return (both hide mechanisms)",
			tool: ToolConfig{
				Parameters: map[string]Parameter{
					"disabled_and_return": {
						Description: "Disabled + return",
						Type:        "string",
						Enabled:     false,
						Return:      "value",
					},
				},
			},
			expectVisible:  []string{},
			expectHidden:   []string{"disabled_and_return"},
			expectRequired: []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			properties, required := ConvertToolParameters(&tt.tool)

			// Check visible parameters
			for _, paramName := range tt.expectVisible {
				if _, ok := properties[paramName]; !ok {
					t.Errorf("expected parameter '%s' to be visible, but it was hidden", paramName)
				}
			}

			// Check hidden parameters
			for _, paramName := range tt.expectHidden {
				if _, ok := properties[paramName]; ok {
					t.Errorf("expected parameter '%s' to be hidden, but it was visible", paramName)
				}
			}

			// Check required parameters
			if len(required) != len(tt.expectRequired) {
				t.Errorf("expected %d required params, got %d: %v", len(tt.expectRequired), len(required), required)
			}
			for _, paramName := range tt.expectRequired {
				found := false
				for _, r := range required {
					if r == paramName {
						found = true
						break
					}
				}
				if !found {
					t.Errorf("expected parameter '%s' to be required, but it wasn't", paramName)
				}
			}
		})
	}
}
