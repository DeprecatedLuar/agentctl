package telegram

import (
	"html"
	"regexp"

	"github.com/DeprecatedLuar/agentctl/internal/interfaces"
)

var (
	// Pre-compile regex patterns for performance
	// *italic* / _italic_ patterns are Telegram-specific (capturing surrounds,
	// used to preserve the surrounding characters in the replacement), so
	// they aren't shared with cli.
	italicPattern1 = regexp.MustCompile(`(^|[^*_])\*([^*]+?)\*([^*]|$)`) // *text*
	italicPattern2 = regexp.MustCompile(`(^|[^*_])_([^_]+?)_([^_]|$)`)   // _text_
)

// formatForTelegram converts common markdown to Telegram HTML
func formatForTelegram(text string) string {
	// Escape HTML special chars first
	text = html.EscapeString(text)

	// Convert markdown to HTML (order matters: bold-italic first, then bold, then italic)

	// ***bold-italic*** -> <b><i>text</i></b>
	text = interfaces.ApplyBoldItalic(text, "<b><i>$1</i></b>")

	// **bold** or __bold__ -> <b>bold</b>
	text = interfaces.ApplyBold(text, "<b>$1</b>")

	// *italic* or _italic_ -> <i>italic</i>
	text = italicPattern1.ReplaceAllString(text, "$1<i>$2</i>$3")
	text = italicPattern2.ReplaceAllString(text, "$1<i>$2</i>$3")

	// ~~strikethrough~~ -> <s>strikethrough</s>
	text = interfaces.ApplyStrike(text, "<s>$1</s>")

	// ```code block``` -> <pre>code</pre> (must run before single-backtick codePattern,
	// otherwise codePattern consumes the triple backticks as adjacent single-backtick pairs)
	text = interfaces.ApplyCodeBlock(text, "<pre>$1</pre>")

	// `code` -> <code>code</code>
	text = interfaces.ApplyCode(text, "<code>$1</code>")

	return text
}
