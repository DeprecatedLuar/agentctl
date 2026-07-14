package gateways

import "testing"

func TestProtectCodeSpans(t *testing.T) {
	wrapBlock := func(content string) string { return "[BLOCK:" + content + "]" }
	wrapCode := func(content string) string { return "[CODE:" + content + "]" }

	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "inline code with underscores stays intact",
			input:    "Use `snake_case_var` here",
			expected: "Use [CODE:snake_case_var] here",
		},
		{
			name:     "underscores straddling two code spans don't merge",
			input:    "Use `x_` and `_y` here",
			expected: "Use [CODE:x_] and [CODE:_y] here",
		},
		{
			name:     "code block extracted before inline code",
			input:    "```block```  and `inline`",
			expected: "[BLOCK:block]  and [CODE:inline]",
		},
		{
			name:     "no code spans leaves text untouched",
			input:    "Plain text, no code",
			expected: "Plain text, no code",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			protected, restore := ProtectCodeSpans(tt.input, wrapBlock, wrapCode)
			result := restore(protected)
			if result != tt.expected {
				t.Errorf("ProtectCodeSpans() = %q, want %q", result, tt.expected)
			}
		})
	}
}

// TestProtectCodeSpansShieldsFromMarkdown verifies the actual bug scenario:
// bold/italic conversion run between extraction and restoration must not be
// able to reach into code content or across code span boundaries.
func TestProtectCodeSpansShieldsFromMarkdown(t *testing.T) {
	input := "Use `x_` and `_y` here"
	wrapBlock := func(content string) string { return "<pre>" + content + "</pre>" }
	wrapCode := func(content string) string { return "<code>" + content + "</code>" }

	protected, restore := ProtectCodeSpans(input, wrapBlock, wrapCode)

	// Simulate a markdown pass that would otherwise treat the underscores
	// left over in the surrounding text as italic markers - the placeholders
	// contain no markdown-significant characters, so this must be a no-op.
	protected = ApplyBold(protected, "<b>$1</b>")

	result := restore(protected)
	expected := "Use <code>x_</code> and <code>_y</code> here"
	if result != expected {
		t.Errorf("got %q, want %q", result, expected)
	}
}
