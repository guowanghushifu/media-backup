package rclone

import "strings"

const infoMarker = "INFO  :"

func ParseStats(line string) (string, bool) {
	payload, ok := ParseInfoPayload(line)
	if !ok {
		return "", false
	}
	if !strings.Contains(payload, " / ") || !strings.Contains(payload, "ETA") {
		return "", false
	}
	return payload, true
}

func ParseInfoPayload(line string) (string, bool) {
	line = strings.TrimSpace(line)
	if line == "" {
		return "", false
	}

	idx := strings.Index(line, infoMarker)
	if idx == -1 {
		return "", false
	}
	payload := strings.TrimSpace(line[idx+len(infoMarker):])
	if payload == "" {
		return "", false
	}
	return payload, true
}
