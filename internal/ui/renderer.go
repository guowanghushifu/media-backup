package ui

import (
	"fmt"
	"strings"
	"time"
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

	eventLines := make([]string, 0, len(events))
	if len(events) == 0 {
		eventLines = append(eventLines, "暂无事件")
		lines = append(lines, "")
		lines = append(lines, panel("RECENT EVENTS", eventLines)...)
		return strings.Join(lines, "\n")
	}
	for _, event := range events {
		eventLines = append(eventLines, fmt.Sprintf("[%s] %s", event.At.Format("2006-01-02 15:04:05"), event.Message))
	}
	lines = append(lines, "")
	lines = append(lines, panel("RECENT EVENTS", eventLines)...)
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

	rows := []string{"NAME        PROGRESS  SPEED       ETA       STATUS"}
	for _, job := range active {
		progress, speed, eta, state := parseJobSummary(job.Summary)
		rows = append(rows, formatActiveJobRow(job.Name, progress, speed, eta, state))
	}
	return panel("ACTIVE JOBS", rows)
}

func formatActiveJobRow(name string, progress string, speed string, eta string, state string) string {
	return strings.Join([]string{
		padDisplayCell(name, 10),
		padDisplayCell(progress, 8),
		padDisplayCell(speed, 10),
		padDisplayCell(eta, 8),
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
