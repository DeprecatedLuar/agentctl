package agent

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/DeprecatedLuar/agentctl/internal/config"
	"github.com/DeprecatedLuar/agentctl/internal/resolution"
)

func TestExecuteTool_WorkingDirectory(t *testing.T) {
	// Create a temporary directory structure
	tmpDir := t.TempDir()
	toolsDir := filepath.Join(tmpDir, "tools")
	nestedDir := filepath.Join(toolsDir, "nested")

	if err := os.MkdirAll(nestedDir, 0755); err != nil {
		t.Fatalf("failed to create nested dir: %v", err)
	}

	// Create a test script
	scriptPath := filepath.Join(nestedDir, "test.sh")
	scriptContent := `#!/usr/bin/env bash
echo "Working directory: $(pwd)"
echo "Script location: $(dirname "$0")"
echo "Argument: $1"
`
	if err := os.WriteFile(scriptPath, []byte(scriptContent), 0755); err != nil {
		t.Fatalf("failed to create script: %v", err)
	}

	// Create a tool that uses the script
	tool := &config.ToolConfig{
		Name:    "test",
		Command: "./test.sh {{message}}",
		Path:    filepath.Join(nestedDir, "test.toml"),
	}

	// Execute the tool
	args := map[string]interface{}{
		"message": "hello",
	}

	ctx := resolution.NewValidationContext(tmpDir)
	result := ExecuteTool(tool, args, tmpDir, ctx, nil, false, false)

	// Verify the working directory was set correctly
	expectedDir := nestedDir
	if !strings.Contains(result.Output, expectedDir) {
		t.Errorf("Expected working directory to be %s, got:\n%s", expectedDir, result.Output)
	}

	// Verify the argument was passed
	if !strings.Contains(result.Output, "Argument: hello") {
		t.Errorf("Expected argument 'hello' in output, got:\n%s", result.Output)
	}
}

func TestExecuteTool_RelativePath(t *testing.T) {
	// Create a temporary directory structure
	tmpDir := t.TempDir()
	toolsDir := filepath.Join(tmpDir, "tools")
	nestedDir := filepath.Join(toolsDir, "subdir")

	if err := os.MkdirAll(nestedDir, 0755); err != nil {
		t.Fatalf("failed to create nested dir: %v", err)
	}

	// Create a helper script
	helperPath := filepath.Join(nestedDir, "helper.sh")
	if err := os.WriteFile(helperPath, []byte("#!/usr/bin/env bash\necho 'Helper works!'"), 0755); err != nil {
		t.Fatalf("failed to create helper script: %v", err)
	}

	// Create a tool that calls the helper with a relative path
	tool := &config.ToolConfig{
		Name:    "caller",
		Command: "./helper.sh",
		Path:    filepath.Join(nestedDir, "caller.toml"),
	}

	// Execute the tool
	ctx := resolution.NewValidationContext(tmpDir)
	result := ExecuteTool(tool, map[string]interface{}{}, tmpDir, ctx, nil, false, false)

	// Verify the helper was executed successfully
	if !strings.Contains(result.Output, "Helper works!") {
		t.Errorf("Expected 'Helper works!' in output, got:\n%s", result.Output)
	}
}

func TestResolveToolReport(t *testing.T) {
	ctx := resolution.NewValidationContext(t.TempDir())

	t.Run("empty report returns zero value", func(t *testing.T) {
		tool := &config.ToolConfig{Path: "/agent/tools/foo.toml"}
		exec := ExecResult{Output: "hello"}

		got, err := ResolveToolReport(tool, exec, ctx)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got != (ToolReport{}) {
			t.Errorf("expected zero ToolReport, got %+v", got)
		}
	})

	t.Run("family derived from parent folder", func(t *testing.T) {
		tool := &config.ToolConfig{
			Report: "read {{path}}",
			Path:   "/agent/tools/memory/read.toml",
		}
		exec := ExecResult{Substitutions: map[string]string{"path": "notes.md"}}

		got, err := ResolveToolReport(tool, exec, ctx)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		want := ToolReport{Family: "memory", Message: "read notes.md"}
		if got != want {
			t.Errorf("got %+v, want %+v", got, want)
		}
	})

	t.Run("top-level tool falls back to its own name as family", func(t *testing.T) {
		tool := &config.ToolConfig{
			Name:   "foo",
			Report: "ran {{name}}",
			Path:   "/agent/tools/foo.toml",
		}
		exec := ExecResult{Substitutions: map[string]string{"name": "foo"}}

		got, err := ResolveToolReport(tool, exec, ctx)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		want := ToolReport{Family: "foo", Message: "ran foo"}
		if got != want {
			t.Errorf("got %+v, want %+v", got, want)
		}
	})

	t.Run("command and result tokens substituted with truncation", func(t *testing.T) {
		tool := &config.ToolConfig{
			Name:   "foo",
			Report: "cmd={{$command}} result={{$result}}",
			Path:   "/agent/tools/foo.toml",
		}
		exec := ExecResult{
			Command: "echo hi",
			Output:  strings.Repeat("x", reportPreviewLen+50),
		}

		got, err := ResolveToolReport(tool, exec, ctx)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		wantResult := truncate(exec.Output, reportPreviewLen)
		want := ToolReport{Family: "foo", Message: "cmd=echo hi result=" + wantResult}
		if got != want {
			t.Errorf("got %+v, want %+v", got, want)
		}
	})
}
