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
	if !strings.Contains(result, expectedDir) {
		t.Errorf("Expected working directory to be %s, got:\n%s", expectedDir, result)
	}

	// Verify the argument was passed
	if !strings.Contains(result, "Argument: hello") {
		t.Errorf("Expected argument 'hello' in output, got:\n%s", result)
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
	if !strings.Contains(result, "Helper works!") {
		t.Errorf("Expected 'Helper works!' in output, got:\n%s", result)
	}
}
