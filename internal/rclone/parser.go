package rclone

import "strings"

const transferredPrefix = "Transferred:"

func ParseStats(line string) (string, bool) {
	if !strings.HasPrefix(line, transferredPrefix) {
		return "", false
	}

	stats := strings.TrimSpace(strings.TrimPrefix(line, transferredPrefix))
	if stats == "" {
		return "", false
	}
	return stats, true
}
