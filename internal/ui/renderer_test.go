package ui_test

import (
	"strings"
	"testing"
	"time"
	"unicode/utf8"

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
		"┌─ RECENT EVENTS (0) ─────────┐",
		"│ Watching for new files... │",
		"└────────────────────────────┘",
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
		{At: time.Date(2026, 4, 22, 15, 4, 3, 0, time.UTC), Message: "THIS_IS_TEST/file-02.mkv: Copied (new)"},
		{At: time.Date(2026, 4, 22, 15, 3, 58, 0, time.UTC), Message: "THIS_IS_TEST/file-01.mkv: Copied (new)"},
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
		"┌─ ACTIVE JOBS ───────────────────────────────────────────────┐",
		"│ NAME        PROGRESS  SPEED         ETA           STATUS  │",
		"│ job-a       83%       29.793 MiB/s  ETA 00:05     COPYING │",
		"│ job-b       31%       48.2 MiB/s    ETA 09:12     COPYING │",
		"└────────────────────────────────────────────────────────────┘",
		"",
		"┌─ RECENT EVENTS (2) ────────────────────────────────────────┐",
		"│ 15:04:03  DONE    THIS_IS_TEST/file-02.mkv: Copied (new) │",
		"│ 15:03:58  DONE    THIS_IS_TEST/file-01.mkv: Copied (new) │",
		"└───────────────────────────────────────────────────────────┘",
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
		"┌─ RECENT EVENTS (1) ────────────────────────────────────────┐",
		"│ 15:04:03  DONE    THIS_IS_TEST/file-02.mkv: Copied (new) │",
		"└───────────────────────────────────────────────────────────┘",
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
		"┌─ RECENT EVENTS (0) ─────────┐",
		"│ Watching for new files... │",
		"└────────────────────────────┘",
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
		"┌─ ACTIVE JOBS ───────────────────────────────────────────────┐",
		"│ NAME        PROGRESS  SPEED         ETA           STATUS  │",
		"│ job-a       83%       29.793 MiB/s  ETA 00:05     COPYING │",
		"└────────────────────────────────────────────────────────────┘",
		"",
		"┌─ RECENT EVENTS (0) ─────────┐",
		"│ Watching for new files... │",
		"└────────────────────────────┘",
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

func TestRenderDashboardShowsTaggedRecentEventsNewestFirst(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 4, 23, 9, 30, 0, 0, time.UTC)
	out := ui.RenderDashboard(
		now,
		nil,
		[]ui.EventRecord{
			{At: time.Date(2026, 4, 23, 9, 29, 59, 0, time.UTC), Message: "THIS_IS_TEST/file-02.mkv: Copied (new)"},
			{At: time.Date(2026, 4, 23, 9, 29, 58, 0, time.UTC), Message: "[movie] 启动扫描发现 3 个文件，任务标记为待上传"},
		},
		0,
		5,
	)

	if !strings.Contains(out, "RECENT EVENTS (2)") {
		t.Fatalf("RenderDashboard() missing event count in %q", out)
	}
	first := strings.Index(out, "09:29:59")
	second := strings.Index(out, "09:29:58")
	if first == -1 || second == -1 || first > second {
		t.Fatalf("RenderDashboard() event order is not newest first: %q", out)
	}
	for _, want := range []string{"DONE", "SCAN", "THIS_IS_TEST/file-02.mkv", "[movie] 启动扫描发现 3 个文件"} {
		if !strings.Contains(out, want) {
			t.Fatalf("RenderDashboard() missing %q in %q", want, out)
		}
	}
}

func TestRenderDashboardTagsFailureEventsBeforeRetryQueue(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 4, 23, 9, 30, 0, 0, time.UTC)
	out := ui.RenderDashboard(
		now,
		nil,
		[]ui.EventRecord{
			{At: time.Date(2026, 4, 23, 9, 29, 59, 0, time.UTC), Message: "[movie] 上传失败，进入重试等待"},
		},
		0,
		5,
	)

	if !strings.Contains(out, "09:29:59  FAIL") {
		t.Fatalf("RenderDashboard() missing FAIL tag for retrying failure event in %q", out)
	}
	if strings.Contains(out, "09:29:59  QUEUE") {
		t.Fatalf("RenderDashboard() mis-tagged retrying failure event as QUEUE in %q", out)
	}
}

func TestRenderDashboardTagsSuccessfulRequeueEventsAsDone(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 4, 23, 9, 30, 0, 0, time.UTC)
	out := ui.RenderDashboard(
		now,
		nil,
		[]ui.EventRecord{
			{At: time.Date(2026, 4, 23, 9, 29, 59, 0, time.UTC), Message: "[JOB] 上传完成，检测到新增文件，重新排队"},
		},
		0,
		5,
	)

	if !strings.Contains(out, "09:29:59  DONE") {
		t.Fatalf("RenderDashboard() missing DONE tag for successful requeue event in %q", out)
	}
	if strings.Contains(out, "09:29:59  QUEUE") {
		t.Fatalf("RenderDashboard() mis-tagged successful requeue event as QUEUE in %q", out)
	}
}

