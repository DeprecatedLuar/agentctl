package logger

import (
	"fmt"
	"log/slog"
	"strings"
)

const colorReset = "\033[0m"

// Each tag has one pastel color, used for both the [TAG] itself and the
// rest of the line (message + attr values), so a line reads as one color
// family. Attr keys always stay cyan regardless of tag (see colorCyan).
const (
	colorCyan = "\033[36m"       // attr key labels (e.g. socket=), not tied to any tag
	colorGray = "\033[38;5;245m" // timestamp only — not tag-tinted

	colorRecv = "\033[38;5;150m"

	// CALL, REPL, and plain INFO all share this color — same round-trip,
	// no reason to give the request/response legs different colors.
	colorInfo = "\033[38;5;111m"

	colorTool = "\033[38;5;216m"
	colorSent = "\033[38;5;183m"

	colorDebug = "\033[38;5;250m"
	colorWarn  = "\033[38;5;229m"
	colorError = "\033[38;5;210m"
)

// Kind values group Info-level log lines by pipeline stage, shown as their
// own colored tag (e.g. [CALL]) instead of the generic [INFO]. Pass via the
// "kind" attr: logger.Info(msg, "kind", logger.KindCall, ...). Elevated
// severity (Warn/Error) always takes the level tag instead, so failures
// stay visually distinct regardless of what stage they happened in.
const (
	KindRecv = "recv" // message received from the user — turn starts
	KindCall = "call" // request sent to the AI
	KindRepl = "repl" // response received from the AI
	KindTool = "tool" // a tool executed
	KindSent = "sent" // final reply delivered to the user — turn ends
)

// format renders one log record as a line of output, TTY-aware.
func (h *PrettyHandler) format(r slog.Record, kind string, attrs []slog.Attr) string {
	if h.isTTY {
		return h.formatTTY(r, kind, attrs)
	}
	return h.formatPlain(r, kind, attrs)
}

func (h *PrettyHandler) formatTTY(r slog.Record, kind string, attrs []slog.Attr) string {
	color := tagColor(r.Level, kind)
	tagStr := fmt.Sprintf("%s[%s]%s", color, tagLabel(r.Level, kind), colorReset)
	timeStr := r.Time.Format("15:04:05")

	output := fmt.Sprintf("%s %s%s%s %s%s%s",
		tagStr,
		colorGray, timeStr, colorReset,
		color, r.Message, colorReset,
	)

	for _, a := range attrs {
		output += fmt.Sprintf(" %s%s%s=%s%v%s",
			colorCyan, a.Key, colorReset,
			color, a.Value, colorReset,
		)
	}

	return output + "\n"
}

func (h *PrettyHandler) formatPlain(r slog.Record, kind string, attrs []slog.Attr) string {
	output := fmt.Sprintf("time=%s level=%s msg=%q",
		r.Time.Format("2006-01-02T15:04:05.000Z07:00"),
		tagLabel(r.Level, kind),
		r.Message,
	)

	for _, a := range attrs {
		output += fmt.Sprintf(" %s=%v", a.Key, a.Value)
	}

	return output + "\n"
}

// tagLabel is the text shown inside the brackets — the kind name (e.g. CALL)
// at Info/Debug, or the level name (WARN/ERROR/...) once severity is elevated
// or no kind was set. A Warn/Error always shows its level, so failures stay
// visible regardless of which pipeline stage they came from.
func tagLabel(level slog.Level, kind string) string {
	if kind != "" && level < slog.LevelWarn {
		return strings.ToUpper(kind)
	}
	return level.String()
}

// tagColor returns the color for a tag. Mirrors tagLabel's priority: kind
// wins below Warn, level otherwise.
func tagColor(level slog.Level, kind string) string {
	if kind != "" && level < slog.LevelWarn {
		switch kind {
		case KindRecv:
			return colorRecv
		case KindCall, KindRepl:
			return colorInfo
		case KindTool:
			return colorTool
		case KindSent:
			return colorSent
		}
	}

	switch level {
	case slog.LevelDebug:
		return colorDebug
	case slog.LevelWarn:
		return colorWarn
	case slog.LevelError:
		return colorError
	default:
		return colorInfo
	}
}

// PrintBox prints a bordered header block to stdout, used for the
// agent-startup summary (name, provider/model, tool+interface counts).
func PrintBox(lines []string) {
	width := 0
	for _, l := range lines {
		if len(l) > width {
			width = len(l)
		}
	}

	border := strings.Repeat("─", width+2)
	fmt.Printf("%s\n", border)
	for _, l := range lines {
		fmt.Printf(" %s\n", l)
	}
	fmt.Printf("%s\n\n", border)
}
