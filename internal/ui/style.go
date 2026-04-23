package ui

import (
	"strings"
	"unicode"
)

const (
	ansiReset  = "\x1b[0m"
	ansiBold   = "\x1b[1m"
	ansiBlue   = "\x1b[36m"
	ansiGreen  = "\x1b[32m"
	ansiYellow = "\x1b[33m"
	ansiRed    = "\x1b[31m"
)

func colorize(code string, text string) string {
	return code + text + ansiReset
}

func panel(title string, body []string) []string {
	width := displayWidth(title)
	for _, line := range body {
		if lineWidth := displayWidth(line); lineWidth > width {
			width = lineWidth
		}
	}

	lines := []string{
		"┌─ " + title + " " + strings.Repeat("─", width-displayWidth(title)) + "─┐",
	}
	for _, line := range body {
		lines = append(lines, "│ "+line+strings.Repeat(" ", width-displayWidth(line))+" │")
	}
	lines = append(lines, "└"+strings.Repeat("─", width+3)+"┘")
	return lines
}

func displayWidth(text string) int {
	width := 0
	for _, r := range text {
		switch {
		case r == 0:
			continue
		case unicode.Is(unicode.Mn, r):
			continue
		case isWideRune(r):
			width += 2
		default:
			width++
		}
	}
	return width
}

func isWideRune(r rune) bool {
	return r >= 0x1100 && (
		r <= 0x115f ||
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
