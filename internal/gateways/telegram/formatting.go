package telegram

import (
	"html"
	"regexp"

	"github.com/DeprecatedLuar/agentctl/internal/gateways"
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

	// Extract code spans before any other conversion - see ProtectCodeSpans.
	text, restoreCode := gateways.ProtectCodeSpans(text,
		func(content string) string { return "<pre>" + content + "</pre>" },
		func(content string) string { return "<code>" + content + "</code>" },
	)

	// Convert markdown to HTML (order matters: bold-italic first, then bold, then italic)

	// ***bold-italic*** -> <b><i>text</i></b>
	text = gateways.ApplyBoldItalic(text, "<b><i>$1</i></b>")

	// **bold** or __bold__ -> <b>bold</b>
	text = gateways.ApplyBold(text, "<b>$1</b>")

	// *italic* or _italic_ -> <i>italic</i>
	text = italicPattern1.ReplaceAllString(text, "$1<i>$2</i>$3")
	text = italicPattern2.ReplaceAllString(text, "$1<i>$2</i>$3")

	// ~~strikethrough~~ -> <s>strikethrough</s>
	text = gateways.ApplyStrike(text, "<s>$1</s>")

	// Restore code spans verbatim now that no further passes can corrupt them.
	text = restoreCode(text)

	return text
}
