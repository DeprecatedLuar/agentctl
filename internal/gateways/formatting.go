package gateways

import (
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/DeprecatedLuar/agentctl/internal/syscommands"
)

// codeSpanPlaceholder marks where an extracted code span is later restored.
// \x00 bytes can't occur in normal text and contain no markdown-significant
// characters, so subsequent bold/italic/strike passes can't match across them.
const codeSpanPlaceholder = "\x00%d\x00"

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

// ProtectCodeSpans extracts ```code blocks``` and `inline code` from text,
// replacing each with a placeholder token, before any bold/italic/strike
// conversion runs. Markdown emphasis characters (*, _) commonly appear inside
// or straddle code spans (e.g. `snake_case`, or a stray "`x_` ... `_y`"), and
// converting them in place lets a tag opened inside one code span close
// inside another - producing invalid, overlapping markup. Extracting code
// first and restoring it last (via the returned restore func, to be called
// after the caller's own markdown passes) shields its content entirely.
//
// wrapBlock/wrapCode format an extracted span's raw content into the
// caller's target representation (HTML tags for Telegram, ANSI codes for
// CLI, etc.) - only that formatting differs per gateway, the extraction and
// restoration logic itself is shared.
func ProtectCodeSpans(text string, wrapBlock, wrapCode func(content string) string) (string, func(string) string) {
	var spans []string
	extract := func(text string, pattern *regexp.Regexp, wrap func(string) string) string {
		return pattern.ReplaceAllStringFunc(text, func(m string) string {
			content := pattern.FindStringSubmatch(m)[1]
			spans = append(spans, wrap(content))
			return fmt.Sprintf(codeSpanPlaceholder, len(spans)-1)
		})
	}

	// ``` must run before ` (single backtick), otherwise the single-backtick
	// pattern would consume the triple backticks as adjacent pairs.
	text = extract(text, MarkdownCodeBlockPattern, wrapBlock)
	text = extract(text, MarkdownCodePattern, wrapCode)

	restore := func(s string) string {
		for i, span := range spans {
			s = strings.Replace(s, fmt.Sprintf(codeSpanPlaceholder, i), span, 1)
		}
		return s
	}
	return text, restore
}

// FenceCodeBlock wraps text in markdown code-block fences so it renders
// consistently as a code block wherever it's delivered (each gateway's own
// Format* pipeline turns ``` into its native code-block styling). Used to
// standardize tool-use report rendering regardless of what a tool's
// `report` template contains.
func FenceCodeBlock(text string) string {
	return "```" + text + "```"
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
