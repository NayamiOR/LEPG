package client

import (
	"fmt"
	"log/slog"
	"strings"
	"unicode"

	"LEPG/internal/model"
)

// logReading emits a uniformly formatted, column-aligned reading log line.
// Fields are padded to display-column widths (CJK-aware) so values align visually.
func logReading(source, device, point string, dataType model.DataType, value any, unit string) {
	slog.Info("reading",
		"source", padRight(source, 6),
		"device", padRight(device, 20),
		"point", padRight(point, 20),
		"type", padRight(string(dataType), 8),
		"value", truncRight(fmt.Sprintf("%v", value), 12),
		"unit", padRight(unit, 6),
	)
}

// displayWidth returns the visual column count of s in a monospace terminal,
// counting CJK fullwidth characters as 2 columns.
func displayWidth(s string) int {
	w := 0
	for _, r := range s {
		if isFullwidth(r) {
			w += 2
		} else {
			w += 1
		}
	}
	return w
}

func isFullwidth(r rune) bool {
	return unicode.Is(unicode.Han, r) ||
		unicode.Is(unicode.Hangul, r) ||
		unicode.Is(unicode.Katakana, r) ||
		unicode.Is(unicode.Hiragana, r) ||
		r == '℃' || r == '℉' ||
		(r >= 0xFF01 && r <= 0xFF60) ||
		(r >= 0xFFE0 && r <= 0xFFE6)
}

// padRight pads s with trailing spaces to the given display-column width.
func padRight(s string, width int) string {
	dw := displayWidth(s)
	if dw >= width {
		return s
	}
	return s + strings.Repeat(" ", width-dw)
}

// truncRight formats s to exactly `width` display columns:
// pads with spaces if shorter, truncates with "…" if longer.
func truncRight(s string, width int) string {
	dw := displayWidth(s)
	if dw <= width {
		return padRight(s, width)
	}
	// Truncate rune by rune, reserving 1 column for "…"
	target := width - 1
	out := ""
	ow := 0
	for _, r := range s {
		rw := 1
		if isFullwidth(r) {
			rw = 2
		}
		if ow+rw > target {
			break
		}
		out += string(r)
		ow += rw
	}
	return out + "…"
}