func TestRenderDashboardPreservesProvidedEventOrder(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 4, 23, 9, 30, 0, 0, time.UTC)
	out := ui.RenderDashboard(
		now,
		nil,
		[]ui.EventRecord{
			{At: time.Date(2026, 4, 23, 9, 29, 58, 0, time.UTC), Message: "older"},
			{At: time.Date(2026, 4, 23, 9, 29, 59, 0, time.UTC), Message: "newer"},
		},
		0,
		5,
	)

	first := strings.Index(out, "09:29:58")
	second := strings.Index(out, "09:29:59")
	if first == -1 || second == -1 || first > second {
		t.Fatalf("RenderDashboard() did not preserve provided event order: %q", out)
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

	want := "│ 动漫-b      83%       29.8 MiB/s    ETA 00:05     COPYING │"
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

func TestRenderDashboardKeepsActiveJobColumnsAlignedForRuntimeValues(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 4, 23, 9, 30, 0, 0, time.UTC)
	out := ui.RenderDashboard(
		now,
		[]ui.JobStatus{
			{Name: "job-a", Summary: "832 MiB / 1000 MiB, 83%, 29.793 MiB/s, ETA 5s"},
			{Name: "job-b", Summary: "12.4 GiB / 40.0 GiB, 31%, 48.2 MiB/s, ETA 1h2m3s"},
		},
		nil,
		0,
		5,
	)

	lines := strings.Split(out, "\n")
	var header string
	var rows []string
	for _, line := range lines {
		if strings.Contains(line, "NAME") && strings.Contains(line, "PROGRESS") && strings.Contains(line, "STATUS") {
			header = strings.TrimSpace(strings.Trim(line, "│"))
			continue
		}
		if strings.Contains(line, "job-a") || strings.Contains(line, "job-b") {
			rows = append(rows, strings.TrimSpace(strings.Trim(line, "│")))
		}
	}

	if header == "" || len(rows) != 2 {
		t.Fatalf("RenderDashboard() did not render expected active job table in %q", out)
	}

	headerStarts := columnStarts(header)
	if len(headerStarts) != 5 {
		t.Fatalf("active job header columns = %v, want 5 columns in %q", headerStarts, header)
	}

	for _, row := range rows {
		rowStarts := columnStarts(row)
		if len(rowStarts) != len(headerStarts) {
			t.Fatalf("active job row columns = %v, want %v in %q", rowStarts, headerStarts, row)
		}
		for i := range headerStarts {
			if rowStarts[i] != headerStarts[i] {
				t.Fatalf("active job row column %d starts at %d, want %d; row=%q header=%q", i, rowStarts[i], headerStarts[i], row, header)
			}
		}
	}
}

func TestRenderDashboardShowsMonthDayTimeForOlderEvents(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 4, 23, 9, 30, 0, 0, time.UTC)
	out := ui.RenderDashboard(
		now,
		nil,
		[]ui.EventRecord{
			{At: time.Date(2026, 4, 22, 23, 59, 0, 0, time.UTC), Message: "older"},
		},
		0,
		5,
	)

	if !strings.Contains(out, "04-22 23:59  INFO") {
		t.Fatalf("RenderDashboard() missing non-same-day timestamp fallback in %q", out)
	}
}

func columnStarts(line string) []int {
	starts := []int{0}
	width := 0
	spaceRun := 0

	for len(line) > 0 {
		r, size := utf8.DecodeRuneInString(line)
		line = line[size:]

		runeWidth := 1
		if r >= 0x1100 && (r <= 0x115f ||
			r == 0x2329 ||
			r == 0x232a ||
			(r >= 0x2e80 && r <= 0xa4cf && r != 0x303f) ||
			(r >= 0xac00 && r <= 0xd7a3) ||
			(r >= 0xf900 && r <= 0xfaff) ||
			(r >= 0xfe10 && r <= 0xfe19) ||
			(r >= 0xfe30 && r <= 0xfe6f) ||
			(r >= 0xff00 && r <= 0xff60) ||
			(r >= 0xffe0 && r <= 0xffe6) ||
			(r >= 0x1f300 && r <= 0x1f64f) ||
			(r >= 0x1f900 && r <= 0x1f9ff) ||
			(r >= 0x20000 && r <= 0x3fffd)) {
			runeWidth = 2
		}

		if r == ' ' {
			spaceRun += runeWidth
		} else {
			if spaceRun >= 2 {
				starts = append(starts, width)
			}
			spaceRun = 0
		}
		width += runeWidth
	}

	return starts
}
