package ui_test

import (
	"strings"
	"testing"
	"time"

	"github.com/guowanghushifu/media-backup/internal/ui"
)

func TestRenderIdle(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 4, 22, 15, 4, 5, 0, time.UTC)
	want := strings.Join([]string{
		"┌─ SYSTEM STATUS ─────────────────────────────────────┐",
		"│ STATE IDLE  ACTIVE 0/0  QUEUE 0  UPDATED 15:04:05 │",
		"└────────────────────────────────────────────────────┘",
		"",
		"┌─ ACTIVE JOBS ─────────┐",
		"│ No active transfers │",
		"└──────────────────────┘",
		"",
		"┌─ RECENT EVENTS ─┐",
		"│ 暂无事件      │",
		"└────────────────┘",
	}, "\n")

	if got := ui.RenderIdle(now); got != want {
		t.Fatalf("RenderIdle() = %q, want %q", got, want)
	}
}

func TestRenderActiveDashboard(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 4, 22, 15, 4, 5, 0, time.UTC)
	active := []ui.JobStatus{
		{Name: "job-a", Summary: "832 MiB / 1000 MiB, 83%, 29.793 MiB/s, ETA 5s"},
		{Name: "job-b", Summary: "12.4 GiB / 40.0 GiB, 31%, 48.2 MiB/s, ETA 9m12s"},
	}
	events := []ui.EventRecord{
		{At: time.Date(2026, 4, 22, 15, 3, 58, 0, time.UTC), Message: "THIS_IS_TEST/file-01.mkv: Copied (new)"},
		{At: time.Date(2026, 4, 22, 15, 4, 3, 0, time.UTC), Message: "THIS_IS_TEST/file-02.mkv: Copied (new)"},
	}
	out := ui.RenderDashboard(
		now,
		active,
		events,
		1,
		5,
	)
	wantLines := []string{
		"┌─ SYSTEM STATUS ────────────────────────────────────────┐",
		"│ STATE RUNNING  ACTIVE 2/5  QUEUE 1  UPDATED 15:04:05 │",
		"└───────────────────────────────────────────────────────┘",
		"",
		"┌─ ACTIVE JOBS ────────────────────────────────────────────┐",
		"│ NAME        PROGRESS  SPEED       ETA       STATUS     │",
		"│ job-a       83%       29.793 MiB/s  ETA 00:05  COPYING │",
		"│ job-b       31%       48.2 MiB/s  ETA 09:12  COPYING   │",
		"└─────────────────────────────────────────────────────────┘",
		"",
		"┌─ RECENT EVENTS ────────────────────────────────────────────────┐",
		"│ [2026-04-22 15:03:58] THIS_IS_TEST/file-01.mkv: Copied (new) │",
		"│ [2026-04-22 15:04:03] THIS_IS_TEST/file-02.mkv: Copied (new) │",
		"└───────────────────────────────────────────────────────────────┘",
	}
	want := strings.Join(wantLines, "\n")
	if got := out; got != want {
		t.Fatalf("RenderDashboard() = %q, want %q", got, want)
	}

	lines := strings.Split(out, "\n")
	if len(lines) != len(wantLines) {
		t.Fatalf("RenderDashboard() line count = %d, want %d", len(lines), len(wantLines))
	}
	for i, wantLine := range wantLines {
		if lines[i] != wantLine {
			t.Fatalf("RenderDashboard() line %d = %q, want %q", i+1, lines[i], wantLine)
		}
	}
}

func TestRenderDashboardIdleIncludesRecentEvents(t *testing.T) {
	t.Parallel()

	out := ui.RenderDashboard(
		time.Date(2026, 4, 22, 15, 4, 5, 0, time.UTC),
		nil,
		[]ui.EventRecord{
			{At: time.Date(2026, 4, 22, 15, 4, 3, 0, time.UTC), Message: "THIS_IS_TEST/file-02.mkv: Copied (new)"},
		},
		0,
		5,
	)
	want := strings.Join([]string{
		"┌─ SYSTEM STATUS ─────────────────────────────────────┐",
		"│ STATE IDLE  ACTIVE 0/5  QUEUE 0  UPDATED 15:04:05 │",
		"└────────────────────────────────────────────────────┘",
		"",
		"┌─ ACTIVE JOBS ─────────┐",
		"│ No active transfers │",
		"└──────────────────────┘",
		"",
		"┌─ RECENT EVENTS ────────────────────────────────────────────────┐",
		"│ [2026-04-22 15:04:03] THIS_IS_TEST/file-02.mkv: Copied (new) │",
		"└───────────────────────────────────────────────────────────────┘",
	}, "\n")
	if got := out; got != want {
		t.Fatalf("RenderDashboard() = %q, want %q", out, want)
	}
}

