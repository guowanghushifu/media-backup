package ui

import (
	"strings"
	"testing"
	"time"
)

func TestRenderIdle(t *testing.T) {
	t.Parallel()

	out := RenderIdle(time.Date(2026, 4, 22, 15, 4, 5, 0, time.UTC))
	if !strings.Contains(out, "当前状态：空闲") {
		t.Fatalf("RenderIdle() = %q", out)
	}
}

func TestRenderActiveDashboard(t *testing.T) {
	t.Parallel()

	out := RenderDashboard(
		time.Date(2026, 4, 22, 15, 4, 5, 0, time.UTC),
		[]JobStatus{
			{Name: "job-a", Summary: "Transferred: 35%"},
			{Name: "job-b", Summary: "Transferred: 74%"},
		},
		1,
		5,
	)
	if !strings.Contains(out, "活跃任务: 2/5") {
		t.Fatalf("RenderDashboard() = %q", out)
	}
	if !strings.Contains(out, "[job-a]") || !strings.Contains(out, "[job-b]") {
		t.Fatalf("RenderDashboard() missing job lines: %q", out)
	}
}
