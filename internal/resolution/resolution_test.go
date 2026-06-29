package resolution

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestBuildSystemVariables(t *testing.T) {
	// Test with full context
	ctx := Context{
		AgentName: "test-agent",
		UserID:    "alice",
		Username:  "Alice Smith",
		SessionID: "20260624_153045_abc123",
		Interface: "cli",
		Model:     "gpt-4o",
		Provider:  "openai",
		Timestamp: time.Date(2026, 6, 24, 15, 30, 45, 0, time.UTC),
	}

	vars := buildSystemVariables(ctx)

	// Check all system variables are populated
	if vars["$agent"] != "test-agent" {
		t.Errorf("$agent = %q, want %q", vars["$agent"], "test-agent")
	}
	if vars["$user"] != "alice" {
		t.Errorf("$user = %q, want %q", vars["$user"], "alice")
	}
	if vars["$username"] != "Alice Smith" {
		t.Errorf("$username = %q, want %q", vars["$username"], "Alice Smith")
	}
	if vars["$session"] != "20260624_153045_abc123" {
		t.Errorf("$session = %q, want %q", vars["$session"], "20260624_153045_abc123")
	}
	if vars["$interface"] != "cli" {
		t.Errorf("$interface = %q, want %q", vars["$interface"], "cli")
	}
	if vars["$model"] != "gpt-4o" {
		t.Errorf("$model = %q, want %q", vars["$model"], "gpt-4o")
	}
	if vars["$provider"] != "openai" {
		t.Errorf("$provider = %q, want %q", vars["$provider"], "openai")
	}
	if vars["$timestamp"] != "2026-06-24T15:30:45Z" {
		t.Errorf("$timestamp = %q, want %q", vars["$timestamp"], "2026-06-24T15:30:45Z")
	}
	if vars["$date"] != "2026-06-24" {
		t.Errorf("$date = %q, want %q", vars["$date"], "2026-06-24")
	}
}

func TestBuildSystemVariables_EmptyContext(t *testing.T) {
	// Test with minimal context (validation mode)
	ctx := Context{
		AgentName: "test-agent",
		// All other fields are zero values
	}

	vars := buildSystemVariables(ctx)

	// Check that keys exist even with empty values
	if _, exists := vars["$agent"]; !exists {
		t.Error("$agent key missing")
	}
	if _, exists := vars["$user"]; !exists {
		t.Error("$user key missing")
	}
	if _, exists := vars["$username"]; !exists {
		t.Error("$username key missing")
	}
	if _, exists := vars["$session"]; !exists {
		t.Error("$session key missing")
	}
	if _, exists := vars["$interface"]; !exists {
		t.Error("$interface key missing")
	}
	if _, exists := vars["$timestamp"]; !exists {
		t.Error("$timestamp key missing")
	}
	if _, exists := vars["$date"]; !exists {
		t.Error("$date key missing")
	}
	if _, exists := vars["$model"]; !exists {
		t.Error("$model key missing")
	}
	if _, exists := vars["$provider"]; !exists {
		t.Error("$provider key missing")
	}

	// Check that empty values are actually empty strings
	if vars["$user"] != "" {
		t.Errorf("$user = %q, want empty string", vars["$user"])
	}
	if vars["$timestamp"] != "" {
		t.Errorf("$timestamp = %q, want empty string", vars["$timestamp"])
	}
}

func TestSubstituteVariables(t *testing.T) {
	sysVars := map[string]string{
		"$agent": "my-agent",
		"$user":  "alice",
	}
	userVars := map[string]string{
		"custom": "value",
	}

	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "system variable",
			input:    "Agent: {{$agent}}",
			expected: "Agent: my-agent",
		},
		{
			name:     "user variable",
			input:    "Custom: {{custom}}",
			expected: "Custom: value",
		},
		{
			name:     "system takes precedence",
			input:    "{{$agent}} {{$user}}",
			expected: "my-agent alice",
		},
		{
			name:     "unknown variable preserved",
			input:    "{{unknown}} text",
			expected: "{{unknown}} text",
		},
		{
			name:     "escaped placeholder",
			input:    `\{{$agent}} literal`,
			expected: "{{$agent}} literal",
		},
		{
			name:     "directive preserved (not substituted)",
			input:    "{{file:path.txt}}",
			expected: "{{file:path.txt}}",
		},
		{
			name:     "multiple variables",
			input:    "User {{$user}} on {{$agent}} with {{custom}}",
			expected: "User alice on my-agent with value",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := substituteVariables(tt.input, sysVars, userVars)
			if result != tt.expected {
				t.Errorf("substituteVariables() = %q, want %q", result, tt.expected)
			}
		})
	}
}

func TestProcess_WithDirectivesAndVariables(t *testing.T) {
	// Create temp directory for test files
	tmpDir := t.TempDir()

	// Create test file
	testFile := filepath.Join(tmpDir, "test.txt")
	if err := os.WriteFile(testFile, []byte("File content: {{$user}}"), 0644); err != nil {
		t.Fatal(err)
	}

	ctx := Context{
		AgentPath: tmpDir,
		AgentName: "test-agent",
		UserID:    "bob",
		Timestamp: time.Date(2026, 6, 24, 12, 0, 0, 0, time.UTC),
	}

	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "only variables",
			input:    "User: {{$user}}, Agent: {{$agent}}",
			expected: "User: bob, Agent: test-agent",
		},
		{
			name:     "directive then variable",
			input:    "{{file:test.txt}}",
			expected: "File content: bob",
		},
		{
			name:     "mixed content",
			input:    "Start {{$agent}} {{file:test.txt}} End",
			expected: "Start test-agent File content: bob End",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := Process(tt.input, ctx)
			if err != nil {
				t.Fatalf("Process() error = %v", err)
			}
			if result != tt.expected {
				t.Errorf("Process() = %q, want %q", result, tt.expected)
			}
		})
	}
}

