package ui

import (
	"strings"
	"unicode"
)

func fitDisplayColumns(text string, width int) string {
	if width <= 0 {
		return ""
	}
	if displayColumns(text) > width {
		return truncateDisplayColumns(text, width)
	}
	return text + strings.Repeat(" ", width-displayColumns(text))
}

func truncateDisplayColumns(text string, width int) string {
	if width <= 0 {
		return ""
	}
	if displayColumns(text) <= width {
		return text
	}
	if width <= 3 {
		return strings.Repeat(".", width)
	}

	target := width - 3
	current := 0
	var b strings.Builder
	for _, r := range text {
		rw := runeDisplayWidth(r)
		if rw == 0 {
			b.WriteRune(r)
			continue
		}
		if current+rw > target {
			break
		}
		b.WriteRune(r)
		current += rw
	}
	b.WriteString("...")
	return b.String()
}

func displayColumns(text string) int {
	width := 0
	for _, r := range text {
		width += runeDisplayWidth(r)
	}
	return width
}

func runeDisplayWidth(r rune) int {
	switch {
	case r == 0:
		return 0
	case unicode.Is(unicode.Mn, r):
		return 0
	case isDoubleWidthRune(r):
		return 2
	default:
		return 1
	}
}

func isDoubleWidthRune(r rune) bool {
	return r >= 0x1100 && (r <= 0x115f ||
		r == 0x2329 ||
		r == 0x232a ||
		(r >= 0x2e80 && r <= 0xa4cf && r != 0x303f) ||
		(r >= 0xac00 && r <= 0xd7a3) ||
		(r >= 0xf900 && r <= 0xfaff) ||
		(r >= 0xfe10 && r <= 0xfe19) ||
		(r >= 0xfe30 && r <= 0xfe6f) ||
		(r >= 0xff00 && r <= 0xff60) ||
		(r >= 0xffe0 && r <= 0xffe6) ||
		(r >= 0x1f300 && r <= 0x1f64f) ||
		(r >= 0x1f900 && r <= 0x1f9ff) ||
		(r >= 0x20000 && r <= 0x3fffd))
}
