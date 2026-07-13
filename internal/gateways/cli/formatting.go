package cli

import (
	"os"
	"regexp"

	"github.com/DeprecatedLuar/agentctl/internal/gateways"
)

const (
	// ANSI escape codes
	ansiReset         = "\033[0m"
	ansiBold          = "\033[1m"
	ansiItalic        = "\033[3m"
	ansiDim           = "\033[2m"
	ansiStrikethrough = "\033[9m"
	ansiCyan          = "\033[36m" // For code blocks
)

var (
	// Pre-compile regex patterns for performance
	// *italic* / _italic_ patterns are CLI-specific (non-capturing surrounds,
	// only the inner text is captured), so they aren't shared with telegram.
	italicPattern1 = regexp.MustCompile(`(?:^|[^*_])\*([^*]+?)\*(?:[^*]|$)`) // *text*
	italicPattern2 = regexp.MustCompile(`(?:^|[^*_])_([^_]+?)_(?:[^_]|$)`)   // _text_
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
	text = gateways.ApplyBoldItalic(text, ansiBold+ansiItalic+"$1"+ansiReset)

	// **bold** or __bold__ -> bold ANSI
	text = gateways.ApplyBold(text, ansiBold+"$1"+ansiReset)

	// *italic* or _italic_ -> italic ANSI
	text = italicPattern1.ReplaceAllString(text, ansiItalic+"$1"+ansiReset)
	text = italicPattern2.ReplaceAllString(text, ansiItalic+"$1"+ansiReset)

	// ~~strikethrough~~ -> strikethrough ANSI
	text = gateways.ApplyStrike(text, ansiStrikethrough+"$1"+ansiReset)

	// `code` -> dim ANSI (inline code)
	text = gateways.ApplyCode(text, ansiDim+"$1"+ansiReset)

	// ```code block``` -> cyan ANSI (code block, more visible than dim)
	text = gateways.ApplyCodeBlock(text, ansiCyan+"$1"+ansiReset)

	return text
}
