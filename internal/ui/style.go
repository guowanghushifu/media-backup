package ui

import (
	"strings"
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
	if totalWidth < 5 {
		totalWidth = 5
	}

	bodyWidth := totalWidth - 4
	linesBody := append([]string{}, body...)
	for len(linesBody) < minBodyRows {
		linesBody = append(linesBody, "")
	}

	maxTitleWidth := totalWidth - 5
	displayTitle := title
	if maxTitleWidth <= 0 {
		displayTitle = ""
	} else if displayColumns(title) > maxTitleWidth {
		displayTitle = truncateDisplayColumns(title, maxTitleWidth)
	}

	titleWidth := displayColumns(displayTitle)
	topFill := totalWidth - titleWidth - 5
	if topFill < 0 {
		topFill = 0
	}

	lines := []string{
		"┌─ " + displayTitle + " " + strings.Repeat("─", topFill) + "┐",
	}
	for _, line := range linesBody {
		lines = append(lines, "│ "+padOrTrimDisplay(line, bodyWidth)+" │")
	}
	lines = append(lines, "└"+strings.Repeat("─", totalWidth-2)+"┘")
	return lines
}

func panelTotalWidth(title string, body []string) int {
	width := displayColumns(title) + 5
	for _, line := range body {
		if lineWidth := displayColumns(line) + 4; lineWidth > width {
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
	if displayColumns(text) > width {
		return truncateDisplayColumns(text, width)
	}
	return text + strings.Repeat(" ", width-displayColumns(text))
}