func TestValidateSyntax(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		wantError bool
	}{
		{
			name:      "valid file directive",
			input:     "{{file:path.txt}}",
			wantError: false,
		},
		{
			name:      "valid exec directive",
			input:     "{{exec:./script.sh}}",
			wantError: false,
		},
		{
			name:      "valid variables",
			input:     "{{$agent}} {{user}} {{custom}}",
			wantError: false,
		},
		{
			name:      "unknown directive type",
			input:     "{{unknown:path}}",
			wantError: true,
		},
		{
			name:      "empty directive path",
			input:     "{{file:}}",
			wantError: true,
		},
		{
			name:      "space after colon",
			input:     "{{file: path.txt}}",
			wantError: true,
		},
		{
			name:      "space before closing braces",
			input:     "{{file:path.txt }}",
			wantError: true,
		},
		{
			name:      "escaped placeholder",
			input:     `\{{file:path.txt}}`,
			wantError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateSyntax(tt.input)
			if (err != nil) != tt.wantError {
				t.Errorf("ValidateSyntax() error = %v, wantError %v", err, tt.wantError)
			}
		})
	}
}

func TestProcess_EmptyContextValidation(t *testing.T) {
	// Test that processing works with minimal context (validation mode)
	ctx := Context{
		AgentPath: "/tmp/test-agent",
		AgentName: "test-agent",
		// All other fields empty
	}

	input := "Agent: {{$agent}}, User: {{$user}}, Session: {{$session}}"
	result, err := Process(input, ctx)
	if err != nil {
		t.Fatalf("Process() error = %v", err)
	}

	expected := "Agent: test-agent, User: , Session: "
	if result != expected {
		t.Errorf("Process() = %q, want %q", result, expected)
	}
}

func TestProcess_UserVarsAndSystemVars(t *testing.T) {
	ctx := Context{
		AgentPath: "/tmp/test",
		AgentName: "test-agent",
		UserID:    "alice",
		UserVars: map[string]string{
			"custom": "my-value",
			"$user":  "should-be-ignored", // System vars take precedence
		},
	}

	input := "User: {{$user}}, Custom: {{custom}}"
	result, err := Process(input, ctx)
	if err != nil {
		t.Fatalf("Process() error = %v", err)
	}

	expected := "User: alice, Custom: my-value"
	if result != expected {
		t.Errorf("Process() = %q, want %q", result, expected)
	}
}

func TestProcess_NestedDirectives(t *testing.T) {
	// Create temp directory for test files
	tmpDir := t.TempDir()

	// Create per-user directory structure
	aliceDir := filepath.Join(tmpDir, "users", "alice")
	bobDir := filepath.Join(tmpDir, "users", "bob")
	if err := os.MkdirAll(aliceDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(bobDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Create per-user files
	aliceFile := filepath.Join(aliceDir, "notes.txt")
	bobFile := filepath.Join(bobDir, "notes.txt")
	if err := os.WriteFile(aliceFile, []byte("Alice's notes"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(bobFile, []byte("Bob's notes"), 0644); err != nil {
		t.Fatal(err)
	}

	// Create per-user scripts
	scriptsDir := filepath.Join(tmpDir, "scripts")
	aliceScriptDir := filepath.Join(scriptsDir, "alice")
	bobScriptDir := filepath.Join(scriptsDir, "bob")
	if err := os.MkdirAll(aliceScriptDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(bobScriptDir, 0755); err != nil {
		t.Fatal(err)
	}

	aliceScript := filepath.Join(aliceScriptDir, "greet.sh")
	bobScript := filepath.Join(bobScriptDir, "greet.sh")
	if err := os.WriteFile(aliceScript, []byte("#!/bin/sh\necho 'Hello, Alice!'"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(bobScript, []byte("#!/bin/sh\necho 'Hello, Bob!'"), 0755); err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name     string
		userID   string
		input    string
		expected string
	}{
		{
			name:     "file directive with variable in path - alice",
			userID:   "alice",
			input:    "Notes: {{file:users/{{$user}}/notes.txt}}",
			expected: "Notes: Alice's notes",
		},
		{
			name:     "file directive with variable in path - bob",
			userID:   "bob",
			input:    "Notes: {{file:users/{{$user}}/notes.txt}}",
			expected: "Notes: Bob's notes",
		},
		{
			name:     "exec directive with variable in path - alice",
			userID:   "alice",
			input:    "Greeting: {{exec:scripts/{{$user}}/greet.sh}}",
			expected: "Greeting: Hello, Alice!\n", // echo adds newline
		},
		{
			name:     "exec directive with variable in path - bob",
			userID:   "bob",
			input:    "Greeting: {{exec:scripts/{{$user}}/greet.sh}}",
			expected: "Greeting: Hello, Bob!\n", // echo adds newline
		},
		{
			name:     "multiple nested directives",
			userID:   "alice",
			input:    "{{file:users/{{$user}}/notes.txt}} | {{exec:scripts/{{$user}}/greet.sh}}",
			expected: "Alice's notes | Hello, Alice!\n", // echo adds newline
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := Context{
				AgentPath: tmpDir,
				AgentName: "test-agent",
				UserID:    tt.userID,
				Timestamp: time.Now(),
			}

			result, err := Process(tt.input, ctx)
			if err != nil {
				t.Fatalf("Process() error = %v", err)
			}
			if result != tt.expected {
				t.Errorf("Process() = %q, want %q", result, tt.expected)
			}
		})
	}
}
