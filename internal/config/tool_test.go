package config

import (
	"os"
	"path/filepath"
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
