package ui

import (
	"fmt"
	"strings"
	"time"
)

const (
	activeJobNameWidth     = 10
	activeJobProgressWidth = 8
	activeJobSpeedWidth    = 12
	activeJobETAWidth      = 12
)

type JobStatus struct {
	Name    string
	Summary string
}

type EventRecord struct {
	At      time.Time
	Message string
}

func RenderIdle(now time.Time) string {
	return RenderDashboard(now, nil, nil, 0, 0)
}

func RenderDashboard(now time.Time, active []JobStatus, events []EventRecord, waiting int, maxParallel int) string {
	lines := renderSummaryPanel(now, len(active), waiting, maxParallel)

	lines = append(lines, "")
	lines = append(lines, renderActiveJobsPanel(active)...)
	lines = append(lines, "")
	lines = append(lines, renderEventsPanel(now, events)...)
	return strings.Join(lines, "\n")
}

func renderSummaryPanel(now time.Time, activeCount int, waiting int, maxParallel int) []string {
	state := "IDLE"
	if activeCount > 0 {
		state = "RUNNING"
	} else if waiting > 0 {
		state = "QUEUED"
	}

	return panel("SYSTEM STATUS", []string{
		fmt.Sprintf("%s  ACTIVE %d/%d  QUEUE %d  UPDATED %s",
			"STATE "+state,
			activeCount,
			maxParallel,
			waiting,
			now.Format("15:04:05"),
		),
	})
}

func renderActiveJobsPanel(active []JobStatus) []string {
	if len(active) == 0 {
		return panel("ACTIVE JOBS", []string{"No active transfers"})
	}

	rows := []string{formatActiveJobRow("NAME", "PROGRESS", "SPEED", "ETA", "STATUS")}
	for _, job := range active {
		progress, speed, eta, state := parseJobSummary(job.Summary)
		rows = append(rows, formatActiveJobRow(job.Name, progress, speed, eta, state))
	}
	return panel("ACTIVE JOBS", rows)
}

func formatActiveJobRow(name string, progress string, speed string, eta string, state string) string {
	return strings.Join([]string{
		padDisplayCell(name, activeJobNameWidth),
		padDisplayCell(progress, activeJobProgressWidth),
		padDisplayCell(speed, activeJobSpeedWidth),
		padDisplayCell(eta, activeJobETAWidth),
		state,
	}, "  ")
}

func padDisplayCell(text string, width int) string {
	padding := width - displayWidth(text)
	if padding < 0 {
		padding = 0
	}
	return text + strings.Repeat(" ", padding)
}

func parseJobSummary(summary string) (progress string, speed string, eta string, state string) {
	progress = "-"
	speed = "-"
	eta = "-"
	state = "WAITING"

	parts := strings.Split(summary, ", ")
	if len(parts) < 4 {
		return progress, speed, eta, state
	}

	progress = parts[1]
	speed = parts[2]
	eta = formatETA(parts[3])
	state = "COPYING"
	return progress, speed, eta, state
}

func formatETA(raw string) string {
	trimmed := strings.TrimPrefix(raw, "ETA ")
	d, err := time.ParseDuration(trimmed)
	if err != nil {
		return raw
	}

	totalSeconds := int(d.Seconds())
	hours := totalSeconds / 3600
	minutes := (totalSeconds % 3600) / 60
	seconds := totalSeconds % 60
	if hours > 0 {
		return fmt.Sprintf("ETA %02d:%02d:%02d", hours, minutes, seconds)
	}
	return fmt.Sprintf("ETA %02d:%02d", minutes, seconds)
}

func renderEventsPanel(now time.Time, events []EventRecord) []string {
	if len(events) == 0 {
		return panel("RECENT EVENTS (0)", []string{"Watching for new files..."})
	}

	rows := make([]string, 0, len(events))
	for _, event := range events {
		tag, message := classifyEvent(event.Message)
		rows = append(rows, fmt.Sprintf("%s  %-6s  %s", formatEventTime(now, event.At), tag, message))
	}
	return panel(fmt.Sprintf("RECENT EVENTS (%d)", len(events)), rows)
}

func classifyEvent(message string) (string, string) {
	switch {
	case strings.Contains(message, "Copied (new)"):
		return "DONE", message
	case strings.Contains(message, "启动扫描发现"), strings.Contains(message, "链接目录发现"), strings.Contains(message, "检测到新文件"):
		return "SCAN", message
	case strings.Contains(message, "上传失败"):
		return "FAIL", message
	case strings.Contains(message, "上传完成"):
		return "DONE", message
	case strings.Contains(message, "调度开始上传"), strings.Contains(message, "重新排队"), strings.Contains(message, "重试"):
		return "QUEUE", message
	default:
		return "INFO", message
	}
}

func formatEventTime(now time.Time, at time.Time) string {
	localNow := now.In(at.Location())
	if localNow.Year() == at.Year() && localNow.YearDay() == at.YearDay() {
		return at.Format("15:04:05")
	}
	return at.Format("01-02 15:04")
}
