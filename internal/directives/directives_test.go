package directives

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestProcessDirectives_EscapeSequence(t *testing.T) {
	tmpDir := t.TempDir()

	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "escape single directive",
			input:    `This is a \{{file:test.txt}} literal`,
			expected: `This is a {{file:test.txt}} literal`,
		},
		{
			name:     "escape exec directive",
			input:    `Run \{{exec:./script.sh}} to process`,
			expected: `Run {{exec:./script.sh}} to process`,
		},
		{
			name:     "escape variable placeholder",
			input:    `Use \{{variable}} in your config`,
			expected: `Use {{variable}} in your config`,
		},
		{
			name:     "multiple escapes",
			input:    `\{{file:a}} and \{{exec:b}} are literals`,
			expected: `{{file:a}} and {{exec:b}} are literals`,
		},
		{
			name:     "mixed escaped and unescaped",
			input:    `\{{file:escaped}} but {{var}} is not`,
			expected: `{{file:escaped}} but {{var}} is not`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := ProcessDirectives(tt.input, tmpDir)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if result != tt.expected {
				t.Errorf("expected %q, got %q", tt.expected, result)
			}
		})
	}
}

func TestProcessDirectives_FileDirective(t *testing.T) {
	tmpDir := t.TempDir()

	// Create a test file
	testFile := filepath.Join(tmpDir, "test.txt")
	if err := os.WriteFile(testFile, []byte("hello world"), 0644); err != nil {
		t.Fatalf("failed to create test file: %v", err)
	}

	input := "Content: {{file:./test.txt}}"
	result, err := ProcessDirectives(input, tmpDir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	expected := "Content: hello world"
	if result != expected {
		t.Errorf("expected %q, got %q", expected, result)
	}
}

func TestProcessDirectives_ExecDirective(t *testing.T) {
	tmpDir := t.TempDir()

	// Create a test script
	script := filepath.Join(tmpDir, "test.sh")
	if err := os.WriteFile(script, []byte("#!/bin/sh\necho hello"), 0755); err != nil {
		t.Fatalf("failed to create script: %v", err)
	}

	input := "Time: {{exec:./test.sh}}"
	result, err := ProcessDirectives(input, tmpDir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// echo adds a newline, so we need to trim
	expected := "Time: hello\n"
	if result != expected {
		t.Errorf("expected %q, got %q", expected, result)
	}
}

func TestProcessDirectives_NestedDirectives(t *testing.T) {
	tmpDir := t.TempDir()

	// Create a test script
	script := filepath.Join(tmpDir, "nested.sh")
	if err := os.WriteFile(script, []byte("#!/bin/sh\necho nested"), 0755); err != nil {
		t.Fatalf("failed to create script: %v", err)
	}

	// Create a file that contains another directive
	nestedFile := filepath.Join(tmpDir, "nested.txt")
	if err := os.WriteFile(nestedFile, []byte("Value: {{exec:./nested.sh}}"), 0644); err != nil {
		t.Fatalf("failed to create nested file: %v", err)
	}

	input := "{{file:./nested.txt}}"
	result, err := ProcessDirectives(input, tmpDir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	expected := "Value: nested\n"
	if result != expected {
		t.Errorf("expected %q, got %q", expected, result)
	}
}

func TestProcessDirectives_PreserveVariables(t *testing.T) {
	tmpDir := t.TempDir()

	// Variables (no colon) should be preserved
	input := "Use {{variable}} and {{another_var}} here"
	result, err := ProcessDirectives(input, tmpDir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result != input {
		t.Errorf("variables should be preserved: expected %q, got %q", input, result)
	}
}


func TestProcessDirectives_ErrorHandling(t *testing.T) {
	tmpDir := t.TempDir()

	tests := []struct {
		name          string
		input         string
		expectError   bool
		errorContains string
		resultContains string
	}{
		{
			name:          "file not found",
			input:         "{{file:./nonexistent.txt}}",
			expectError:   true,
			errorContains: "failed to load file",
		},
		{
			name:          "unknown directive",
			input:         "{{unknown:./file.txt}}",
			expectError:   true,
			errorContains: "unknown directive type",
		},
		{
			name:          "empty path",
			input:         "{{file:}}",
			expectError:   true,
			errorContains: "empty path not allowed",
		},
		{
			name:          "space after colon",
			input:         "{{exec: ./script.sh}}",
			expectError:   true,
			errorContains: "space after colon not allowed",
		},
		{
			name:          "space before closing braces",
			input:         "{{file:./test.txt }}",
			expectError:   true,
			errorContains: "space before closing braces not allowed",
		},
		{
			name:           "exec script not found",
			input:          "{{exec:./nonexistent.sh}}",
			expectError:    false, // execScript returns formatted error as value, not error
			resultContains: "exit",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := ProcessDirectives(tt.input, tmpDir)

			if tt.expectError {
				if err == nil {
					t.Fatal("expected error, got none")
				}
				if !strings.Contains(err.Error(), tt.errorContains) {
					t.Errorf("expected error containing %q, got %q", tt.errorContains, err.Error())
				}
			} else {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				if tt.resultContains != "" && !strings.Contains(result, tt.resultContains) {
					t.Errorf("expected result containing %q, got %q", tt.resultContains, result)
				}
			}
		})
	}
}

func TestProcessDirectives_DepthLimit(t *testing.T) {
	tmpDir := t.TempDir()

	// Create files that reference each other in a chain
	for i := 0; i < 12; i++ {
		filename := filepath.Join(tmpDir, filepath.Base(tmpDir)+"."+string(rune('a'+i)))
		nextFile := filepath.Base(tmpDir) + "." + string(rune('a'+i+1))
		content := "{{file:./" + nextFile + "}}"
		if err := os.WriteFile(filename, []byte(content), 0644); err != nil {
			t.Fatalf("failed to create file: %v", err)
		}
	}

	// Try to process first file (will recurse deeply)
	firstFile := filepath.Base(tmpDir) + ".a"
	input := "{{file:./" + firstFile + "}}"
	_, err := ProcessDirectives(input, tmpDir)

	if err == nil {
		t.Fatal("expected depth limit error, got none")
	}

	if !strings.Contains(err.Error(), "too deep") {
		t.Errorf("expected 'too deep' error, got: %v", err)
	}
}
