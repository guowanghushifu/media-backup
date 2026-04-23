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
		"┌──────────────────────────────────────────────────────────────────────────────┐",
		"│ ┌─ 系统状态 ───────────────────────────────────────────────────────────────┐ │",
		"│ │ 状态 空闲 ｜ 活动 0/0 ｜ 排队 0 ｜ 更新 15:04:05                         │ │",
		"│ └──────────────────────────────────────────────────────────────────────────┘ │",
		"│                                                                              │",
		"│ ┌─ 活动任务 ───────────────────────────────────────────────────────────────┐ │",
		"│ │ 暂无活动任务                                                             │ │",
		"│ │                                                                          │ │",
		"│ │                                                                          │ │",
		"│ │                                                                          │ │",
		"│ │                                                                          │ │",
		"│ │                                                                          │ │",
		"│ │                                                                          │ │",
		"│ │                                                                          │ │",
		"│ │                                                                          │ │",
		"│ │                                                                          │ │",
		"│ └──────────────────────────────────────────────────────────────────────────┘ │",
		"│                                                                              │",
		"│ ┌─ 最近事件 (0) ───────────────────────────────────────────────────────────┐ │",
		"│ │ 正在等待新文件...                                                        │ │",
		"│ │                                                                          │ │",
		"│ │                                                                          │ │",
		"│ │                                                                          │ │",
		"│ │                                                                          │ │",
		"│ │                                                                          │ │",
		"│ │                                                                          │ │",
		"│ │                                                                          │ │",
		"│ │                                                                          │ │",
		"│ │                                                                          │ │",
		"│ └──────────────────────────────────────────────────────────────────────────┘ │",
		"└──────────────────────────────────────────────────────────────────────────────┘",
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
		"┌──────────────────────────────────────────────────────────────────────────────┐",
		"│ ┌─ 系统状态 ───────────────────────────────────────────────────────────────┐ │",
		"│ │ 状态 运行中 ｜ 活动 2/5 ｜ 排队 1 ｜ 更新 15:04:05                       │ │",
		"│ └──────────────────────────────────────────────────────────────────────────┘ │",
		"│                                                                              │",
		"│ ┌─ 活动任务 ───────────────────────────────────────────────────────────────┐ │",
		"│ │ 名称                   进度      速度              预计           状态   │ │",
		"│ │ job-a                  83%       29.793 MiB/s      预计 00:05     传输中 │ │",
		"│ │ job-b                  31%       48.2 MiB/s        预计 09:12     传输中 │ │",
		"│ │                                                                          │ │",
		"│ │                                                                          │ │",
		"│ │                                                                          │ │",
		"│ │                                                                          │ │",
		"│ │                                                                          │ │",
		"│ │                                                                          │ │",
		"│ │                                                                          │ │",
		"│ └──────────────────────────────────────────────────────────────────────────┘ │",
		"│                                                                              │",
		"│ ┌─ 最近事件 (2) ───────────────────────────────────────────────────────────┐ │",
		"│ │ 15:04:03  完成    THIS_IS_TEST/file-02.mkv: Copied (new)                 │ │",
		"│ │ 15:03:58  完成    THIS_IS_TEST/file-01.mkv: Copied (new)                 │ │",
		"│ │                                                                          │ │",
		"│ │                                                                          │ │",
		"│ │                                                                          │ │",
		"│ │                                                                          │ │",
		"│ │                                                                          │ │",
		"│ │                                                                          │ │",
		"│ │                                                                          │ │",
		"│ │                                                                          │ │",
		"│ └──────────────────────────────────────────────────────────────────────────┘ │",
		"└──────────────────────────────────────────────────────────────────────────────┘",
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
		"┌──────────────────────────────────────────────────────────────────────────────┐",
		"│ ┌─ 系统状态 ───────────────────────────────────────────────────────────────┐ │",
		"│ │ 状态 空闲 ｜ 活动 0/5 ｜ 排队 0 ｜ 更新 15:04:05                         │ │",
		"│ └──────────────────────────────────────────────────────────────────────────┘ │",
		"│                                                                              │",
		"│ ┌─ 活动任务 ───────────────────────────────────────────────────────────────┐ │",
		"│ │ 暂无活动任务                                                             │ │",
		"│ │                                                                          │ │",
		"│ │                                                                          │ │",
		"│ │                                                                          │ │",
		"│ │                                                                          │ │",
		"│ │                                                                          │ │",
		"│ │                                                                          │ │",
		"│ │                                                                          │ │",
		"│ │                                                                          │ │",
		"│ │                                                                          │ │",
		"│ └──────────────────────────────────────────────────────────────────────────┘ │",
		"│                                                                              │",
		"│ ┌─ 最近事件 (1) ───────────────────────────────────────────────────────────┐ │",
		"│ │ 15:04:03  完成    THIS_IS_TEST/file-02.mkv: Copied (new)                 │ │",
		"│ │                                                                          │ │",
		"│ │                                                                          │ │",
		"│ │                                                                          │ │",
		"│ │                                                                          │ │",
		"│ │                                                                          │ │",
		"│ │                                                                          │ │",
		"│ │                                                                          │ │",
		"│ │                                                                          │ │",
		"│ │                                                                          │ │",
		"│ └──────────────────────────────────────────────────────────────────────────┘ │",
		"└──────────────────────────────────────────────────────────────────────────────┘",
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
		"┌──────────────────────────────────────────────────────────────────────────────┐",
		"│ ┌─ 系统状态 ───────────────────────────────────────────────────────────────┐ │",
		"│ │ 状态 排队中 ｜ 活动 0/5 ｜ 排队 3 ｜ 更新 15:04:05                       │ │",
		"│ └──────────────────────────────────────────────────────────────────────────┘ │",
		"│                                                                              │",
		"│ ┌─ 活动任务 ───────────────────────────────────────────────────────────────┐ │",
		"│ │ 暂无活动任务                                                             │ │",
		"│ │                                                                          │ │",
		"│ │                                                                          │ │",
		"│ │                                                                          │ │",
		"│ │                                                                          │ │",
		"│ │                                                                          │ │",
		"│ │                                                                          │ │",
		"│ │                                                                          │ │",
		"│ │                                                                          │ │",
		"│ │                                                                          │ │",
		"│ └──────────────────────────────────────────────────────────────────────────┘ │",
		"│                                                                              │",
		"│ ┌─ 最近事件 (0) ───────────────────────────────────────────────────────────┐ │",
		"│ │ 正在等待新文件...                                                        │ │",
		"│ │                                                                          │ │",
		"│ │                                                                          │ │",
		"│ │                                                                          │ │",
		"│ │                                                                          │ │",
		"│ │                                                                          │ │",
		"│ │                                                                          │ │",
		"│ │                                                                          │ │",
		"│ │                                                                          │ │",
		"│ │                                                                          │ │",
		"│ └──────────────────────────────────────────────────────────────────────────┘ │",
		"└──────────────────────────────────────────────────────────────────────────────┘",
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
		"┌──────────────────────────────────────────────────────────────────────────────┐",
		"│ ┌─ 系统状态 ───────────────────────────────────────────────────────────────┐ │",
		"│ │ 状态 运行中 ｜ 活动 1/5 ｜ 排队 0 ｜ 更新 15:04:05                       │ │",
		"│ └──────────────────────────────────────────────────────────────────────────┘ │",
		"│                                                                              │",
		"│ ┌─ 活动任务 ───────────────────────────────────────────────────────────────┐ │",
		"│ │ 名称                   进度      速度              预计           状态   │ │",
		"│ │ job-a                  83%       29.793 MiB/s      预计 00:05     传输中 │ │",
		"│ │                                                                          │ │",
		"│ │                                                                          │ │",
		"│ │                                                                          │ │",
		"│ │                                                                          │ │",
		"│ │                                                                          │ │",
		"│ │                                                                          │ │",
		"│ │                                                                          │ │",
		"│ │                                                                          │ │",
		"│ └──────────────────────────────────────────────────────────────────────────┘ │",
		"│                                                                              │",
		"│ ┌─ 最近事件 (0) ───────────────────────────────────────────────────────────┐ │",
		"│ │ 正在等待新文件...                                                        │ │",
		"│ │                                                                          │ │",
		"│ │                                                                          │ │",
		"│ │                                                                          │ │",
		"│ │                                                                          │ │",
		"│ │                                                                          │ │",
		"│ │                                                                          │ │",
		"│ │                                                                          │ │",
		"│ │                                                                          │ │",
		"│ │                                                                          │ │",
		"│ └──────────────────────────────────────────────────────────────────────────┘ │",
		"└──────────────────────────────────────────────────────────────────────────────┘",
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
		"系统状态",
		"活动任务",
		"最近事件",
		"┌",
		"└",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("RenderDashboard() missing %q in %q", want, out)
		}
	}
}

