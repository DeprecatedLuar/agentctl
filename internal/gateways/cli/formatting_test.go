package cli

import (
	"testing"
)

func TestFormatForCLI(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		contains []string // Check for ANSI codes or plain text
	}{
		{
			name:     "bold text",
			input:    "This is **bold** text",
			contains: []string{"bold"}, // Will have ANSI codes in TTY
		},
		{
			name:     "italic text",
			input:    "This is *italic* text",
			contains: []string{"italic"},
		},
		{
			name:     "inline code",
			input:    "Use `code` here",
			contains: []string{"code"},
		},
		{
			name:     "mixed formatting",
			input:    "Use **bold** with `code` examples",
			contains: []string{"bold", "code", "examples"},
		},
		{
			name:     "plain text unchanged",
			input:    "Plain text without formatting",
			contains: []string{"Plain text without formatting"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := FormatForCLI(tt.input)

			// Verify content is present (may or may not have ANSI codes depending on TTY)
			for _, substr := range tt.contains {
				if !contains(result, substr) {
					t.Errorf("FormatForCLI() result missing substring %q in output %q", substr, result)
				}
			}
		})
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > len(substr) && findSubstring(s, substr))
}

func findSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
