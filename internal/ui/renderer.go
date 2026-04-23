package ui

import (
	"fmt"
	"strings"
	"time"
)

const (
	activeJobNameMaxWidth  = 40
	activeJobProgressWidth = 8
	activeJobSpeedWidth    = 16
	activeJobETAWidth      = 13
	activeJobStateWidth    = 6
	minActiveJobRows       = 10
	minEventRows           = 10
	minDashboardWidth      = 9
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
	return RenderDashboardWithWidth(now, nil, nil, 0, 0, defaultWidth)
}

func RenderDashboard(now time.Time, active []JobStatus, events []EventRecord, waiting int, maxParallel int) string {
	return RenderDashboardWithWidth(now, active, events, waiting, maxParallel, defaultWidth)
}

func RenderDashboardWithWidth(now time.Time, active []JobStatus, events []EventRecord, waiting int, maxParallel int, width int) string {
	summaryTitle, summaryBody := renderSummaryPanel(now, len(active), waiting, maxParallel)
	totalWidth := width
	if totalWidth < minDashboardWidth {
		totalWidth = minDashboardWidth
	}

	innerPanelWidth := totalWidth - 4
	if innerPanelWidth < 5 {
		innerPanelWidth = 5
		totalWidth = innerPanelWidth + 4
	}

	jobsTitle, jobsBody := renderActiveJobsPanel(active, innerPanelWidth)
	eventsTitle, eventsBody := renderEventsPanel(now, events)

	lines := renderPanel(summaryTitle, summaryBody, innerPanelWidth, 1)
	lines = append(lines, "")
	lines = append(lines, renderPanel(jobsTitle, jobsBody, innerPanelWidth, minActiveJobRows)...)
	lines = append(lines, "")
	lines = append(lines, renderPanel(eventsTitle, eventsBody, innerPanelWidth, minEventRows)...)

	return strings.Join(outerFrame(lines, totalWidth), "\n")
}

func renderSummaryPanel(now time.Time, activeCount int, waiting int, maxParallel int) (string, []string) {
	state := "空闲"
	if activeCount > 0 {
		state = "运行中"
	} else if waiting > 0 {
		state = "排队中"
	}

	return "系统状态", []string{
		fmt.Sprintf("%s %s ｜ 活动 %d/%d ｜ 排队 %d ｜ 更新 %s",
			"状态",
			state,
			activeCount,
			maxParallel,
			waiting,
			now.Format("15:04:05"),
		),
	}
}

func renderActiveJobsPanel(active []JobStatus, panelWidth int) (string, []string) {
	if len(active) == 0 {
		return "活动任务", []string{"暂无活动任务"}
	}

	nameWidth := activeJobNameColumnWidth(panelWidth)
	rows := []string{formatActiveJobRow("名称", "进度", "速度", "预计", "状态", nameWidth)}
	for _, job := range active {
		progress, speed, eta, state := parseJobSummary(job.Summary)
		rows = append(rows, formatActiveJobRow(job.Name, progress, speed, eta, state, nameWidth))
	}
	return "活动任务", rows
}

func activeJobNameColumnWidth(panelWidth int) int {
	bodyWidth := panelWidth - 4
	usedWidth := activeJobProgressWidth + activeJobSpeedWidth + activeJobETAWidth + activeJobStateWidth + 8
	availableWidth := bodyWidth - usedWidth
	if availableWidth < displayColumns("名称") {
		return displayColumns("名称")
	}
	if availableWidth > activeJobNameMaxWidth {
		return activeJobNameMaxWidth
	}
	return availableWidth
}

func formatActiveJobRow(name string, progress string, speed string, eta string, state string, nameWidth int) string {
	return strings.Join([]string{
		fitDisplayColumns(name, nameWidth),
		padDisplayCell(progress, activeJobProgressWidth),
		padDisplayCell(speed, activeJobSpeedWidth),
		padDisplayCell(eta, activeJobETAWidth),
		state,
	}, "  ")
}

func padDisplayCell(text string, width int) string {
	return padOrTrimDisplay(text, width)
}

func parseJobSummary(summary string) (progress string, speed string, eta string, state string) {
	progress = "-"
	speed = "-"
	eta = "-"
	state = "等待中"

	parts := strings.Split(summary, ", ")
	if len(parts) < 4 {
		return progress, speed, eta, state
	}

	progress = parts[1]
	speed = parts[2]
	eta = formatETA(parts[3])
	state = "传输中"
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
		return fmt.Sprintf("预计 %02d:%02d:%02d", hours, minutes, seconds)
	}
	return fmt.Sprintf("预计 %02d:%02d", minutes, seconds)
}

func renderEventsPanel(now time.Time, events []EventRecord) (string, []string) {
	if len(events) == 0 {
		return "最近事件 (0)", []string{"正在等待新文件..."}
	}

	rows := make([]string, 0, len(events))
	for _, event := range events {
		tag, message := classifyEvent(event.Message)
		rows = append(rows, fmt.Sprintf("%s  %s  %s", formatEventTime(now, event.At), padDisplayCell(tag, 6), message))
	}
	return fmt.Sprintf("最近事件 (%d)", len(events)), rows
}

func classifyEvent(message string) (string, string) {
	switch {
	case strings.Contains(message, "Copied (new)"):
		return "完成", message
	case strings.Contains(message, "启动扫描发现"), strings.Contains(message, "链接目录发现"), strings.Contains(message, "检测到新文件"):
		return "扫描", message
	case strings.Contains(message, "上传失败"):
		return "失败", message
	case strings.Contains(message, "上传完成"):
		return "完成", message
	case strings.Contains(message, "调度开始上传"), strings.Contains(message, "重新排队"), strings.Contains(message, "重试"):
		return "排队", message
	default:
		return "信息", message
	}
}

func formatEventTime(now time.Time, at time.Time) string {
	localNow := now.In(at.Location())
	if localNow.Year() == at.Year() && localNow.YearDay() == at.YearDay() {
		return at.Format("15:04:05")
	}
	return at.Format("01-02 15:04")
}
