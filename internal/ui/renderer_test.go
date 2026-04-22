package ui_test

import (
	"strings"
	"testing"
	"time"

	"github.com/wangdazhuo/media-backup/internal/ui"
)

func TestRenderIdle(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 4, 22, 15, 4, 5, 0, time.UTC)
	want := "[2026-04-22 15:04:05] 当前状态：空闲"

	if got := ui.RenderIdle(now); got != want {
		t.Fatalf("RenderIdle() = %q, want %q", got, want)
	}
}

func TestRenderActiveDashboard(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 4, 22, 15, 4, 5, 0, time.UTC)
	active := []ui.JobStatus{
		{Name: "job-a", Summary: "832 MiB / 1000 MiB, 83%, 29.793 MiB/s, ETA 5s"},
		{Name: "job-b", Summary: "12.4 GiB / 40.0 GiB, 31%, 48.2 MiB/s, ETA 9m12s", Event: "THIS_IS_TEST/uploadtest.bin: Copied (new)"},
	}
	out := ui.RenderDashboard(
		now,
		active,
		1,
		5,
	)
	want := strings.Join([]string{
		"[2026-04-22 15:04:05] 当前状态：正在传输 | 活跃任务: 2/5 | 等待中: 1",
		"[job-a] 832 MiB / 1000 MiB, 83%, 29.793 MiB/s, ETA 5s",
		"[job-b] 12.4 GiB / 40.0 GiB, 31%, 48.2 MiB/s, ETA 9m12s",
		"[job-b] 最近事件: THIS_IS_TEST/uploadtest.bin: Copied (new)",
	}, "\n")
	if out != want {
		t.Fatalf("RenderDashboard() = %q, want %q", out, want)
	}

	lines := strings.Split(out, "\n")
	if len(lines) != 4 {
		t.Fatalf("RenderDashboard() line count = %d, want 4", len(lines))
	}
	wantLines := []string{
		"[job-a] 832 MiB / 1000 MiB, 83%, 29.793 MiB/s, ETA 5s",
		"[job-b] 12.4 GiB / 40.0 GiB, 31%, 48.2 MiB/s, ETA 9m12s",
		"[job-b] 最近事件: THIS_IS_TEST/uploadtest.bin: Copied (new)",
	}
	for i, wantLine := range wantLines {
		if lines[i+1] != wantLine {
			t.Fatalf("RenderDashboard() line %d = %q, want %q", i+2, lines[i+1], wantLine)
		}
	}
}

func TestRenderActiveDashboardNoActiveJobsHeaderOnly(t *testing.T) {
	t.Parallel()

	out := ui.RenderDashboard(
		time.Date(2026, 4, 22, 15, 4, 5, 0, time.UTC),
		nil,
		3,
		5,
	)
	want := "[2026-04-22 15:04:05] 当前状态：正在传输 | 活跃任务: 0/5 | 等待中: 3"
	if out != want {
		t.Fatalf("RenderDashboard() = %q, want %q", out, want)
	}
}
