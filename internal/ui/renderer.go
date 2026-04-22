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
	lines := []string{renderStatusLine(now, len(active), waiting, maxParallel)}
	for _, job := range active {
		lines = append(lines, fmt.Sprintf("[%s] %s", job.Name, job.Summary))
	}
	lines = append(lines, "", "最近事件:")
	if len(events) == 0 {
		lines = append(lines, "暂无事件")
		return strings.Join(lines, "\n")
	}
	for _, event := range events {
		lines = append(lines, fmt.Sprintf("[%s] %s", event.At.Format("2006-01-02 15:04:05"), event.Message))
	}
	return strings.Join(lines, "\n")
}

func renderStatusLine(now time.Time, activeCount int, waiting int, maxParallel int) string {
	if activeCount == 0 {
		if waiting > 0 {
			return fmt.Sprintf("[%s] 当前状态：等待中 | 活跃任务: 0/%d | 等待中: %d",
				now.Format("2006-01-02 15:04:05"), maxParallel, waiting)
		}
		return fmt.Sprintf("[%s] 当前状态：空闲", now.Format("2006-01-02 15:04:05"))
	}
	return fmt.Sprintf("[%s] 当前状态：正在传输 | 活跃任务: %d/%d | 等待中: %d",
		now.Format("2006-01-02 15:04:05"), activeCount, maxParallel, waiting)
}
