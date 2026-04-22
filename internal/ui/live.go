package ui

const (
	enterAlternateScreen = "\x1b[?1049h"
	leaveAlternateScreen = "\x1b[?1049l"
	repaintFromHome      = "\x1b[H\x1b[J"
)

func EnterAlternateScreen() string {
	return enterAlternateScreen
}

func LeaveAlternateScreen() string {
	return leaveAlternateScreen
}

func RewriteFrame(content string) string {
	return repaintFromHome + content
}
