package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDirectives(t *testing.T) {
	// Create temp test directory
	tmpDir := t.TempDir()
	
	// Create test files
	contextContent := "Context from file.\nNested: {{exec:./script.sh}}"
	if err := os.WriteFile(filepath.Join(tmpDir, "context.md"), []byte(contextContent), 0644); err != nil {
		t.Fatal(err)
	}
	
	scriptContent := "#!/bin/sh\necho 'script output'"
	scriptPath := filepath.Join(tmpDir, "script.sh")
	if err := os.WriteFile(scriptPath, []byte(scriptContent), 0755); err != nil {
		t.Fatal(err)
	}
	
	// Create prompt file
	promptContent := `[>system]
File: {{file:./context.md}}
Var: {{myvar}}

[>>user]
{{input}}`
	
	os.MkdirAll(filepath.Join(tmpDir, "prompts"), 0755)
	if err := os.WriteFile(filepath.Join(tmpDir, "prompts", "default"), []byte(promptContent), 0644); err != nil {
		t.Fatal(err)
	}
	
	// Parse prompt
	vars := map[string]string{"myvar": "REPLACED"}
	prompt, issues := Parse(tmpDir, vars)
	
	// Check for errors
	for _, issue := range issues {
		if issue.Type == IssueError {
			t.Fatalf("Parse error: %s", issue.Message)
		}
	}
	
	if prompt == nil {
		t.Fatal("prompt is nil")
	}
	
	if len(prompt.Static) == 0 {
		t.Fatal("no static messages")
	}
	
	content := prompt.Static[0].Content
	
	// Verify {{file:}} was processed
	if !strings.Contains(content, "Context from file.") {
		t.Errorf("{{file:}} directive not processed")
	}
	
	// Verify nested {{exec:}} was processed
	if !strings.Contains(content, "script output") {
		t.Errorf("nested {{exec:}} directive not processed")
	}
	
	// Verify variable was substituted
	if !strings.Contains(content, "REPLACED") {
		t.Errorf("variable substitution failed")
	}
	
	// Verify {{input}} was preserved in input section
	if prompt.Input == nil || !strings.Contains(prompt.Input.Content, "{{input}}") {
		t.Errorf("{{input}} should be preserved in input section")
	}
	
	t.Logf("System message:\n%s", content)
}

func TestDirectiveErrors(t *testing.T) {
	tmpDir := t.TempDir()

	// Test unknown directive
	t.Run("unknown directive", func(t *testing.T) {
		promptContent := `[>system]
{{unknown:./file}}`
		os.MkdirAll(filepath.Join(tmpDir, "prompts"), 0755)
		os.WriteFile(filepath.Join(tmpDir, "prompts", "default"), []byte(promptContent), 0644)

		_, issues := Parse(tmpDir, nil)

		hasError := false
		for _, issue := range issues {
			if issue.Type == IssueError && strings.Contains(issue.Message, "unknown directive") {
				hasError = true
			}
		}

		if !hasError {
			t.Error("expected error for unknown directive")
		}
	})

	// Test failing script
	t.Run("failing script", func(t *testing.T) {
		failScript := "#!/bin/sh\necho 'error message' >&2\nexit 42"
		os.WriteFile(filepath.Join(tmpDir, "fail.sh"), []byte(failScript), 0755)

		promptContent := `[>system]
Result: {{exec:./fail.sh}}`
		os.MkdirAll(filepath.Join(tmpDir, "prompts"), 0755)
		os.WriteFile(filepath.Join(tmpDir, "prompts", "default"), []byte(promptContent), 0644)

		prompt, _ := Parse(tmpDir, nil)

		if prompt == nil || len(prompt.Static) == 0 {
			t.Fatal("prompt should parse even with failing script")
		}

		content := prompt.Static[0].Content

		// Should contain exit code and stderr in "exit N: stderr" format
		if !strings.Contains(content, "exit 42") || !strings.Contains(content, "error message") {
			t.Errorf("failing script should inject 'exit N: stderr' format, got: %s", content)
		}
	})
}
