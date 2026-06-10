package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParse_BasicSections(t *testing.T) {
	tmpDir := t.TempDir()

	promptContent := `[>system]
You are a helpful assistant.

[>user]
Hello

[>>user]
{{input}}`

	promptPath := filepath.Join(tmpDir, "prompt")
	if err := os.WriteFile(promptPath, []byte(promptContent), 0644); err != nil {
		t.Fatal(err)
	}

	result, issues := Parse(tmpDir, map[string]string{"test": "value"})
	for _, issue := range issues {
		if issue.Type == IssueError {
			t.Fatalf("unexpected error: %v", issue.Message)
		}
	}

	if len(result.Static) != 2 {
		t.Errorf("expected 2 static messages, got %d", len(result.Static))
	}

	if result.Static[0].Role != "system" {
		t.Errorf("expected first role to be 'system', got '%s'", result.Static[0].Role)
	}
	if !strings.Contains(result.Static[0].Content, "helpful assistant") {
		t.Errorf("unexpected content: %s", result.Static[0].Content)
	}

	if result.Static[1].Role != "user" {
		t.Errorf("expected second role to be 'user', got '%s'", result.Static[1].Role)
	}

	if result.Input == nil {
		t.Fatal("expected input message to be present")
	}
	if result.Input.Role != "user" {
		t.Errorf("expected input role to be 'user', got '%s'", result.Input.Role)
	}
	if !strings.Contains(result.Input.Content, "{{input}}") {
		t.Errorf("expected {{input}} placeholder to remain, got: %s", result.Input.Content)
	}
}

func TestParse_VariableSubstitution(t *testing.T) {
	tmpDir := t.TempDir()

	promptContent := `[>system]
You are {{role}}.

[>>user]
{{input}}`

	promptPath := filepath.Join(tmpDir, "prompt")
	if err := os.WriteFile(promptPath, []byte(promptContent), 0644); err != nil {
		t.Fatal(err)
	}

	vars := map[string]string{
		"role": "a test assistant",
	}

	result, issues := Parse(tmpDir, vars)
	for _, issue := range issues {
		if issue.Type == IssueError {
			t.Fatalf("unexpected error: %v", issue.Message)
		}
	}

	// Static messages should have variables substituted
	if !strings.Contains(result.Static[0].Content, "a test assistant") {
		t.Errorf("expected variable substitution in static message, got: %s", result.Static[0].Content)
	}
	if strings.Contains(result.Static[0].Content, "{{role}}") {
		t.Errorf("expected {{role}} to be substituted, got: %s", result.Static[0].Content)
	}

	// Input message should keep placeholders
	if !strings.Contains(result.Input.Content, "{{input}}") {
		t.Errorf("expected {{input}} to remain in input message, got: %s", result.Input.Content)
	}
}

func TestParse_FileReference(t *testing.T) {
	tmpDir := t.TempDir()

	// Create a file to reference
	refContent := "This is referenced content."
	refPath := filepath.Join(tmpDir, "reference.txt")
	if err := os.WriteFile(refPath, []byte(refContent), 0644); err != nil {
		t.Fatal(err)
	}

	promptContent := `[>system]
<reference.txt

[>>user]
{{input}}`

	promptPath := filepath.Join(tmpDir, "prompt")
	if err := os.WriteFile(promptPath, []byte(promptContent), 0644); err != nil {
		t.Fatal(err)
	}

	result, issues := Parse(tmpDir, nil)
	for _, issue := range issues {
		if issue.Type == IssueError {
			t.Fatalf("unexpected error: %v", issue.Message)
		}
	}

	if !strings.Contains(result.Static[0].Content, "This is referenced content.") {
		t.Errorf("expected file content to be included, got: %s", result.Static[0].Content)
	}
}

func TestParse_FileNotFound(t *testing.T) {
	tmpDir := t.TempDir()

	promptContent := `[>system]
<nonexistent.txt`

	promptPath := filepath.Join(tmpDir, "prompt")
	if err := os.WriteFile(promptPath, []byte(promptContent), 0644); err != nil {
		t.Fatal(err)
	}

	_, issues := Parse(tmpDir, nil)
	if len(issues) == 0 {
		t.Fatal("expected validation issues for missing file")
	}
	found := false
	for _, issue := range issues {
		if strings.Contains(issue.Message, "nonexistent.txt") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected issue to mention file name, got: %v", issues)
	}
}