func TestRenderDashboardUsesUniformPanelWidth(t *testing.T) {
	t.Parallel()

	out := ui.RenderDashboard(
		time.Date(2026, 4, 23, 10, 1, 1, 0, time.UTC),
		nil,
		nil,
		0,
		5,
	)

	lines := strings.Split(out, "\n")
	var widths []int
	for _, line := range lines {
		if strings.HasPrefix(line, "┌") || strings.HasPrefix(line, "└") || strings.HasPrefix(line, "│ ┌") || strings.HasPrefix(line, "│ └") {
			widths = append(widths, displayWidth(line))
		}
	}

	if len(widths) != 8 {
		t.Fatalf("panel border count = %d, want 8 in %q", len(widths), out)
	}
	for i := 1; i < len(widths); i++ {
		if widths[i] != widths[0] {
			t.Fatalf("panel border widths = %v, want all equal", widths)
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
		"状态 运行中",
		"｜ 活动 2/5",
		"｜ 排队 2",
		"｜ 更新 09:30:00",
		"movies-a",
		"83%",
		"29.8 MiB/s",
		"预计 00:05",
		"anime-b",
		"等待中",
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

	if !strings.Contains(out, "最近事件 (2)") {
		t.Fatalf("RenderDashboard() missing event count in %q", out)
	}
	first := strings.Index(out, "09:29:59")
	second := strings.Index(out, "09:29:58")
	if first == -1 || second == -1 || first > second {
		t.Fatalf("RenderDashboard() event order is not newest first: %q", out)
	}
	for _, want := range []string{"完成", "扫描", "THIS_IS_TEST/file-02.mkv", "[movie] 启动扫描发现 3 个文件"} {
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

	if !strings.Contains(out, "09:29:59  失败") {
		t.Fatalf("RenderDashboard() missing failure tag for retrying failure event in %q", out)
	}
	if strings.Contains(out, "09:29:59  排队") {
		t.Fatalf("RenderDashboard() mis-tagged retrying failure event as queue in %q", out)
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

	if !strings.Contains(out, "09:29:59  完成") {
		t.Fatalf("RenderDashboard() missing done tag for successful requeue event in %q", out)
	}
	if strings.Contains(out, "09:29:59  排队") {
		t.Fatalf("RenderDashboard() mis-tagged successful requeue event as queue in %q", out)
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

	for _, line := range strings.Split(out, "\n") {
		if strings.Contains(line, "动漫-b") {
			got := strings.TrimRight(stripNestedFrameLine(line), " ")
			want := "动漫-b                 83%       29.8 MiB/s        预计 00:05     传输中"
			if got != want {
				t.Fatalf("wide-character row = %q, want %q", got, want)
			}
			return
		}
	}

	t.Fatalf("RenderDashboard() missing wide-character job row in %q", out)
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

	if !strings.Contains(out, "预计 01:02:03") {
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
			{Name: "job-b", Summary: "12.4 GiB / 40.0 GiB, 31%, 112.342 MiB/s, ETA 1h2m3s"},
		},
		nil,
		0,
		5,
	)

	lines := strings.Split(out, "\n")
	var header string
	var rows []string
	for _, line := range lines {
		if strings.Contains(line, "名称") && strings.Contains(line, "进度") && strings.Contains(line, "状态") {
			header = stripNestedFrameLine(line)
			continue
		}
		if strings.Contains(line, "job-a") || strings.Contains(line, "job-b") {
			rows = append(rows, stripNestedFrameLine(line))
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

func TestRenderDashboardTruncatesLongJobNamesWithoutShiftingColumns(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 4, 23, 9, 30, 0, 0, time.UTC)
	out := ui.RenderDashboard(
		now,
		[]ui.JobStatus{
			{Name: "VERY-LONG-TV-SHOW-S01E02-1080P-WEB", Summary: "832 MiB / 1000 MiB, 83%, 29.793 MiB/s, ETA 5s"},
			{Name: "job-b", Summary: "12.4 GiB / 40.0 GiB, 31%, 112.342 MiB/s, ETA 1h2m3s"},
		},
		nil,
		0,
		5,
	)

	lines := strings.Split(out, "\n")
	var header string
	var rows []string
	for _, line := range lines {
		if strings.Contains(line, "名称") && strings.Contains(line, "进度") && strings.Contains(line, "状态") {
			header = stripNestedFrameLine(line)
			continue
		}
		if strings.Contains(line, "VERY-LONG-TV") || strings.Contains(line, "job-b") {
			rows = append(rows, stripNestedFrameLine(line))
		}
	}

	if header == "" || len(rows) != 2 {
		t.Fatalf("RenderDashboard() did not render expected rows in %q", out)
	}

	if !strings.Contains(rows[0], "VERY-LONG-TV-SHOW-...") {
		t.Fatalf("long-name row = %q, want truncated name", rows[0])
	}
	if strings.Contains(rows[0], "VERY-LONG-TV-SHOW-S01E02-1080P-WEB") {
		t.Fatalf("long-name row = %q, want original name to be truncated", rows[0])
	}

	headerStarts := columnStarts(header)
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

func TestRenderDashboardTruncatesActiveFileNamesByDisplayWidth(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 4, 23, 9, 30, 0, 0, time.UTC)
	longName := "中文剧集名称合集S01E02-1080p-WEB-DL-Group.mkv"
	out := ui.RenderDashboardWithWidth(
		now,
		[]ui.JobStatus{
			{Name: longName, Summary: "832 MiB / 1000 MiB, 83%, 29.793 MiB/s, ETA 5s"},
		},
		nil,
		0,
		5,
		120,
	)

	var row string
	for _, line := range strings.Split(out, "\n") {
		if strings.Contains(line, "83%") {
			row = stripNestedFrameLine(line)
			break
		}
	}
	if row == "" {
		t.Fatalf("RenderDashboard() missing active job row in %q", out)
	}

	wantName := trimDisplayToWidth(longName, 40)
	if !strings.Contains(row, wantName) {
		t.Fatalf("active row = %q, want 40-column display truncation %q", row, wantName)
	}
	if strings.Contains(row, "1080p-WEB-DL-Group.mkv") {
		t.Fatalf("active row = %q, want tail to be truncated", row)
	}
}

func TestRenderDashboardTruncatesMixedWidthActiveNamesToFortyColumnsAndKeepsAlignment(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 4, 23, 9, 30, 0, 0, time.UTC)
	longMixedName := "电影Movie合集01-特别篇-1080p-Group-Extra-Cut.mkv"
	out := ui.RenderDashboardWithWidth(
		now,
		[]ui.JobStatus{
			{Name: longMixedName, Summary: "832 MiB / 1000 MiB, 83%, 29.793 MiB/s, ETA 5s"},
			{Name: "短片Movie.mkv", Summary: "12.4 GiB / 40.0 GiB, 31%, 112.342 MiB/s, ETA 1h2m3s"},
		},
		nil,
		0,
		5,
		120,
	)

	lines := strings.Split(out, "\n")
	var header string
	var mixedRow string
	var shortRow string
	for _, line := range lines {
		if strings.Contains(line, "名称") && strings.Contains(line, "进度") && strings.Contains(line, "状态") {
			header = stripNestedFrameLine(line)
			continue
		}
		if strings.Contains(line, "83%") {
			mixedRow = stripNestedFrameLine(line)
		}
		if strings.Contains(line, "31%") {
			shortRow = stripNestedFrameLine(line)
		}
	}

	if header == "" || mixedRow == "" || shortRow == "" {
		t.Fatalf("RenderDashboard() did not render expected active job rows in %q", out)
	}

	wantName := trimDisplayToWidth(longMixedName, 40)
	if !strings.Contains(mixedRow, wantName) {
		t.Fatalf("mixed-width row = %q, want 40-column truncated name %q", mixedRow, wantName)
	}
	if got := displayWidth(firstColumn(header, mixedRow)); got != 40 {
		t.Fatalf("mixed-width name cell width = %d, want 40 in %q", got, mixedRow)
	}

	headerStarts := columnStarts(header)
	for _, row := range []string{mixedRow, shortRow} {
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

func stripNestedFrameLine(line string) string {
	trimmed := strings.TrimSpace(line)
	trimmed = strings.TrimPrefix(trimmed, "│ ")
	trimmed = strings.TrimSuffix(trimmed, " │")
	trimmed = strings.TrimPrefix(trimmed, "│ ")
	trimmed = strings.TrimSuffix(trimmed, " │")
	return trimmed
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

	if !strings.Contains(out, "04-22 23:59  信息") {
		t.Fatalf("RenderDashboard() missing non-same-day timestamp fallback in %q", out)
	}
}

func TestRenderDashboardTruncatesLongEventToRequestedWidth(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 4, 23, 10, 54, 30, 0, time.UTC)
	out := ui.RenderDashboardWithWidth(
		now,
		nil,
		[]ui.EventRecord{
			{
				At:      time.Date(2026, 4, 23, 10, 54, 20, 0, time.UTC),
				Message: "琅琊榜 (2015) {tmdb-64197}/S01/琅琊榜 S01E03 - 2160p.WEB-DL.H265.AAC-HHWEB.mp4: Copied (new)",
			},
		},
		0,
		5,
		50,
	)

	var eventLine string
	for _, line := range strings.Split(out, "\n") {
		if got := displayWidth(line); got > 50 {
			t.Fatalf("RenderDashboardWithWidth() line width = %d, want <= 50 in %q", got, line)
		}
		if strings.Contains(line, "10:54:20") {
			eventLine = line
		}
	}

	if eventLine == "" {
		t.Fatalf("RenderDashboardWithWidth() missing long event line in %q", out)
	}
	if !strings.Contains(eventLine, "...") {
		t.Fatalf("RenderDashboardWithWidth() event line was not truncated: %q", eventLine)
	}
	if !strings.Contains(eventLine, "琅琊榜") {
		t.Fatalf("RenderDashboardWithWidth() event line lost leading event text: %q", eventLine)
	}
	if strings.Contains(eventLine, "Copied (new)") {
		t.Fatalf("RenderDashboardWithWidth() event line kept full tail instead of truncating: %q", eventLine)
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

func displayWidth(text string) int {
	width := 0
	for len(text) > 0 {
		r, size := utf8.DecodeRuneInString(text)
		text = text[size:]

		switch {
		case r == 0:
			continue
		case r >= 0x0300 && r <= 0x036f:
			continue
		case r >= 0x1100 && (r <= 0x115f ||
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
			(r >= 0x20000 && r <= 0x3fffd)):
			width += 2
		default:
			width++
		}
	}

	return width
}

func trimDisplayToWidth(text string, width int) string {
	if displayWidth(text) <= width {
		return text
	}
	if width <= 3 {
		return strings.Repeat(".", width)
	}

	target := width - 3
	current := 0
	var b strings.Builder
	for len(text) > 0 {
		r, size := utf8.DecodeRuneInString(text)
		text = text[size:]

		runeWidth := 1
		if runeWidthForTest(r) == 2 {
			runeWidth = 2
		}
		if current+runeWidth > target {
			break
		}
		b.WriteRune(r)
		current += runeWidth
	}
	b.WriteString("...")
	return b.String()
}

func firstColumn(header string, row string) string {
	starts := columnStarts(header)
	if len(starts) < 2 {
		return strings.TrimRight(row, " ")
	}
	return trimToDisplayWidth(row, starts[1]-2)
}

func trimToDisplayWidth(text string, width int) string {
	if width <= 0 {
		return ""
	}

	current := 0
	var b strings.Builder
	for len(text) > 0 {
		r, size := utf8.DecodeRuneInString(text)
		text = text[size:]

		runeWidth := runeWidthForTest(r)
		if current+runeWidth > width {
			break
		}
		b.WriteRune(r)
		current += runeWidth
	}
	return strings.TrimRight(b.String(), " ")
}

func runeWidthForTest(r rune) int {
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
		return 2
	}
	return 1
}
