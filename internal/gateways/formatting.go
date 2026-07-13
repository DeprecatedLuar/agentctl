package gateways

import (
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/DeprecatedLuar/agentctl/internal/syscommands"
)

// Shared markdown regex patterns used by both cli and telegram formatting.
// Only patterns whose structure (capture groups) is identical across
// gateways live here - italic patterns differ per gateway (capturing
// vs non-capturing groups) and stay in each package's own formatting.go.
var (
	MarkdownBoldItalicPattern1 = regexp.MustCompile(`\*\*\*(.+?)\*\*\*`)         // ***text***
	MarkdownBoldItalicPattern2 = regexp.MustCompile(`__\*(.+?)\*__`)             // __*text*__
	MarkdownBoldItalicPattern3 = regexp.MustCompile(`\*\*_(.+?)_\*\*`)           // **_text_**
	MarkdownBoldPattern1       = regexp.MustCompile(`\*\*(.+?)\*\*`)             // **text**
	MarkdownBoldPattern2       = regexp.MustCompile(`__(.+?)__`)                 // __text__
	MarkdownStrikePattern      = regexp.MustCompile(`~~(.+?)~~`)                 // ~~strike~~
	MarkdownCodePattern        = regexp.MustCompile("`(.+?)`")                   // `code`
	MarkdownCodeBlockPattern   = regexp.MustCompile("```(?:\n)?(.+?)(?:\n)?```") // ```code```
)

// ApplyBoldItalic replaces all three bold-italic markdown variants with the
// same replacement template (which may reference the "$1" capture group).
func ApplyBoldItalic(text, replacement string) string {
	text = MarkdownBoldItalicPattern1.ReplaceAllString(text, replacement)
	text = MarkdownBoldItalicPattern2.ReplaceAllString(text, replacement)
	text = MarkdownBoldItalicPattern3.ReplaceAllString(text, replacement)
	return text
}

// ApplyBold replaces both bold markdown variants (**text** and __text__)
// with the same replacement template.
func ApplyBold(text, replacement string) string {
	text = MarkdownBoldPattern1.ReplaceAllString(text, replacement)
	text = MarkdownBoldPattern2.ReplaceAllString(text, replacement)
	return text
}

// ApplyStrike replaces ~~strike~~ markdown with the given replacement template.
func ApplyStrike(text, replacement string) string {
	return MarkdownStrikePattern.ReplaceAllString(text, replacement)
}

// ApplyCode replaces `code` markdown with the given replacement template.
func ApplyCode(text, replacement string) string {
	return MarkdownCodePattern.ReplaceAllString(text, replacement)
}

// ApplyCodeBlock replaces ```code block``` markdown with the given replacement template.
func ApplyCodeBlock(text, replacement string) string {
	return MarkdownCodeBlockPattern.ReplaceAllString(text, replacement)
}

// FormatNewSession formats a /new command result. Identical across all interfaces.
func FormatNewSession(result syscommands.CommandResult) string {
	data := result.Data.(map[string]string)
	return fmt.Sprintf("New session started\n\nModel: %s\nProvider: %s\nMemory: %s messages",
		data["model"], data["provider"], data["memory"])
}

// FormatTimestamp converts a Unix timestamp to YYYY-MM-DD format.
func FormatTimestamp(ts int64) string {
	if ts == 0 {
		return "unknown"
	}
	t := time.Unix(ts, 0)
	return t.Format("2006-01-02")
}

// FormatSessionList formats a /sessions command result as a newline-separated
// list, one line per session. When numbered is true each line is prefixed
// "1. ", "2. ", ... (CLI); otherwise each line is prefixed "- " (Telegram).
func FormatSessionList(sessions []syscommands.SessionInfo, numbered bool) string {
	if len(sessions) == 0 {
		return "No sessions found"
	}

	var b strings.Builder
	b.WriteString("Sessions:\n")
	for i, s := range sessions {
		title := s.Title
		if title == "" {
			title = "(untitled)"
		}

		date := FormatTimestamp(s.CreatedAt)

		prefix := "- "
		if numbered {
			prefix = fmt.Sprintf("%d. ", i+1)
		}
		fmt.Fprintf(&b, "%s%s (%s)", prefix, title, date)
		if s.IsActive {
			b.WriteString(" [active]")
		}
		b.WriteString("\n")
	}

	return strings.TrimSpace(b.String())
}