func TestParse_MultipleInputSections(t *testing.T) {
	tmpDir := t.TempDir()

	promptContent := `[>>user]
First input

[>>user]
Second input`

	promptPath := filepath.Join(tmpDir, "prompt")
	if err := os.WriteFile(promptPath, []byte(promptContent), 0644); err != nil {
		t.Fatal(err)
	}

	_, issues := Parse(tmpDir, nil)
	if len(issues) == 0 {
		t.Fatal("expected validation issues for multiple [>>] sections")
	}
	found := false
	for _, issue := range issues {
		if strings.Contains(issue.Message, "multiple") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected issue about multiple sections, got: %v", issues)
	}
}

func TestParse_NoInputSection(t *testing.T) {
	tmpDir := t.TempDir()

	promptContent := `[>system]
Only static content.`

	promptPath := filepath.Join(tmpDir, "prompt")
	if err := os.WriteFile(promptPath, []byte(promptContent), 0644); err != nil {
		t.Fatal(err)
	}

	result, issues := Parse(tmpDir, nil)
	for _, issue := range issues {
		if issue.Type == IssueError {
			t.Fatalf("unexpected error: %v", issue.Message)
		}
	}

	if result.Input != nil {
		t.Error("expected input to be nil when no [>>] section")
	}
	// Note: warning is printed to stderr, not returned as error
}

func TestParse_AbsoluteFilePath(t *testing.T) {
	tmpDir := t.TempDir()
	refPath := filepath.Join(tmpDir, "absolute.txt")
	if err := os.WriteFile(refPath, []byte("absolute content"), 0644); err != nil {
		t.Fatal(err)
	}

	promptContent := "[>system]\n<" + refPath

	promptPath := filepath.Join(tmpDir, "prompt")
	if err := os.WriteFile(promptPath, []byte(promptContent), 0644); err != nil {
		t.Fatal(err)
	}

	result, issues := Parse(tmpDir, nil)
	for _, issue := range issues {
		if issue.Type == IssueError {
			t.Fatalf("unexpected error: %v", issue.Message)
		}
	}

	if !strings.Contains(result.Static[0].Content, "absolute content") {
		t.Errorf("expected absolute path content, got: %s", result.Static[0].Content)
	}
}

func TestParse_BinaryFile(t *testing.T) {
	tmpDir := t.TempDir()

	// Create a binary file (with null bytes)
	binaryPath := filepath.Join(tmpDir, "binary.bin")
	binaryContent := []byte{0x00, 0x01, 0x02, 0xFF}
	if err := os.WriteFile(binaryPath, binaryContent, 0644); err != nil {
		t.Fatal(err)
	}

	promptContent := `[>system]
<binary.bin`

	promptPath := filepath.Join(tmpDir, "prompt")
	if err := os.WriteFile(promptPath, []byte(promptContent), 0644); err != nil {
		t.Fatal(err)
	}

	_, issues := Parse(tmpDir, nil)
	if len(issues) == 0 {
		t.Fatal("expected validation issues for binary file")
	}
	found := false
	for _, issue := range issues {
		if strings.Contains(issue.Message, "binary") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected issue about binary file, got: %v", issues)
	}
}

func TestParse_EmptyPrompt(t *testing.T) {
	tmpDir := t.TempDir()

	promptPath := filepath.Join(tmpDir, "prompt")
	if err := os.WriteFile(promptPath, []byte(""), 0644); err != nil {
		t.Fatal(err)
	}

	result, issues := Parse(tmpDir, nil)
	for _, issue := range issues {
		if issue.Type == IssueError {
			t.Fatalf("unexpected error: %v", issue.Message)
		}
	}

	if len(result.Static) != 0 {
		t.Errorf("expected no static messages, got %d", len(result.Static))
	}
	if result.Input != nil {
		t.Error("expected no input message")
	}
}
