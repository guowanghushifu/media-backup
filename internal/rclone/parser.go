package rclone

import "strings"

const transferredPrefix = "Transferred:"

func ParseStats(line string) (string, bool) {
	line = strings.TrimSpace(line)
	if !strings.Contains(line, transferredPrefix) {
		return "", false
	}
	return line, true
}
