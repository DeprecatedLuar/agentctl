package cli

import (
	"os"
	"regexp"
)

const (
	// ANSI escape codes
	ansiReset         = "\033[0m"
	ansiBold          = "\033[1m"
	ansiItalic        = "\033[3m"
	ansiDim           = "\033[2m"
	ansiStrikethrough = "\033[9m"
	ansiCyan          = "\033[36m"  // For code blocks
)

var (
	// Pre-compile regex patterns for performance
	// Order matters: process bold-italic first, then bold, then italic
	boldItalicPattern1 = regexp.MustCompile(`\*\*\*(.+?)\*\*\*`)        // ***text***
	boldItalicPattern2 = regexp.MustCompile(`__\*(.+?)\*__`)            // __*text*__
	boldItalicPattern3 = regexp.MustCompile(`\*\*_(.+?)_\*\*`)          // **_text_**
	boldPattern1       = regexp.MustCompile(`\*\*(.+?)\*\*`)            // **text**
	boldPattern2       = regexp.MustCompile(`__(.+?)__`)                // __text__
	italicPattern1     = regexp.MustCompile(`(?:^|[^*_])\*([^*]+?)\*(?:[^*]|$)`)   // *text*
	italicPattern2     = regexp.MustCompile(`(?:^|[^*_])_([^_]+?)_(?:[^_]|$)`)     // _text_
	strikePattern      = regexp.MustCompile(`~~(.+?)~~`)                // ~~strike~~
	codePattern        = regexp.MustCompile("`(.+?)`")                  // `code`
	codeBlockPattern   = regexp.MustCompile("```(?:\n)?(.+?)(?:\n)?```") // ```code```
)

// isTerminal checks if stdout is connected to a terminal (not a pipe/redirect)
func isTerminal() bool {
	fileInfo, err := os.Stdout.Stat()
	if err != nil {
		return false
	}
	return (fileInfo.Mode() & os.ModeCharDevice) != 0
}

// FormatForCLI converts markdown to ANSI codes if stdout is a terminal
func FormatForCLI(text string) string {
	if !isTerminal() {
		// Plain text for pipes/redirects
		return text
	}

	// Convert markdown to ANSI codes (order matters: bold-italic first, then bold, then italic)

	// ***bold-italic*** -> bold+italic ANSI
	text = boldItalicPattern1.ReplaceAllString(text, ansiBold+ansiItalic+"$1"+ansiReset)
	text = boldItalicPattern2.ReplaceAllString(text, ansiBold+ansiItalic+"$1"+ansiReset)
	text = boldItalicPattern3.ReplaceAllString(text, ansiBold+ansiItalic+"$1"+ansiReset)

	// **bold** or __bold__ -> bold ANSI
	text = boldPattern1.ReplaceAllString(text, ansiBold+"$1"+ansiReset)
	text = boldPattern2.ReplaceAllString(text, ansiBold+"$1"+ansiReset)

	// *italic* or _italic_ -> italic ANSI
	text = italicPattern1.ReplaceAllString(text, ansiItalic+"$1"+ansiReset)
	text = italicPattern2.ReplaceAllString(text, ansiItalic+"$1"+ansiReset)

	// ~~strikethrough~~ -> strikethrough ANSI
	text = strikePattern.ReplaceAllString(text, ansiStrikethrough+"$1"+ansiReset)

	// `code` -> dim ANSI (inline code)
	text = codePattern.ReplaceAllString(text, ansiDim+"$1"+ansiReset)

	// ```code block``` -> cyan ANSI (code block, more visible than dim)
	text = codeBlockPattern.ReplaceAllString(text, ansiCyan+"$1"+ansiReset)

	return text
}
