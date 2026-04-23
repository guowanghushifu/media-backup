package ui

import (
	"io"
	"os"

	"golang.org/x/sys/unix"
)

const (
	enterAlternateScreen = "\x1b[?1049h"
	leaveAlternateScreen = "\x1b[?1049l"
	hideCursor           = "\x1b[?25l"
	showCursor           = "\x1b[?25h"
	repaintFromHome      = "\x1b[H\x1b[J"
	defaultWidth         = 80
)

func EnterAlternateScreen() string {
	return enterAlternateScreen + hideCursor
}

func LeaveAlternateScreen() string {
	return showCursor + leaveAlternateScreen
}

func RewriteFrame(content string) string {
	return repaintFromHome + content
}

func DetectWidth(writer io.Writer) int {
	file, ok := writer.(*os.File)
	if !ok {
		return defaultWidth
	}

	ws, err := unix.IoctlGetWinsize(int(file.Fd()), unix.TIOCGWINSZ)
	if err != nil || ws == nil || ws.Col == 0 {
		return defaultWidth
	}

	return int(ws.Col)
}