func TestRenderDashboardShowsQueuedStatusWithoutActiveJobs(t *testing.T) {
	t.Parallel()

	out := ui.RenderDashboard(
		time.Date(2026, 4, 22, 15, 4, 5, 0, time.UTC),
		nil,
		nil,
		3,
		5,
	)
	want := strings.Join([]string{
		"┌─ SYSTEM STATUS ───────────────────────────────────────┐",
		"│ STATE QUEUED  ACTIVE 0/5  QUEUE 3  UPDATED 15:04:05 │",
		"└──────────────────────────────────────────────────────┘",
		"",
		"┌─ ACTIVE JOBS ─────────┐",
		"│ No active transfers │",
		"└──────────────────────┘",
		"",
		"┌─ RECENT EVENTS ─┐",
		"│ 暂无事件      │",
		"└────────────────┘",
	}, "\n")
	if got := out; got != want {
		t.Fatalf("RenderDashboard() = %q, want %q", out, want)
	}
}

func TestRenderDashboardShowsPlaceholderWhenNoEvents(t *testing.T) {
	t.Parallel()

	out := ui.RenderDashboard(
		time.Date(2026, 4, 22, 15, 4, 5, 0, time.UTC),
		[]ui.JobStatus{
			{Name: "job-a", Summary: "832 MiB / 1000 MiB, 83%, 29.793 MiB/s, ETA 5s"},
		},
		nil,
		0,
		5,
	)
	want := strings.Join([]string{
		"┌─ SYSTEM STATUS ────────────────────────────────────────┐",
		"│ STATE RUNNING  ACTIVE 1/5  QUEUE 0  UPDATED 15:04:05 │",
		"└───────────────────────────────────────────────────────┘",
		"",
		"┌─ ACTIVE JOBS ────────────────────────────────────────────┐",
		"│ NAME        PROGRESS  SPEED       ETA       STATUS     │",
		"│ job-a       83%       29.793 MiB/s  ETA 00:05  COPYING │",
		"└─────────────────────────────────────────────────────────┘",
		"",
		"┌─ RECENT EVENTS ─┐",
		"│ 暂无事件      │",
		"└────────────────┘",
	}, "\n")
	if got := out; got != want {
		t.Fatalf("RenderDashboard() = %q, want %q", out, want)
	}
}

func TestRenderDashboardUsesFramedPanelTitles(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 4, 23, 9, 30, 0, 0, time.UTC)
	out := ui.RenderDashboard(now, nil, nil, 0, 3)

	for _, want := range []string{
		"SYSTEM STATUS",
		"ACTIVE JOBS",
		"RECENT EVENTS",
		"┌",
		"└",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("RenderDashboard() missing %q in %q", want, out)
		}
	}
}

func TestRenderDashboardShowsMetricSummaryAndStructuredJobs(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 4, 23, 9, 30, 0, 0, time.UTC)
	out := ui.RenderDashboard(
		now,
		[]ui.JobStatus{
			{Name: "movies-a", Summary: "832 MiB / 1.0 GiB, 83%, 29.8 MiB/s, ETA 5s"},
			{Name: "anime-b", Summary: "等待 rclone 输出"},
		},
		nil,
		2,
		5,
	)

	for _, want := range []string{
		"STATE RUNNING",
		"ACTIVE 2/5",
		"QUEUE 2",
		"UPDATED 09:30:00",
		"movies-a",
		"83%",
		"29.8 MiB/s",
		"ETA 00:05",
		"anime-b",
		"WAITING",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("RenderDashboard() missing %q in %q", want, out)
		}
	}
}

func TestRenderDashboardAlignsWideCharacterJobNames(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 4, 23, 9, 30, 0, 0, time.UTC)
	out := ui.RenderDashboard(
		now,
		[]ui.JobStatus{
			{Name: "动漫-b", Summary: "832 MiB / 1.0 GiB, 83%, 29.8 MiB/s, ETA 5s"},
		},
		nil,
		0,
		5,
	)

	want := "│ 动漫-b      83%       29.8 MiB/s  ETA 00:05  COPYING │"
	if !strings.Contains(out, want) {
		t.Fatalf("RenderDashboard() missing %q in %q", want, out)
	}
}

func TestRenderDashboardFormatsHourETAInStructuredJobs(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 4, 23, 9, 30, 0, 0, time.UTC)
	out := ui.RenderDashboard(
		now,
		[]ui.JobStatus{
			{Name: "movie-a", Summary: "832 MiB / 1.0 GiB, 83%, 29.8 MiB/s, ETA 1h2m3s"},
		},
		nil,
		0,
		5,
	)

	if !strings.Contains(out, "ETA 01:02:03") {
		t.Fatalf("RenderDashboard() missing hour ETA format in %q", out)
	}
}
