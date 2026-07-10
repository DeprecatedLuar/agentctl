package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/DeprecatedLuar/agentctl/internal/resolution"
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
	if err := os.WriteFile(filepath.Join(tmpDir, "prompts", "chat_template"), []byte(promptContent), 0644); err != nil {
		t.Fatal(err)
	}
	
	// Parse prompt
	ctx := resolution.Context{
		AgentPath: tmpDir,
		BaseDir:   tmpDir,
		AgentName: "test",
		UserVars:  map[string]string{"myvar": "REPLACED"},
	}
	prompt, issues := Parse(tmpDir, ctx)
	
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
		os.WriteFile(filepath.Join(tmpDir, "prompts", "chat_template"), []byte(promptContent), 0644)

		ctx := resolution.Context{AgentPath: tmpDir, BaseDir: tmpDir}
	_, issues := Parse(tmpDir, ctx)

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
		os.WriteFile(filepath.Join(tmpDir, "prompts", "chat_template"), []byte(promptContent), 0644)

		ctx := resolution.Context{AgentPath: tmpDir, BaseDir: tmpDir}
	prompt, _ := Parse(tmpDir, ctx)

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

// TestDirectives_FileRelativeNesting verifies that a {{file:}} directive loaded from
// within another file resolves relative paths against that file's own directory
// (not the agent root), while the top-level prompts/chat_template itself still
// falls back to agent root.
func TestDirectives_FileRelativeNesting(t *testing.T) {
	tmpDir := t.TempDir()

	// sibling.md lives next to the file that includes it, under prompts/, not agent root.
	os.MkdirAll(filepath.Join(tmpDir, "prompts"), 0755)
	if err := os.WriteFile(filepath.Join(tmpDir, "prompts", "system.md"), []byte("Included: {{file:./sibling.md}}"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(tmpDir, "prompts", "sibling.md"), []byte("sibling content"), 0644); err != nil {
		t.Fatal(err)
	}
	// A same-named file at agent root proves resolution isn't falling back there.
	if err := os.WriteFile(filepath.Join(tmpDir, "sibling.md"), []byte("WRONG agent-root content"), 0644); err != nil {
		t.Fatal(err)
	}

	promptContent := `[>system]
{{file:./prompts/system.md}}`
	if err := os.WriteFile(filepath.Join(tmpDir, "prompts", "chat_template"), []byte(promptContent), 0644); err != nil {
		t.Fatal(err)
	}

	ctx := resolution.Context{AgentPath: tmpDir, BaseDir: tmpDir}
	prompt, issues := Parse(tmpDir, ctx)
	for _, issue := range issues {
		if issue.Type == IssueError {
			t.Fatalf("Parse error: %s", issue.Message)
		}
	}

	content := prompt.Static[0].Content
	if !strings.Contains(content, "sibling content") {
		t.Errorf("expected nested {{file:./sibling.md}} to resolve relative to prompts/ dir, got: %s", content)
	}
	if strings.Contains(content, "WRONG agent-root content") {
		t.Errorf("nested {{file:./sibling.md}} incorrectly resolved against agent root, got: %s", content)
	}
}

// TestResolvePromptPath_ExtensionPriority verifies chat_template (no extension) is
// preferred over chat_template.md when both exist, and that chat_template.md is
// used as a fallback when only it exists.
func TestResolvePromptPath_ExtensionPriority(t *testing.T) {
	t.Run("no-extension wins when both exist", func(t *testing.T) {
		tmpDir := t.TempDir()
		os.MkdirAll(filepath.Join(tmpDir, "prompts"), 0755)
		os.WriteFile(filepath.Join(tmpDir, "prompts", "chat_template"), []byte("[>system]\nplain"), 0644)
		os.WriteFile(filepath.Join(tmpDir, "prompts", "chat_template.md"), []byte("[>system]\nmarkdown"), 0644)

		ctx := resolution.Context{AgentPath: tmpDir, BaseDir: tmpDir}
		prompt, issues := Parse(tmpDir, ctx)
		for _, issue := range issues {
			if issue.Type == IssueError {
				t.Fatalf("Parse error: %s", issue.Message)
			}
		}
		if !strings.Contains(prompt.Static[0].Content, "plain") {
			t.Errorf("expected extensionless chat_template to win, got: %s", prompt.Static[0].Content)
		}
	})

	t.Run("falls back to .md when no-extension is absent", func(t *testing.T) {
		tmpDir := t.TempDir()
		os.MkdirAll(filepath.Join(tmpDir, "prompts"), 0755)
		os.WriteFile(filepath.Join(tmpDir, "prompts", "chat_template.md"), []byte("[>system]\nmarkdown"), 0644)

		ctx := resolution.Context{AgentPath: tmpDir, BaseDir: tmpDir}
		prompt, issues := Parse(tmpDir, ctx)
		for _, issue := range issues {
			if issue.Type == IssueError {
				t.Fatalf("Parse error: %s", issue.Message)
			}
		}
		if !strings.Contains(prompt.Static[0].Content, "markdown") {
			t.Errorf("expected chat_template.md fallback, got: %s", prompt.Static[0].Content)
		}
	})
}
