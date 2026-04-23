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
	return renderPanel(title, body, panelTotalWidth(title, body), 0)
}

func renderPanel(title string, body []string, totalWidth int, minBodyRows int) []string {
	if minWidth := panelTotalWidth(title, body); totalWidth < minWidth {
		totalWidth = minWidth
	}

	bodyWidth := totalWidth - 4
	linesBody := append([]string{}, body...)
	for len(linesBody) < minBodyRows {
		linesBody = append(linesBody, "")
	}

	titleWidth := displayWidth(title)
	topFill := totalWidth - titleWidth - 5
	if topFill < 0 {
		topFill = 0
	}

	lines := []string{
		"┌─ " + title + " " + strings.Repeat("─", topFill) + "┐",
	}
	for _, line := range linesBody {
		lines = append(lines, "│ "+padOrTrimDisplay(line, bodyWidth)+" │")
	}
	lines = append(lines, "└"+strings.Repeat("─", totalWidth-2)+"┘")
	return lines
}

func panelTotalWidth(title string, body []string) int {
	width := displayWidth(title) + 5
	for _, line := range body {
		if lineWidth := displayWidth(line) + 4; lineWidth > width {
			width = lineWidth
		}
	}
	return width
}

func outerFrame(lines []string, totalWidth int) []string {
	if totalWidth < 6 {
		totalWidth = 6
	}
	innerWidth := totalWidth - 4
	framed := []string{"┌" + strings.Repeat("─", totalWidth-2) + "┐"}
	blank := "│ " + strings.Repeat(" ", innerWidth) + " │"

	for _, line := range lines {
		if line == "" {
			framed = append(framed, blank)
			continue
		}
		framed = append(framed, "│ "+padOrTrimDisplay(line, innerWidth)+" │")
	}

	framed = append(framed, "└"+strings.Repeat("─", totalWidth-2)+"┘")
	return framed
}

func padOrTrimDisplay(text string, width int) string {
	if width <= 0 {
		return ""
	}
	if displayWidth(text) > width {
		return trimDisplay(text, width)
	}
	return text + strings.Repeat(" ", width-displayWidth(text))
}

func trimDisplay(text string, width int) string {
	if width <= 0 {
		return ""
	}
	if displayWidth(text) <= width {
		return text
	}
	if width <= 3 {
		return strings.Repeat(".", width)
	}

	target := width - 3
	current := 0
	var b strings.Builder
	for _, r := range text {
		rw := 1
		if isWideRune(r) {
			rw = 2
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
