package telegram

import (
	"html"
	"regexp"
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

// formatForTelegram converts common markdown to Telegram HTML
func formatForTelegram(text string) string {
	// Escape HTML special chars first
	text = html.EscapeString(text)

	// Convert markdown to HTML (order matters: bold-italic first, then bold, then italic)

	// ***bold-italic*** -> <b><i>text</i></b>
	text = boldItalicPattern1.ReplaceAllString(text, "<b><i>$1</i></b>")
	text = boldItalicPattern2.ReplaceAllString(text, "<b><i>$1</i></b>")
	text = boldItalicPattern3.ReplaceAllString(text, "<b><i>$1</i></b>")

	// **bold** or __bold__ -> <b>bold</b>
	text = boldPattern1.ReplaceAllString(text, "<b>$1</b>")
	text = boldPattern2.ReplaceAllString(text, "<b>$1</b>")

	// *italic* or _italic_ -> <i>italic</i>
	text = italicPattern1.ReplaceAllString(text, "<i>$1</i>")
	text = italicPattern2.ReplaceAllString(text, "<i>$1</i>")

	// ~~strikethrough~~ -> <s>strikethrough</s>
	text = strikePattern.ReplaceAllString(text, "<s>$1</s>")

	// `code` -> <code>code</code>
	text = codePattern.ReplaceAllString(text, "<code>$1</code>")

	// ```code block``` -> <pre>code</pre>
	text = codeBlockPattern.ReplaceAllString(text, "<pre>$1</pre>")

	return text
}
