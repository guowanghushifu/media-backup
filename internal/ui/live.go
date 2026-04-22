package ui

import (
	"fmt"
	"strings"
)

func RewriteFrame(previousLines int, content string) (string, int) {
	lines := frameLineCount(content)
	if previousLines <= 0 {
		return content, lines
	}

	var b strings.Builder
	b.WriteString("\r")
	if previousLines > 1 {
		fmt.Fprintf(&b, "\x1b[%dA", previousLines-1)
	}
	b.WriteString("\x1b[J")
	b.WriteString(content)
	return b.String(), lines
}

func frameLineCount(content string) int {
	if content == "" {
		return 1
	}
	return 1 + strings.Count(content, "\n")
}
