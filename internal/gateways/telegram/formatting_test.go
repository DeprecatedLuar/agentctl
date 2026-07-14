package telegram

import (
	"testing"
)

func TestFormatForTelegram(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "bold text",
			input:    "This is **bold** text",
			expected: "This is <b>bold</b> text",
		},
		{
			name:     "italic text",
			input:    "This is *italic* text",
			expected: "This is <i>italic</i> text",
		},
		{
			name:     "inline code",
			input:    "Use `code` here",
			expected: "Use <code>code</code> here",
		},
		{
			name:     "code block",
			input:    "```code block```",
			expected: "<pre>code block</pre>",
		},
		{
			name:     "mixed formatting",
			input:    "Use **bold** with `code` examples",
			expected: "Use <b>bold</b> with <code>code</code> examples",
		},
		{
			name:     "HTML escaping",
			input:    "Explain <html> tags & symbols",
			expected: "Explain &lt;html&gt; tags &amp; symbols",
		},
		{
			name:     "plain text unchanged",
			input:    "Plain text without formatting",
			expected: "Plain text without formatting",
		},
		{
			name:     "underscores inside inline code are not treated as italic",
			input:    "Use `snake_case_var` here",
			expected: "Use <code>snake_case_var</code> here",
		},
		{
			name:     "underscores straddling two code spans don't produce overlapping tags",
			input:    "Use `x_` and `_y` here",
			expected: "Use <code>x_</code> and <code>_y</code> here",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := formatForTelegram(tt.input)
			if result != tt.expected {
				t.Errorf("formatForTelegram() = %q, want %q", result, tt.expected)
			}
		})
	}
}
