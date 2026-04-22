package ui

import (
	"fmt"
	"strings"
	"time"
)

type JobStatus struct {
	Name    string
	Summary string
	Event   string
}

func RenderIdle(now time.Time) string {
	return fmt.Sprintf("[%s] 当前状态：空闲", now.Format("2006-01-02 15:04:05"))
}

func RenderDashboard(now time.Time, active []JobStatus, waiting int, maxParallel int) string {
	lines := []string{
		fmt.Sprintf("[%s] 当前状态：正在传输 | 活跃任务: %d/%d | 等待中: %d",
			now.Format("2006-01-02 15:04:05"), len(active), maxParallel, waiting),
	}
	for _, job := range active {
		lines = append(lines, fmt.Sprintf("[%s] %s", job.Name, job.Summary))
		if job.Event != "" {
			lines = append(lines, fmt.Sprintf("[%s] 最近事件: %s", job.Name, job.Event))
		}
	}
	return strings.Join(lines, "\n")
}
