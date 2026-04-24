package app

import (
	"context"
	"errors"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/guowanghushifu/media-backup/internal/config"
	"github.com/guowanghushifu/media-backup/internal/queue"
	"github.com/guowanghushifu/media-backup/internal/ui"
	"github.com/guowanghushifu/media-backup/internal/watcher"
)

func TestSnapshotUIIncludesRecentEventsWhileIdle(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 4, 22, 17, 4, 10, 0, time.UTC)
	s := newTestService()

	s.appendRecentEvent(now.Add(-2*time.Second), "THIS_IS_TEST/uploadtest.bin: Copied (new)")

	active, events, waiting := s.snapshotUI()
	if waiting != 0 {
		t.Fatalf("waiting = %d, want 0", waiting)
	}
	if len(active) != 0 {
		t.Fatalf("len(active) = %d, want 0", len(active))
	}
	if len(events) != 1 {
		t.Fatalf("len(events) = %d, want 1", len(events))
	}
	if events[0].At != now.Add(-2*time.Second) {
		t.Fatalf("events[0].At = %v, want %v", events[0].At, now.Add(-2*time.Second))
	}
	if events[0].Message != "THIS_IS_TEST/uploadtest.bin: Copied (new)" {
		t.Fatalf("events[0].Message = %q, want copied event", events[0].Message)
	}
}

func TestSnapshotUISortsActiveJobsByName(t *testing.T) {
	t.Parallel()

	s := newTestService()
	s.jobs["/links/movie/feature.mkv"] = &jobRuntime{
		cfg:        config.JobConfig{Name: "MOVIE"},
		key:        "/links/movie/feature.mkv",
		sourcePath: "/source/movie/feature.mkv",
		linkPath:   "/links/movie/zeta-feature.mkv",
		remoteDir:  "remote:movie/",
		summary:    "movie summary",
		active:     true,
	}
	s.jobs["/links/remux/feature.mkv"] = &jobRuntime{
		cfg:        config.JobConfig{Name: "4K-REMUX"},
		key:        "/links/remux/feature.mkv",
		sourcePath: "/source/remux/feature.mkv",
		linkPath:   "/links/remux/feature.mkv",
		remoteDir:  "remote:remux/",
		summary:    "remux summary",
		active:     true,
	}

	for i := 0; i < 50; i++ {
		active, _, _ := s.snapshotUI()
		if len(active) != 2 {
			t.Fatalf("len(active) = %d, want 2", len(active))
		}
		if active[0].Name != "feature.mkv" || active[1].Name != "zeta-feature.mkv" {
			t.Fatalf("active order = [%s %s], want [feature.mkv zeta-feature.mkv]", active[0].Name, active[1].Name)
		}
	}
}

func TestSnapshotUIRowsUseStableHiddenTiebreakerForIdenticalVisibleContent(t *testing.T) {
	t.Parallel()

	s := newTestService()
	s.jobs["job-b"] = &jobRuntime{
		cfg:        config.JobConfig{Name: "MOVIE"},
		key:        "job-b",
		sourcePath: "/source/library-b/feature.mkv",
		linkPath:   "/links/library-b/feature.mkv",
		remoteDir:  "remote:library-b/",
		summary:    "42 GiB / 100 GiB, 42%, 12 MiB/s, ETA 1h2m3s",
		active:     true,
	}
	s.jobs["job-a"] = &jobRuntime{
		cfg:        config.JobConfig{Name: "MOVIE"},
		key:        "job-a",
		sourcePath: "/source/library-a/feature.mkv",
		linkPath:   "/links/library-a/feature.mkv",
		remoteDir:  "remote:library-a/",
		summary:    "42 GiB / 100 GiB, 42%, 12 MiB/s, ETA 1h2m3s",
		active:     true,
	}

	rows := s.snapshotUIRows()
	if len(rows) != 2 {
		t.Fatalf("len(rows) = %d, want 2", len(rows))
	}
	if got, want := rows[0].jobKey, "job-a"; got != want {
		t.Fatalf("rows[0].jobKey = %q, want %q", got, want)
	}
	if got, want := rows[1].jobKey, "job-b"; got != want {
		t.Fatalf("rows[1].jobKey = %q, want %q", got, want)
	}

	active, _, _ := s.snapshotUI()
	if len(active) != 2 {
		t.Fatalf("len(active) = %d, want 2", len(active))
	}
	wantStatus := ui.JobStatus{
		Name:    "feature.mkv",
		Summary: "42 GiB / 100 GiB, 42%, 12 MiB/s, ETA 1h2m3s",
	}
	if got := active[0]; got != wantStatus {
		t.Fatalf("active[0] = %#v, want %#v", got, wantStatus)
	}
	if got := active[1]; got != wantStatus {
		t.Fatalf("active[1] = %#v, want %#v", got, wantStatus)
	}
}

func TestSnapshotUIUsesActiveFileBasenameInsteadOfConfigName(t *testing.T) {
	t.Parallel()

	s := newTestService()
	s.jobs["/links/remux/示例电影.S01E02.mkv"] = &jobRuntime{
		cfg:        config.JobConfig{Name: "4K-REMUX"},
		key:        "/links/remux/示例电影.S01E02.mkv",
		sourcePath: "/source/remux/示例电影.S01E02.mkv",
		linkPath:   "/links/remux/示例电影.S01E02.mkv",
		remoteDir:  "remote:remux/",
		summary:    "movie summary",
		active:     true,
	}

	active, _, _ := s.snapshotUI()
	if len(active) != 1 {
		t.Fatalf("len(active) = %d, want 1", len(active))
	}
	if got, want := active[0].Name, "示例电影.S01E02.mkv"; got != want {
		t.Fatalf("active[0].Name = %q, want %q", got, want)
	}
	if active[0].Name == "4K-REMUX" {
		t.Fatalf("active[0].Name = %q, want basename instead of config name", active[0].Name)
	}
}

func TestConfigJobListDoesNotDeriveConfigFromRuntimeTasks(t *testing.T) {
	t.Parallel()

	s := newTestService()
	s.jobs["/links/movie/feature.mkv"] = &jobRuntime{
		cfg: config.JobConfig{
			Name:         "MOVIE",
			SourceDir:    "/source/movie",
			LinkDir:      "/links/movie",
			RcloneRemote: "remote:movie",
		},
		key:        "/links/movie/feature.mkv",
		sourcePath: "/source/movie/feature.mkv",
		linkPath:   "/links/movie/feature.mkv",
		remoteDir:  "remote:movie/",
	}

	if jobs := s.configJobList(); len(jobs) != 0 {
		t.Fatalf("configJobList() = %v, want no config jobs when configJobs is empty", jobs)
	}
}

func TestSnapshotUIReturnsNewestEventsFirst(t *testing.T) {
	t.Parallel()

	s := newTestService()
	base := time.Date(2026, 4, 23, 9, 30, 0, 0, time.UTC)
	s.appendRecentEvent(base.Add(-2*time.Second), "older")
	s.appendRecentEvent(base.Add(-1*time.Second), "newer")

	_, events, _ := s.snapshotUI()
	if got, want := events[0].Message, "newer"; got != want {
		t.Fatalf("events[0].Message = %q, want %q", got, want)
	}
	if got, want := events[1].Message, "older"; got != want {
		t.Fatalf("events[1].Message = %q, want %q", got, want)
	}
}

func TestAppendRecentEventKeepsOnlyLatestTen(t *testing.T) {
	t.Parallel()

	base := time.Date(2026, 4, 22, 17, 4, 10, 0, time.UTC)
	s := newTestService()

	for i := 0; i < 12; i++ {
		s.appendRecentEvent(base.Add(time.Duration(i)*time.Second), "event-"+strconv.Itoa(i))
	}

	if len(s.recentEvents) != 10 {
		t.Fatalf("len(recentEvents) = %d, want 10", len(s.recentEvents))
	}
	if s.recentEvents[0].message != "event-2" {
		t.Fatalf("recentEvents[0].message = %q, want %q", s.recentEvents[0].message, "event-2")
	}
	if s.recentEvents[0].at != base.Add(2*time.Second) {
		t.Fatalf("recentEvents[0].at = %v, want %v", s.recentEvents[0].at, base.Add(2*time.Second))
	}
	if s.recentEvents[9].message != "event-11" {
		t.Fatalf("recentEvents[9].message = %q, want %q", s.recentEvents[9].message, "event-11")
	}
	if s.recentEvents[9].at != base.Add(11*time.Second) {
		t.Fatalf("recentEvents[9].at = %v, want %v", s.recentEvents[9].at, base.Add(11*time.Second))
	}
}

func TestHandleRcloneOutputLineRecordsParsedTransferEvent(t *testing.T) {
	t.Parallel()

	at := time.Date(2026, 4, 22, 17, 4, 17, 0, time.UTC)
	s := newTestService()
	job := &jobRuntime{
		cfg:        config.JobConfig{Name: "movie"},
		key:        "/link/movie.mkv",
		sourcePath: "/source/movie.mkv",
		linkPath:   "/link/movie.mkv",
		remoteDir:  "remote:movie/",
	}

	s.handleRcloneOutputLine(job, "2026/04/22 17:04:17 INFO  : THIS_IS_TEST/uploadtest.bin: Copied (new)", func() time.Time {
		return at
	})

	if len(s.recentEvents) != 1 {
		t.Fatalf("len(recentEvents) = %d, want 1", len(s.recentEvents))
	}
	if s.recentEvents[0].at != at {
		t.Fatalf("recentEvents[0].at = %v, want %v", s.recentEvents[0].at, at)
	}
	if s.recentEvents[0].message != "THIS_IS_TEST/uploadtest.bin: Copied (new)" {
		t.Fatalf("recentEvents[0].message = %q, want copied event", s.recentEvents[0].message)
	}
}

func TestHandleRcloneOutputLineUpdatesSummaryForStatsWithoutRecentEvent(t *testing.T) {
	t.Parallel()

	s := newTestService()
	job := &jobRuntime{
		cfg:        config.JobConfig{Name: "movie"},
		key:        "/link/movie.mkv",
		sourcePath: "/source/movie.mkv",
		linkPath:   "/link/movie.mkv",
		remoteDir:  "remote:movie/",
	}

	s.handleRcloneOutputLine(job, "2026/04/22 17:04:18 INFO  :       42 GiB / 100 GiB, 42%, 12 MiB/s, ETA 1h2m3s", func() time.Time {
		t.Fatal("now() should not be called for stats lines")
		return time.Time{}
	})

	if job.summary != "42 GiB / 100 GiB, 42%, 12 MiB/s, ETA 1h2m3s" {
		t.Fatalf("job.summary = %q, want parsed stats summary", job.summary)
	}
	if len(s.recentEvents) != 0 {
		t.Fatalf("len(recentEvents) = %d, want 0", len(s.recentEvents))
	}
}

func TestRunUILoopSkipsUnchangedRefreshUntilKeepalive(t *testing.T) {
	t.Parallel()

	start := time.Date(2026, 4, 22, 17, 4, 9, 0, time.UTC)
	keepalive := time.Date(2026, 4, 22, 17, 4, 12, 0, time.UTC)
	s := newTestService()
	writer := newRecordingWriter()
	s.uiWriter = writer
	s.uiWidth = func() int { return 80 }
	s.now = func() time.Time { return start }
	ticks := make(chan time.Time)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})

	go func() {
		s.runUILoop(ctx, ticks)
		close(done)
	}()

	writer.waitForWrites(t, 2)

	ticks <- start.Add(time.Second)
	writer.assertNoWrite(t)

	ticks <- keepalive
	writer.waitForWrites(t, 1)

	cancel()
	<-done

	active, events, waiting := s.snapshotUI()
	want := ui.EnterAlternateScreen() +
		ui.RewriteFrame(ui.RenderDashboardWithWidth(start, active, events, waiting, s.cfg.MaxParallelUploads, 80)) +
		ui.RefreshFrame(ui.RenderDashboardWithWidth(keepalive, active, events, waiting, s.cfg.MaxParallelUploads, 80)) +
		ui.LeaveAlternateScreen() +
		"\n"
	if got := writer.String(); got != want {
		t.Fatalf("runUILoop() output = %q, want %q", got, want)
	}
}

func TestRunUILoopUsesFullRefreshWhenWidthChanges(t *testing.T) {
	t.Parallel()

	start := time.Date(2026, 4, 22, 17, 4, 9, 0, time.UTC)
	now := time.Date(2026, 4, 22, 17, 4, 10, 0, time.UTC)
	s := newTestService()
	writer := newRecordingWriter()
	s.uiWriter = writer
	widths := []int{80, 100}
	s.uiWidth = func() int {
		width := widths[0]
		if len(widths) > 1 {
			widths = widths[1:]
		}
		return width
	}
	s.now = func() time.Time { return start }
	ticks := make(chan time.Time)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})

	go func() {
		s.runUILoop(ctx, ticks)
		close(done)
	}()

	writer.waitForWrites(t, 2)

	ticks <- now
	writer.waitForWrites(t, 1)

	cancel()
	<-done

	active, events, waiting := s.snapshotUI()
	want := ui.EnterAlternateScreen() +
		ui.RewriteFrame(ui.RenderDashboardWithWidth(start, active, events, waiting, s.cfg.MaxParallelUploads, 80)) +
		ui.RewriteFrame(ui.RenderDashboardWithWidth(now, active, events, waiting, s.cfg.MaxParallelUploads, 100)) +
		ui.LeaveAlternateScreen() +
		"\n"
	if got := writer.String(); got != want {
		t.Fatalf("runUILoop() output = %q, want %q", got, want)
	}
}

func TestRunUILoopRefreshesImmediatelyOnUIWake(t *testing.T) {
	t.Parallel()

	start := time.Date(2026, 4, 22, 17, 4, 9, 0, time.UTC)
	s := newTestService()
	writer := newRecordingWriter()
	s.uiWriter = writer
	s.uiWidth = func() int { return 80 }
	s.now = func() time.Time { return start }
	ticks := make(chan time.Time)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})

	go func() {
		s.runUILoop(ctx, ticks)
		close(done)
	}()

	writer.waitForWrites(t, 2)

	s.mu.Lock()
	s.recentEvents = append(s.recentEvents, recentEvent{
		at:      start,
		message: "wake refresh",
	})
	s.mu.Unlock()
	s.signalUIWake()
	writer.waitForWrites(t, 1)

	cancel()
	<-done

	active, events, waiting := s.snapshotUI()
	want := ui.EnterAlternateScreen() +
		ui.RewriteFrame(ui.RenderDashboardWithWidth(start, nil, nil, 0, s.cfg.MaxParallelUploads, 80)) +
		ui.RefreshFrame(ui.RenderDashboardWithWidth(start, active, events, waiting, s.cfg.MaxParallelUploads, 80)) +
		ui.LeaveAlternateScreen() +
		"\n"
	if got := writer.String(); got != want {
		t.Fatalf("runUILoop() output = %q, want %q", got, want)
	}
}

func TestRunStartsUILoopBeforeInitialScanCompletes(t *testing.T) {
	t.Parallel()

	sourceDir := t.TempDir()
	linkDir := filepath.Join(t.TempDir(), "links")
	cfg := &config.Config{
		MaxParallelUploads: 1,
		StableDuration:     45 * time.Second,
		Jobs: []config.JobConfig{
			{
				Name:      "movie",
				SourceDir: sourceDir,
				LinkDir:   linkDir,
			},
		},
		Extensions: []string{".mkv"},
	}

	s, err := NewService(cfg, log.New(io.Discard, "", 0))
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}

	writer := newRecordingWriter()
	s.uiWriter = writer
	s.uiWidth = func() int { return 80 }
	start := time.Date(2026, 4, 22, 17, 4, 10, 0, time.UTC)
	s.now = func() time.Time { return start }

	scanStarted := make(chan struct{})
	releaseScan := make(chan struct{})
	scanReturned := make(chan struct{})
	s.mkdirAll = func(string, os.FileMode) error {
		return nil
	}
	s.addWatches = func(string) error {
		return nil
	}
	s.scanExisting = func(context.Context, string, string, []string, time.Duration) (int, error) {
		close(scanStarted)
		<-releaseScan
		close(scanReturned)
		return 0, nil
	}
	s.scanLinkedFiles = func(string, []string) ([]string, error) { return nil, nil }

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	errCh := make(chan error, 1)
	go func() {
		errCh <- s.Run(ctx)
	}()

	select {
	case <-scanStarted:
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for startup scan to begin")
	}

	writer.waitForWrites(t, 2)

	select {
	case <-scanReturned:
		t.Fatal("startup scan returned before test released it")
	default:
	}

	want := ui.EnterAlternateScreen() +
		ui.RewriteFrame(ui.RenderDashboardWithWidth(start, nil, nil, 0, s.cfg.MaxParallelUploads, 80))
	if got := writer.String(); got != want {
		t.Fatalf("startup UI output before scan completes = %q, want %q", got, want)
	}

	close(releaseScan)
	cancel()

	if err := <-errCh; err != context.Canceled {
		t.Fatalf("Run() error = %v, want %v", err, context.Canceled)
	}
}

func TestRunMarksJobDirtyWhenStartupScanFindsExistingFiles(t *testing.T) {
	t.Parallel()

	sourceDir := t.TempDir()
	linkDir := filepath.Join(t.TempDir(), "links")
	cfg := &config.Config{
		MaxParallelUploads: 1,
		StableDuration:     90 * time.Second,
		Jobs: []config.JobConfig{
			{
				Name:      "movie",
				SourceDir: sourceDir,
				LinkDir:   linkDir,
			},
		},
		Extensions: []string{".mkv"},
	}

	s, err := NewService(cfg, log.New(io.Discard, "", 0))
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}

	s.uiWriter = io.Discard
	s.mkdirAll = func(string, os.FileMode) error {
		return nil
	}
	s.addWatches = func(string) error {
		return nil
	}
	s.scanLinkedFiles = func(string, []string) ([]string, error) {
		return []string{
			filepath.Join(linkDir, "a.mkv"),
			filepath.Join(linkDir, "b.mkv"),
		}, nil
	}

	ctx, cancel := context.WithCancel(context.Background())
	s.scanExisting = func(ctx context.Context, gotSourceDir, gotLinkDir string, gotExtensions []string, gotStableDuration time.Duration) (int, error) {
		if gotSourceDir != sourceDir {
			t.Fatalf("scanExisting sourceDir = %q, want %q", gotSourceDir, sourceDir)
		}
		if gotLinkDir != linkDir {
			t.Fatalf("scanExisting linkDir = %q, want %q", gotLinkDir, linkDir)
		}
		if len(gotExtensions) != 1 || gotExtensions[0] != ".mkv" {
			t.Fatalf("scanExisting extensions = %v, want [.mkv]", gotExtensions)
		}
		if gotStableDuration != cfg.StableDuration {
			t.Fatalf("scanExisting stableDuration = %v, want %v", gotStableDuration, cfg.StableDuration)
		}
		cancel()
		return 2, nil
	}

	err = s.Run(ctx)
	if err != context.Canceled {
		t.Fatalf("Run() error = %v, want %v", err, context.Canceled)
	}

	ready := s.scheduler.Ready()
	if len(ready) != 2 {
		t.Fatalf("Ready() = %v, want 2 linked-file tasks", ready)
	}
}

func TestStartupCatchUpQueuesEachLinkedFileIndividually(t *testing.T) {
	t.Parallel()

	sourceDir := "/source"
	linkDir := "/link"
	job := config.JobConfig{
		Name:         "MOVIE",
		SourceDir:    sourceDir,
		LinkDir:      linkDir,
		RcloneRemote: "remote:movies",
	}

	s := newTestService()
	s.cfg.Extensions = []string{".mkv"}
	s.cfg.StableDuration = time.Second
	s.configJobs = map[string]config.JobConfig{
		sourceDir: job,
	}
	s.mkdirAll = func(string, os.FileMode) error { return nil }
	s.addWatches = func(string) error { return nil }
	s.scanExisting = func(context.Context, string, string, []string, time.Duration) (int, error) { return 0, nil }
	s.scanLinkedFiles = func(string, []string) ([]string, error) {
		return []string{
			filepath.Join(linkDir, "movie", "a.mkv"),
			filepath.Join(linkDir, "movie", "b.mkv"),
		}, nil
	}

	if err := s.startupCatchUp(context.Background()); err != nil {
		t.Fatalf("startupCatchUp() error = %v", err)
	}

	ready := s.scheduler.Ready()
	if len(ready) != 2 {
		t.Fatalf("len(Ready()) = %d, want 2", len(ready))
	}
	want := map[string]struct{}{
		filepath.Join(linkDir, "movie", "a.mkv"): {},
		filepath.Join(linkDir, "movie", "b.mkv"): {},
	}
	for _, key := range ready {
		if _, ok := want[key]; !ok {
			t.Fatalf("Ready() contains unexpected key %q (all=%v)", key, ready)
		}
	}
}

func TestStartupCatchUpMarksJobDirtyWhenLinkDirHasPendingFiles(t *testing.T) {
	t.Parallel()

	job := config.JobConfig{Name: "MOVIE", SourceDir: "/source", LinkDir: "/link"}
	s := &Service{
		cfg:        &config.Config{StableDuration: time.Minute, Extensions: []string{".mkv"}},
		logger:     log.New(io.Discard, "", 0),
		scheduler:  queue.New(queue.Options{MaxParallel: 1}),
		configJobs: map[string]config.JobConfig{job.SourceDir: job},
		jobs:       map[string]*jobRuntime{},
		retryDue:   map[string]time.Time{},
		wakeCh:     make(chan struct{}, 1),
		now: func() time.Time {
			return time.Date(2026, 4, 23, 11, 0, 0, 0, time.UTC)
		},
	}
	s.mkdirAll = func(string, os.FileMode) error { return nil }
	s.addWatches = func(string) error { return nil }
	s.scanExisting = func(context.Context, string, string, []string, time.Duration) (int, error) { return 0, nil }
	s.scanLinkedFiles = func(string, []string) ([]string, error) {
		return []string{filepath.Join(job.LinkDir, "movie.mkv")}, nil
	}

	if err := s.startupCatchUp(context.Background()); err != nil {
		t.Fatalf("startupCatchUp() error = %v", err)
	}

	ready := s.scheduler.Ready()
	if len(ready) != 1 || ready[0] != filepath.Join(job.LinkDir, "movie.mkv") {
		t.Fatalf("Ready() = %v, want linked file key", ready)
	}
	if len(s.recentEvents) != 1 {
		t.Fatalf("len(recentEvents) = %d, want 1", len(s.recentEvents))
	}
	if got := s.recentEvents[0].message; got != "[MOVIE] 链接目录发现 1 个待上传文件，任务标记为待上传" {
		t.Fatalf("recentEvents[0].message = %q, want link dir startup event", got)
	}
}

func TestStartupCatchUpRecordsSchedulerEventWhenExistingFilesQueued(t *testing.T) {
	t.Parallel()

	job := config.JobConfig{Name: "MOVIE", SourceDir: "/source", LinkDir: "/link"}
	s := &Service{
		cfg:        &config.Config{StableDuration: time.Minute},
		logger:     log.New(io.Discard, "", 0),
		scheduler:  queue.New(queue.Options{MaxParallel: 1}),
		configJobs: map[string]config.JobConfig{job.SourceDir: job},
		jobs:       map[string]*jobRuntime{},
		now: func() time.Time {
			return time.Date(2026, 4, 23, 10, 0, 0, 0, time.UTC)
		},
	}
	s.mkdirAll = func(string, os.FileMode) error { return nil }
	s.addWatches = func(string) error { return nil }
	s.scanExisting = func(context.Context, string, string, []string, time.Duration) (int, error) { return 3, nil }
	s.scanLinkedFiles = func(string, []string) ([]string, error) {
		return []string{filepath.Join(job.LinkDir, "movie.mkv")}, nil
	}

	if err := s.startupCatchUp(context.Background()); err != nil {
		t.Fatalf("startupCatchUp() error = %v", err)
	}
	if len(s.recentEvents) != 1 {
		t.Fatalf("len(recentEvents) = %d, want 1", len(s.recentEvents))
	}
	if got := s.recentEvents[0].message; got != "[MOVIE] 启动扫描发现 3 个文件，任务标记为待上传" {
		t.Fatalf("recentEvents[0].message = %q, want startup queue event", got)
	}
}

func TestProcessFileRecordsQueueEventOnceForIdleJob(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	sourceDir := filepath.Join(root, "source")
	linkDir := filepath.Join(root, "link")
	path := filepath.Join(sourceDir, "movie.mkv")
	if err := os.MkdirAll(sourceDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("data"), 0o644); err != nil {
		t.Fatal(err)
	}

	job := config.JobConfig{Name: "MOVIE", SourceDir: sourceDir, LinkDir: linkDir}
	s := &Service{
		cfg:       &config.Config{StableDuration: time.Millisecond, PollInterval: time.Millisecond, Extensions: []string{".mkv"}},
		logger:    log.New(io.Discard, "", 0),
		scheduler: queue.New(queue.Options{MaxParallel: 1}),
		jobs:      map[string]*jobRuntime{},
		now: func() time.Time {
			return time.Date(2026, 4, 23, 10, 0, 1, 0, time.UTC)
		},
	}

	s.processFile(context.Background(), job, path)
	s.processFile(context.Background(), job, path)

	if len(s.recentEvents) != 1 {
		t.Fatalf("len(recentEvents) = %d, want 1", len(s.recentEvents))
	}
	if got := s.recentEvents[0].message; got != "[MOVIE] movie.mkv｜检测到新文件，任务标记为待上传" {
		t.Fatalf("recentEvents[0].message = %q, want runtime queue event", got)
	}
}

func TestProcessFileQueuesSiblingFilesAsIndependentTasks(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	sourceDir := filepath.Join(root, "source")
	linkDir := filepath.Join(root, "link")
	fileA := filepath.Join(sourceDir, "movie", "a.mkv")
	fileB := filepath.Join(sourceDir, "movie", "b.mkv")
	if err := os.MkdirAll(filepath.Dir(fileA), 0o755); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{fileA, fileB} {
		if err := os.WriteFile(path, []byte("data"), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	job := config.JobConfig{
		Name:         "MOVIE",
		SourceDir:    sourceDir,
		LinkDir:      linkDir,
		RcloneRemote: "remote:movies",
	}
	s := newTestService()
	s.cfg.Extensions = []string{".mkv"}
	s.cfg.StableDuration = time.Millisecond
	s.cfg.PollInterval = time.Millisecond
	s.startUpload = func(ctx context.Context, job *jobRuntime) {
		go s.runUpload(ctx, job)
	}
	s.configJobs = map[string]config.JobConfig{
		sourceDir: job,
	}

	s.processFile(context.Background(), job, fileA)
	s.processFile(context.Background(), job, fileB)

	ready := s.scheduler.Ready()
	if len(ready) != 2 {
		t.Fatalf("len(Ready()) = %d, want 2", len(ready))
	}
	want := map[string]struct{}{
		filepath.Join(linkDir, "movie", "a.mkv"): {},
		filepath.Join(linkDir, "movie", "b.mkv"): {},
	}
	for _, key := range ready {
		if _, ok := want[key]; !ok {
			t.Fatalf("Ready() contains unexpected key %q (all=%v)", key, ready)
		}
	}
}

func TestProcessFileLimitsConcurrentProcessing(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	sourceDir := filepath.Join(root, "source")
	linkDir := filepath.Join(root, "link")
	fileA := filepath.Join(sourceDir, "a.mkv")
	fileB := filepath.Join(sourceDir, "b.mkv")
	if err := os.MkdirAll(sourceDir, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{fileA, fileB} {
		if err := os.WriteFile(path, []byte("data"), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	job := config.JobConfig{Name: "MOVIE", SourceDir: sourceDir, LinkDir: linkDir, RcloneRemote: "remote:movies"}
	s := newTestService()
	s.cfg.Extensions = []string{".mkv"}
	s.cfg.StableDuration = time.Millisecond
	s.cfg.PollInterval = time.Millisecond
	s.processSem = make(chan struct{}, 1)

	entered := make(chan string, 2)
	releaseFirst := make(chan struct{})
	s.beforeProcessFile = func(path string) {
		entered <- path
		if path == fileA {
			<-releaseFirst
		}
	}

	doneA := make(chan struct{})
	doneB := make(chan struct{})
	go func() {
		defer close(doneA)
		s.processFile(context.Background(), job, fileA)
	}()
	select {
	case got := <-entered:
		if got != fileA {
			t.Fatalf("first entered path = %q, want %q", got, fileA)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for first processFile to enter")
	}

	go func() {
		defer close(doneB)
		s.processFile(context.Background(), job, fileB)
	}()
	select {
	case got := <-entered:
		t.Fatalf("second processFile entered while semaphore held: %q", got)
	case <-time.After(50 * time.Millisecond):
	}

	close(releaseFirst)
	select {
	case got := <-entered:
		if got != fileB {
			t.Fatalf("second entered path = %q, want %q", got, fileB)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for second processFile to enter")
	}
	<-doneA
	<-doneB
}

func TestProcessFileDropsDuplicatePathBeforeWaitingForProcessingSlot(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	sourceDir := filepath.Join(root, "source")
	linkDir := filepath.Join(root, "link")
	path := filepath.Join(sourceDir, "movie.mkv")
	if err := os.MkdirAll(sourceDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("data"), 0o644); err != nil {
		t.Fatal(err)
	}

	job := config.JobConfig{Name: "MOVIE", SourceDir: sourceDir, LinkDir: linkDir, RcloneRemote: "remote:movies"}
	s := newTestService()
	s.cfg.Extensions = []string{".mkv"}
	s.cfg.StableDuration = time.Millisecond
	s.cfg.PollInterval = time.Millisecond
	s.processSem = make(chan struct{}, 1)

	entered := make(chan struct{}, 2)
	releaseFirst := make(chan struct{})
	s.beforeProcessFile = func(string) {
		entered <- struct{}{}
		<-releaseFirst
	}

	doneFirst := make(chan struct{})
	go func() {
		defer close(doneFirst)
		s.processFile(context.Background(), job, path)
	}()
	select {
	case <-entered:
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for first processFile to enter")
	}

	doneDuplicate := make(chan struct{})
	go func() {
		defer close(doneDuplicate)
		s.processFile(context.Background(), job, path)
	}()

	select {
	case <-doneDuplicate:
	case <-time.After(time.Second):
		close(releaseFirst)
		<-doneFirst
		t.Fatal("duplicate same-path processFile waited for semaphore instead of returning")
	}

	close(releaseFirst)
	<-doneFirst

	select {
	case <-entered:
		t.Fatal("duplicate same-path processFile reached processing hook")
	default:
	}
}

func TestProcessFileRecordsQueueEventWhenDispatchStartsImmediately(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	sourceDir := filepath.Join(root, "source")
	linkDir := filepath.Join(root, "link")
	path := filepath.Join(sourceDir, "movie.mkv")
	if err := os.MkdirAll(sourceDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("data"), 0o644); err != nil {
		t.Fatal(err)
	}

	job := config.JobConfig{Name: "MOVIE", SourceDir: sourceDir, LinkDir: linkDir}
	s := &Service{
		cfg:       &config.Config{StableDuration: time.Millisecond, PollInterval: time.Millisecond, Extensions: []string{".mkv"}},
		logger:    log.New(io.Discard, "", 0),
		scheduler: queue.New(queue.Options{MaxParallel: 1}),
		jobs:      map[string]*jobRuntime{},
		retryDue:  map[string]time.Time{},
		wakeCh:    make(chan struct{}, 1),
		now: func() time.Time {
			return time.Date(2026, 4, 23, 10, 0, 1, 0, time.UTC)
		},
	}
	s.afterMarkDirty = func(key string) {
		if !s.scheduler.TryStart(key) {
			t.Fatal("TryStart() = false, want true")
		}
	}

	s.processFile(context.Background(), job, path)

	if len(s.recentEvents) != 1 {
		t.Fatalf("len(recentEvents) = %d, want 1", len(s.recentEvents))
	}
	if got := s.recentEvents[0].message; got != "[MOVIE] movie.mkv｜检测到新文件，任务标记为待上传" {
		t.Fatalf("recentEvents[0].message = %q, want runtime queue event even if dispatch wins race", got)
	}
}

func TestProcessFileClearsRetryWaitForSamePathUpdate(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	sourceDir := filepath.Join(root, "source")
	linkDir := filepath.Join(root, "link")
	path := filepath.Join(sourceDir, "movie.mkv")
	if err := os.MkdirAll(sourceDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("data"), 0o644); err != nil {
		t.Fatal(err)
	}

	job := config.JobConfig{Name: "MOVIE", SourceDir: sourceDir, LinkDir: linkDir, RcloneRemote: "remote:movie"}
	linkPath := filepath.Join(linkDir, "movie.mkv")
	s := &Service{
		cfg:       &config.Config{StableDuration: time.Millisecond, PollInterval: time.Millisecond, Extensions: []string{".mkv"}},
		logger:    log.New(io.Discard, "", 0),
		scheduler: queue.New(queue.Options{MaxParallel: 1}),
		jobs: map[string]*jobRuntime{
			linkPath: {
				cfg:        job,
				key:        linkPath,
				sourcePath: path,
				linkPath:   linkPath,
				remoteDir:  "remote:movie/",
			},
		},
		retryDue: map[string]time.Time{},
		wakeCh:   make(chan struct{}, 1),
		now: func() time.Time {
			return time.Date(2026, 4, 23, 10, 0, 8, 0, time.UTC)
		},
	}

	s.scheduler.MarkDirty(linkPath)
	if !s.scheduler.TryStart(linkPath) {
		t.Fatal("TryStart() = false, want true")
	}
	s.scheduler.FinishFailed(linkPath)
	s.retryDue[linkPath] = s.currentTime().Add(time.Minute)

	s.processFile(context.Background(), job, path)

	if _, ok := s.retryDue[linkPath]; ok {
		t.Fatalf("retryDue[%q] exists, want same-path update to clear retry wait", linkPath)
	}
	if ready := s.scheduler.Ready(); len(ready) != 1 || ready[0] != linkPath {
		t.Fatalf("Ready() = %v, want [%q] after retry wait is cleared", ready, linkPath)
	}
	if _, ok := s.failureCounts[linkPath]; ok {
		t.Fatalf("failureCounts[%q] exists, want retry-wait replacement to clear failures", linkPath)
	}
}

func TestRegisterTaskLockedInactiveReregistrationKeepsFailureCount(t *testing.T) {
	t.Parallel()

	cfgJob := config.JobConfig{Name: "MOVIE", SourceDir: "/source", LinkDir: "/link", RcloneRemote: "remote:movie"}
	linkPath := "/link/movie.mkv"
	s := newTestService()
	s.jobs[linkPath] = &jobRuntime{
		cfg:        cfgJob,
		key:        linkPath,
		sourcePath: "/source/movie.mkv",
		linkPath:   linkPath,
		remoteDir:  "remote:movie/",
		active:     false,
	}
	s.failureCounts[linkPath] = 2

	task, state, err := s.registerTaskLocked(cfgJob, "/source/movie.mkv", linkPath)
	if err != nil {
		t.Fatalf("registerTaskLocked() error = %v", err)
	}
	if task == nil {
		t.Fatal("registerTaskLocked() task = nil, want task")
	}
	if state.replacedRunning {
		t.Fatal("state.replacedRunning = true, want false")
	}
	if state.clearedRetryWait {
		t.Fatal("state.clearedRetryWait = true, want false")
	}
	if got := s.failureCounts[linkPath]; got != 2 {
		t.Fatalf("failureCounts[%q] = %d, want 2 after ordinary re-registration", linkPath, got)
	}
}

func TestProcessFileUsesSupersedeEventForActiveFileTask(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	sourceDir := filepath.Join(root, "source")
	linkDir := filepath.Join(root, "link")
	path := filepath.Join(sourceDir, "movie.mkv")
	if err := os.MkdirAll(sourceDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("data"), 0o644); err != nil {
		t.Fatal(err)
	}

	job := config.JobConfig{Name: "MOVIE", SourceDir: sourceDir, LinkDir: linkDir}
	linkPath := filepath.Join(linkDir, "movie.mkv")
	s := &Service{
		cfg:       &config.Config{StableDuration: time.Millisecond, PollInterval: time.Millisecond, Extensions: []string{".mkv"}},
		logger:    log.New(io.Discard, "", 0),
		scheduler: queue.New(queue.Options{MaxParallel: 1}),
		jobs: map[string]*jobRuntime{
			linkPath: {cfg: job, key: linkPath, linkPath: linkPath, sourcePath: path, active: true},
		},
		now: func() time.Time {
			return time.Date(2026, 4, 23, 10, 0, 2, 0, time.UTC)
		},
	}

	s.processFile(context.Background(), job, path)

	if len(s.recentEvents) != 1 {
		t.Fatalf("len(recentEvents) = %d, want 1", len(s.recentEvents))
	}
	if got := s.recentEvents[0].message; got != "[MOVIE] movie.mkv｜检测到同路径文件更新，已取消旧上传并重新排队" {
		t.Fatalf("recentEvents[0].message = %q, want supersede event", got)
	}
}

func TestStartReadyUploadsRecordsDispatchStartEvent(t *testing.T) {
	t.Parallel()

	job := config.JobConfig{Name: "MOVIE", SourceDir: "/source", LinkDir: "/link"}
	linkPath := "/link/movie.mkv"
	s := newTestService()
	s.jobs[linkPath] = &jobRuntime{
		cfg:        job,
		key:        linkPath,
		sourcePath: "/source/movie.mkv",
		linkPath:   linkPath,
		remoteDir:  "remote:movie/",
	}
	s.now = func() time.Time {
		return time.Date(2026, 4, 23, 10, 0, 3, 0, time.UTC)
	}
	s.startUpload = func(context.Context, *jobRuntime) {}
	s.scheduler.MarkDirty(linkPath)

	s.startReadyUploads(context.Background())

	if len(s.recentEvents) != 1 {
		t.Fatalf("len(recentEvents) = %d, want 1", len(s.recentEvents))
	}
	if got := s.recentEvents[0].message; got != "[MOVIE] movie.mkv｜调度开始上传" {
		t.Fatalf("recentEvents[0].message = %q, want dispatch start event", got)
	}
}

func TestStartReadyUploadsStartsMultipleSiblingFilesUpToLimit(t *testing.T) {
	t.Parallel()

	job := config.JobConfig{
		Name:         "MOVIE",
		SourceDir:    "/source",
		LinkDir:      "/link",
		RcloneRemote: "remote:movies",
	}

	s := newTestService()
	s.scheduler = queue.New(queue.Options{MaxParallel: 2})
	s.jobs = map[string]*jobRuntime{
		"/link/a.mkv": {cfg: job, key: "/link/a.mkv", sourcePath: "/source/a.mkv", linkPath: "/link/a.mkv", remoteDir: "remote:movies/"},
		"/link/b.mkv": {cfg: job, key: "/link/b.mkv", sourcePath: "/source/b.mkv", linkPath: "/link/b.mkv", remoteDir: "remote:movies/"},
		"/link/c.mkv": {cfg: job, key: "/link/c.mkv", sourcePath: "/source/c.mkv", linkPath: "/link/c.mkv", remoteDir: "remote:movies/"},
	}

	started := make(map[string]struct{})
	s.startUpload = func(_ context.Context, task *jobRuntime) {
		started[task.linkPath] = struct{}{}
	}
	for _, key := range []string{"/link/a.mkv", "/link/b.mkv", "/link/c.mkv"} {
		s.scheduler.MarkDirty(key)
	}

	s.startReadyUploads(context.Background())

	if len(started) != 2 {
		t.Fatalf("len(started) = %d, want 2", len(started))
	}
}

func TestStartReadyUploadsForgetsQueuedKeyWithoutRuntimeTask(t *testing.T) {
	t.Parallel()

	s := newTestService()
	linkPath := "/link/orphan.mkv"
	s.scheduler.MarkDirty(linkPath)

	s.startReadyUploads(context.Background())

	if s.scheduler.Forget(linkPath) {
		t.Fatalf("Forget(%q) = true, want false after service orphan cleanup", linkPath)
	}
}

func TestRegisterTaskRefreshKeepsNewFileTaskWhenOldUploadCompletes(t *testing.T) {
	t.Parallel()

	cfgJob := config.JobConfig{
		Name:         "MOVIE",
		SourceDir:    "/source",
		LinkDir:      "/link",
		RcloneRemote: "remote:movie",
	}
	linkPath := "/link/movie.mkv"
	oldTask := &jobRuntime{
		cfg:        cfgJob,
		key:        linkPath,
		sourcePath: "/source/movie-old.mkv",
		linkPath:   linkPath,
		remoteDir:  "remote:movie/",
		active:     true,
	}

	s := newTestService()
	s.jobs[linkPath] = oldTask

	newTask, err := s.registerTask(cfgJob, "/source/movie.mkv", linkPath)
	if err != nil {
		t.Fatalf("registerTask() error = %v", err)
	}
	oldTask.active = false

	s.removeTaskIfCompleted(oldTask)

	current := s.taskForKey(linkPath)
	if current == nil {
		t.Fatal("taskForKey() = nil, want refreshed task retained")
	}
	if current != newTask {
		t.Fatalf("taskForKey() = %#v, want refreshed task %#v", current, newTask)
	}
}

func TestRegisterTaskRefreshesCompletedSamePathTaskBeforeRequeue(t *testing.T) {
	t.Parallel()

	cfgJob := config.JobConfig{
		Name:         "MOVIE",
		SourceDir:    "/source",
		LinkDir:      "/link",
		RcloneRemote: "remote:movie",
	}
	linkPath := "/link/movie.mkv"
	oldTask := &jobRuntime{
		cfg:        cfgJob,
		key:        linkPath,
		sourcePath: "/source/movie-old.mkv",
		linkPath:   linkPath,
		remoteDir:  "remote:movie/",
		summary:    "上传完成",
	}

	s := newTestService()
	s.jobs[linkPath] = oldTask

	replacementTask, err := s.registerTask(cfgJob, "/source/movie-new.mkv", linkPath)
	if err != nil {
		t.Fatalf("registerTask() error = %v", err)
	}

	s.removeTaskIfCompleted(oldTask)

	current := s.taskForKey(linkPath)
	if current == nil {
		t.Fatal("taskForKey() = nil, want replacement task retained before requeue")
	}
	if current != replacementTask {
		t.Fatalf("taskForKey() = %#v, want replacement task %#v", current, replacementTask)
	}
	if current != oldTask {
		t.Fatalf("taskForKey() = %#v, want completed task refreshed in place %#v", current, oldTask)
	}

	started := make(chan *jobRuntime, 1)
	s.startUpload = func(_ context.Context, job *jobRuntime) {
		started <- job
	}

	s.scheduler.MarkDirty(replacementTask.key)
	s.startReadyUploads(context.Background())

	select {
	case got := <-started:
		if got != replacementTask {
			t.Fatalf("startUpload() job = %#v, want replacement task %#v", got, replacementTask)
		}
	case <-time.After(time.Second):
		t.Fatal("startUpload() not called, want replacement task to remain uploadable")
	}

	if got, want := replacementTask.sourcePath, "/source/movie-new.mkv"; got != want {
		t.Fatalf("replacementTask.sourcePath = %q, want %q", got, want)
	}
}

func TestRunUploadRecordsCompletionAndFailureEvents(t *testing.T) {
	t.Parallel()

	makeService := func(at time.Time) (*Service, *jobRuntime) {
		s := newTestService()
		s.cfg.RetryInterval = time.Minute
		s.now = func() time.Time { return at }
		s.copyJob = func(context.Context, *jobRuntime) error { return nil }
		s.cleanupLinkedFile = func(string, string) error { return nil }
		linkPath := "/link/movie.mkv"
		job := &jobRuntime{
			cfg: config.JobConfig{
				Name:         "MOVIE",
				SourceDir:    "/source",
				LinkDir:      "/link",
				RcloneRemote: "remote:movie",
			},
			key:        linkPath,
			sourcePath: "/source/movie.mkv",
			linkPath:   linkPath,
			remoteDir:  "remote:movie/",
			active:     true,
		}
		s.jobs[job.key] = job
		s.scheduler.MarkDirty(job.key)
		if !s.scheduler.TryStart(job.key) {
			t.Fatal("TryStart() = false, want true")
		}
		return s, job
	}

	t.Run("success clears job", func(t *testing.T) {
		s, job := makeService(time.Date(2026, 4, 23, 10, 0, 4, 0, time.UTC))

		s.runUpload(context.Background(), job)

		if len(s.recentEvents) != 1 {
			t.Fatalf("len(recentEvents) = %d, want 1", len(s.recentEvents))
		}
		if got := s.recentEvents[0].message; got != "[MOVIE] movie.mkv｜上传完成，任务清空" {
			t.Fatalf("recentEvents[0].message = %q, want success event", got)
		}
		if _, ok := s.jobs[job.key]; ok {
			t.Fatalf("runtime task %q still exists after successful upload", job.key)
		}
		if _, ok := s.retryDue[job.key]; ok {
			t.Fatalf("retryDue still contains completed task key %q", job.key)
		}
		if s.scheduler.Forget(job.key) {
			t.Fatalf("Forget(%q) = true, want false after successful cleanup", job.key)
		}
	})

	t.Run("copy failure enters retry", func(t *testing.T) {
		s, job := makeService(time.Date(2026, 4, 23, 10, 0, 6, 0, time.UTC))
		s.copyJob = func(context.Context, *jobRuntime) error { return errors.New("copy failed") }

		s.runUpload(context.Background(), job)

		if len(s.recentEvents) != 1 {
			t.Fatalf("len(recentEvents) = %d, want 1", len(s.recentEvents))
		}
		if got := s.recentEvents[0].message; got != "[MOVIE] movie.mkv｜上传失败，进入重试等待" {
			t.Fatalf("recentEvents[0].message = %q, want failure event", got)
		}
		if _, ok := s.retryDue[job.linkPath]; !ok {
			t.Fatalf("retryDue missing file key %q", job.linkPath)
		}
	})

	t.Run("cleanup failure enters retry", func(t *testing.T) {
		s, job := makeService(time.Date(2026, 4, 23, 10, 0, 7, 0, time.UTC))
		root := t.TempDir()
		linkDir := filepath.Join(root, "link")
		linkPath := filepath.Join(linkDir, "movie.mkv")
		if err := os.MkdirAll(linkDir, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(linkPath, []byte("data"), 0o644); err != nil {
			t.Fatal(err)
		}
		job.cfg.LinkDir = linkDir
		job.key = linkPath
		job.linkPath = linkPath
		s.jobs = map[string]*jobRuntime{job.key: job}
		s.scheduler = queue.New(queue.Options{MaxParallel: 5})
		s.scheduler.MarkDirty(job.key)
		if !s.scheduler.TryStart(job.key) {
			t.Fatal("TryStart() = false, want true")
		}
		s.cleanupLinkedFile = func(string, string) error { return errors.New("cleanup failed") }

		s.runUpload(context.Background(), job)

		if len(s.recentEvents) != 1 {
			t.Fatalf("len(recentEvents) = %d, want 1", len(s.recentEvents))
		}
		if got := s.recentEvents[0].message; got != "[MOVIE] movie.mkv｜上传失败，进入重试等待" {
			t.Fatalf("recentEvents[0].message = %q, want cleanup failure retry event", got)
		}
		if _, ok := s.retryDue[job.linkPath]; !ok {
			t.Fatalf("retryDue missing file key %q", job.linkPath)
		}
	})
}

func TestRunUploadTreatsPostDeleteCleanupErrorAsSuccess(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	linkDir := filepath.Join(root, "linked files")
	linkPath := filepath.Join(linkDir, "season 1", "episode 1.mkv")
	if err := os.MkdirAll(filepath.Dir(linkPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(linkPath, []byte("data"), 0o644); err != nil {
		t.Fatal(err)
	}

	s := newTestService()
	s.cfg.RetryInterval = time.Minute
	s.now = func() time.Time { return time.Date(2026, 4, 23, 10, 0, 8, 0, time.UTC) }
	s.copyJob = func(context.Context, *jobRuntime) error { return nil }
	s.cleanupLinkedFile = func(string, string) error {
		if err := os.Remove(linkPath); err != nil {
			return err
		}
		return errors.New("prune failed")
	}

	job := &jobRuntime{
		cfg: config.JobConfig{
			Name:         "MOVIE",
			SourceDir:    filepath.Join(root, "source"),
			LinkDir:      linkDir,
			RcloneRemote: "remote:movie",
		},
		key:        linkPath,
		sourcePath: filepath.Join(root, "source", "season 1", "episode 1.mkv"),
		linkPath:   linkPath,
		remoteDir:  "remote:movie/season 1/",
		active:     true,
	}
	s.jobs[job.key] = job
	s.scheduler.MarkDirty(job.key)
	if !s.scheduler.TryStart(job.key) {
		t.Fatal("TryStart() = false, want true")
	}

	s.runUpload(context.Background(), job)

	if len(s.recentEvents) != 1 {
		t.Fatalf("len(recentEvents) = %d, want 1", len(s.recentEvents))
	}
	if got := s.recentEvents[0].message; got != "[MOVIE] episode 1.mkv｜上传完成，任务清空" {
		t.Fatalf("recentEvents[0].message = %q, want success event after post-delete cleanup error", got)
	}
	if _, ok := s.retryDue[job.key]; ok {
		t.Fatalf("retryDue still contains completed task key %q", job.key)
	}
	if _, ok := s.jobs[job.key]; ok {
		t.Fatalf("runtime task %q still exists after successful upload", job.key)
	}
	if _, err := os.Stat(linkPath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("linked file still exists or wrong error = %v, want not exist", err)
	}
}

func TestRunUploadSuccessRemovesOnlyUploadedLinkedFile(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	linkDir := filepath.Join(root, "link")
	uploadedLink := filepath.Join(linkDir, "season1", "episode1.mkv")
	siblingLink := filepath.Join(linkDir, "season1", "episode2.mkv")
	siblingDir := filepath.Join(linkDir, "season2")
	if err := os.MkdirAll(filepath.Dir(uploadedLink), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(siblingDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(uploadedLink, []byte("ep1"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(siblingLink, []byte("ep2"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(siblingDir, "readme.txt"), []byte("keep"), 0o644); err != nil {
		t.Fatal(err)
	}

	s := newTestService()
	s.copyJob = func(context.Context, *jobRuntime) error { return nil }
	s.cleanupLinkedFile = watcher.CleanupLinkedFile
	job := &jobRuntime{
		cfg: config.JobConfig{
			Name:         "MOVIE",
			SourceDir:    filepath.Join(root, "source"),
			LinkDir:      linkDir,
			RcloneRemote: "remote:movie",
		},
		key:        uploadedLink,
		sourcePath: filepath.Join(root, "source", "season1", "episode1.mkv"),
		linkPath:   uploadedLink,
		remoteDir:  "remote:movie/season1/",
		active:     true,
	}
	s.jobs[job.key] = job
	s.scheduler.MarkDirty(job.key)
	if !s.scheduler.TryStart(job.key) {
		t.Fatal("TryStart() = false, want true")
	}

	s.runUpload(context.Background(), job)

	if _, err := os.Stat(uploadedLink); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("uploaded link still exists or wrong error = %v, want not exist", err)
	}
	if _, err := os.Stat(siblingLink); err != nil {
		t.Fatalf("sibling link removed unexpectedly: %v", err)
	}
	if _, err := os.Stat(siblingDir); err != nil {
		t.Fatalf("sibling directory removed unexpectedly: %v", err)
	}
}

func TestProcessFileInPlaceSamePathUpdateCancelsActiveUploadWithoutCleanupOrRetry(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	sourceDir := filepath.Join(root, "source")
	linkDir := filepath.Join(root, "link")
	sourcePath := filepath.Join(sourceDir, "show", "episode-1.mkv")
	if err := os.MkdirAll(filepath.Dir(sourcePath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(sourcePath, []byte("old-bytes"), 0o644); err != nil {
		t.Fatal(err)
	}

	linkPath, err := watcher.LinkFile(sourceDir, linkDir, sourcePath)
	if err != nil {
		t.Fatalf("LinkFile() error = %v", err)
	}

	cfgJob := config.JobConfig{
		Name:         "MOVIE",
		SourceDir:    sourceDir,
		LinkDir:      linkDir,
		RcloneRemote: "remote:movie",
	}

	s := newTestService()
	s.cfg.Extensions = []string{".mkv"}
	s.cfg.StableDuration = time.Millisecond
	s.cfg.PollInterval = time.Millisecond
	s.startUpload = func(ctx context.Context, job *jobRuntime) {
		go s.runUpload(ctx, job)
	}

	started := make(chan struct{}, 1)
	stopped := make(chan error, 1)
	s.copyJob = func(ctx context.Context, job *jobRuntime) error {
		gotBytes, err := os.ReadFile(job.linkPath)
		if err != nil {
			return err
		}
		if string(gotBytes) != "old-bytes" {
			t.Fatalf("upload read %q, want %q", string(gotBytes), "old-bytes")
		}
		select {
		case started <- struct{}{}:
		default:
		}
		<-ctx.Done()
		stopped <- ctx.Err()
		return ctx.Err()
	}

	cleanupCalls := make(chan struct{}, 1)
	s.cleanupLinkedFile = func(string, string) error {
		cleanupCalls <- struct{}{}
		return nil
	}

	oldTask, err := s.registerTask(cfgJob, sourcePath, linkPath)
	if err != nil {
		t.Fatalf("registerTask() error = %v", err)
	}
	s.scheduler.MarkDirty(oldTask.key)
	s.startReadyUploads(context.Background())

	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("upload did not start")
	}

	if err := os.WriteFile(sourcePath, []byte("new-bytes"), 0o644); err != nil {
		t.Fatalf("WriteFile(%q) error = %v", sourcePath, err)
	}

	s.processFile(context.Background(), cfgJob, sourcePath)

	select {
	case err := <-stopped:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("stopped upload error = %v, want context canceled", err)
		}
	case <-time.After(time.Second):
		t.Fatal("active upload was not canceled by same-path update")
	}

	select {
	case <-cleanupCalls:
		t.Fatal("cleanupLinkedFile() called for superseded upload, want no cleanup")
	case <-time.After(100 * time.Millisecond):
	}

	if _, ok := s.retryDue[linkPath]; ok {
		t.Fatalf("retryDue[%q] exists, want superseded upload to skip retry wait", linkPath)
	}
	waitForReadyJob(t, s, linkPath)

	current := s.taskForKey(linkPath)
	if current == nil {
		t.Fatal("taskForKey() = nil, want latest task retained")
	}
	if current == oldTask {
		t.Fatalf("taskForKey() = %#v, want fresh latest task", current)
	}

	gotBytes, err := os.ReadFile(linkPath)
	if err != nil {
		t.Fatalf("ReadFile(%q) error = %v", linkPath, err)
	}
	if string(gotBytes) != "new-bytes" {
		t.Fatalf("linked file content = %q, want %q", string(gotBytes), "new-bytes")
	}
}

func TestProcessFileNewInodeSamePathReplacementCancelsActiveUploadAndRequeuesLatest(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	sourceDir := filepath.Join(root, "source")
	linkDir := filepath.Join(root, "link")
	sourcePath := filepath.Join(sourceDir, "movie.mkv")
	if err := os.MkdirAll(sourceDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(sourcePath, []byte("old-bytes"), 0o644); err != nil {
		t.Fatal(err)
	}

	linkPath, err := watcher.LinkFile(sourceDir, linkDir, sourcePath)
	if err != nil {
		t.Fatalf("LinkFile() error = %v", err)
	}

	cfgJob := config.JobConfig{
		Name:         "MOVIE",
		SourceDir:    sourceDir,
		LinkDir:      linkDir,
		RcloneRemote: "remote:movie",
	}

	s := newTestService()
	s.cfg.Extensions = []string{".mkv"}
	s.cfg.StableDuration = time.Millisecond
	s.cfg.PollInterval = time.Millisecond
	s.startUpload = func(ctx context.Context, job *jobRuntime) {
		go s.runUpload(ctx, job)
	}

	started := make(chan struct{}, 1)
	stopped := make(chan error, 1)
	s.copyJob = func(ctx context.Context, job *jobRuntime) error {
		select {
		case started <- struct{}{}:
		default:
		}
		<-ctx.Done()
		stopped <- ctx.Err()
		return ctx.Err()
	}

	oldTask, err := s.registerTask(cfgJob, sourcePath, linkPath)
	if err != nil {
		t.Fatalf("registerTask() error = %v", err)
	}
	s.scheduler.MarkDirty(oldTask.key)
	s.startReadyUploads(context.Background())

	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("upload did not start")
	}

	if err := os.Remove(sourcePath); err != nil {
		t.Fatalf("Remove(%q) error = %v", sourcePath, err)
	}
	if err := os.WriteFile(sourcePath, []byte("new-bytes"), 0o644); err != nil {
		t.Fatalf("WriteFile(%q) error = %v", sourcePath, err)
	}

	s.processFile(context.Background(), cfgJob, sourcePath)

	select {
	case err := <-stopped:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("stopped upload error = %v, want context canceled", err)
		}
	case <-time.After(time.Second):
		t.Fatal("active upload was not canceled by same-path replacement")
	}

	waitForReadyJob(t, s, linkPath)

	gotBytes, err := os.ReadFile(linkPath)
	if err != nil {
		t.Fatalf("ReadFile(%q) error = %v", linkPath, err)
	}
	if string(gotBytes) != "new-bytes" {
		t.Fatalf("linked file content = %q, want %q", string(gotBytes), "new-bytes")
	}

	sourceInfo, err := os.Stat(sourcePath)
	if err != nil {
		t.Fatalf("Stat(%q) error = %v", sourcePath, err)
	}
	linkInfo, err := os.Stat(linkPath)
	if err != nil {
		t.Fatalf("Stat(%q) error = %v", linkPath, err)
	}
	if !os.SameFile(sourceInfo, linkInfo) {
		t.Fatal("replacement link is not a hard link to replacement source")
	}
}

func TestRunUploadFailurePreservesLinkedFileAndRetriesOnlyFileKey(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	linkDir := filepath.Join(root, "link")
	uploadedLink := filepath.Join(linkDir, "season1", "episode1.mkv")
	siblingLink := filepath.Join(linkDir, "season1", "episode2.mkv")
	if err := os.MkdirAll(filepath.Dir(uploadedLink), 0o755); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{uploadedLink, siblingLink} {
		if err := os.WriteFile(path, []byte("data"), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	s := newTestService()
	s.cfg.RetryInterval = time.Minute
	s.now = func() time.Time { return time.Date(2026, 4, 23, 11, 0, 0, 0, time.UTC) }
	s.copyJob = func(context.Context, *jobRuntime) error { return errors.New("copy failed") }
	s.cleanupLinkedFile = watcher.CleanupLinkedFile
	job := &jobRuntime{
		cfg: config.JobConfig{
			Name:         "MOVIE",
			SourceDir:    filepath.Join(root, "source"),
			LinkDir:      linkDir,
			RcloneRemote: "remote:movie",
		},
		key:        uploadedLink,
		sourcePath: filepath.Join(root, "source", "season1", "episode1.mkv"),
		linkPath:   uploadedLink,
		remoteDir:  "remote:movie/season1/",
		active:     true,
	}
	s.jobs[job.key] = job
	s.scheduler.MarkDirty(job.key)
	if !s.scheduler.TryStart(job.key) {
		t.Fatal("TryStart() = false, want true")
	}

	s.runUpload(context.Background(), job)

	if _, err := os.Stat(uploadedLink); err != nil {
		t.Fatalf("uploaded link should be preserved on failure: %v", err)
	}
	if _, err := os.Stat(siblingLink); err != nil {
		t.Fatalf("sibling link should be preserved on failure: %v", err)
	}
	if len(s.retryDue) != 1 {
		t.Fatalf("len(retryDue) = %d, want 1", len(s.retryDue))
	}
	if _, ok := s.retryDue[job.key]; !ok {
		t.Fatalf("retryDue missing uploaded file key %q", job.key)
	}
}

func TestRunUploadFailureBelowRetryLimitKeepsRetrying(t *testing.T) {
	t.Parallel()

	s := newTestService()
	s.cfg.RetryInterval = time.Minute
	s.cfg.MaxRetryCount = 3
	s.now = func() time.Time { return time.Date(2026, 4, 23, 11, 0, 0, 0, time.UTC) }
	s.copyJob = func(context.Context, *jobRuntime) error { return errors.New("copy failed") }

	job := &jobRuntime{
		cfg:        config.JobConfig{Name: "MOVIE", SourceDir: "/source", LinkDir: "/link", RcloneRemote: "remote:movie"},
		key:        "/link/movie.mkv",
		sourcePath: "/source/movie.mkv",
		linkPath:   "/link/movie.mkv",
		remoteDir:  "remote:movie/",
		active:     true,
	}
	s.jobs[job.key] = job
	s.scheduler.MarkDirty(job.key)
	if !s.scheduler.TryStart(job.key) {
		t.Fatal("TryStart() = false, want true")
	}

	s.runUpload(context.Background(), job)

	if got := s.failureCounts[job.key]; got != 1 {
		t.Fatalf("failureCounts[%q] = %d, want 1", job.key, got)
	}
	if _, ok := s.retryDue[job.key]; !ok {
		t.Fatalf("retryDue missing %q below retry limit", job.key)
	}
}

func TestRunUploadFailureAtRetryLimitStopsRetryAndNotifies(t *testing.T) {
	t.Parallel()

	var notified jobFailureNotification
	var notifyCalls int

	s := newTestService()
	s.cfg.RetryInterval = time.Minute
	s.cfg.MaxRetryCount = 2
	s.now = func() time.Time { return time.Date(2026, 4, 23, 11, 0, 0, 0, time.UTC) }
	s.copyJob = func(context.Context, *jobRuntime) error { return errors.New("copy failed") }
	s.notifyFinalFailure = func(n jobFailureNotification) error {
		notifyCalls++
		notified = n
		return nil
	}

	job := &jobRuntime{
		cfg:        config.JobConfig{Name: "MOVIE", SourceDir: "/source", LinkDir: "/link", RcloneRemote: "remote:movie"},
		key:        "/link/movie.mkv",
		sourcePath: "/source/movie.mkv",
		linkPath:   "/link/movie.mkv",
		remoteDir:  "remote:movie/",
		active:     true,
	}
	s.jobs[job.key] = job
	s.failureCounts[job.key] = 1
	s.scheduler.MarkDirty(job.key)
	if !s.scheduler.TryStart(job.key) {
		t.Fatal("TryStart() = false, want true")
	}

	s.runUpload(context.Background(), job)

	if _, ok := s.failureCounts[job.key]; ok {
		t.Fatalf("failureCounts[%q] still exists after terminal failure", job.key)
	}
	if _, ok := s.retryDue[job.key]; ok {
		t.Fatalf("retryDue[%q] exists, want retry to stop at limit", job.key)
	}
	if notifyCalls != 1 {
		t.Fatalf("notifyCalls = %d, want 1", notifyCalls)
	}
	if notified.LinkPath != job.linkPath {
		t.Fatalf("notified.LinkPath = %q, want %q", notified.LinkPath, job.linkPath)
	}
	if notified.RetryCount != 2 {
		t.Fatalf("notified.RetryCount = %d, want 2", notified.RetryCount)
	}
	if got := s.recentEvents[len(s.recentEvents)-1].message; got != "[MOVIE] movie.mkv｜上传失败，达到最大重试次数，停止重试" {
		t.Fatalf("recentEvents[last].message = %q, want retry-limit event", got)
	}
}

func TestRunUploadFailureAtRetryLimitClearsTerminalState(t *testing.T) {
	t.Parallel()

	s := newTestService()
	s.cfg.MaxRetryCount = 1
	s.copyJob = func(context.Context, *jobRuntime) error { return errors.New("copy failed") }

	job := &jobRuntime{
		cfg:        config.JobConfig{Name: "MOVIE", SourceDir: "/source", LinkDir: "/link", RcloneRemote: "remote:movie"},
		key:        "/link/movie.mkv",
		sourcePath: "/source/movie.mkv",
		linkPath:   "/link/movie.mkv",
		remoteDir:  "remote:movie/",
		active:     true,
	}
	s.jobs[job.key] = job
	s.failureCounts[job.key] = 0
	s.retryDue[job.key] = time.Now().Add(time.Minute)
	s.scheduler.MarkDirty(job.key)
	if !s.scheduler.TryStart(job.key) {
		t.Fatal("TryStart() = false, want true")
	}

	s.runUpload(context.Background(), job)

	if _, ok := s.jobs[job.key]; ok {
		t.Fatalf("jobs[%q] still exists after terminal failure", job.key)
	}
	if _, ok := s.failureCounts[job.key]; ok {
		t.Fatalf("failureCounts[%q] still exists after terminal failure", job.key)
	}
	if _, ok := s.retryDue[job.key]; ok {
		t.Fatalf("retryDue[%q] still exists after terminal failure", job.key)
	}
	if s.scheduler.Forget(job.key) {
		t.Fatalf("scheduler.Forget(%q) = true, want scheduler state already forgotten", job.key)
	}
}

func TestRunUploadSuccessClearsFailureCount(t *testing.T) {
	t.Parallel()

	s := newTestService()
	s.copyJob = func(context.Context, *jobRuntime) error { return nil }
	s.failureCounts["/link/movie.mkv"] = 2

	job := &jobRuntime{
		cfg:        config.JobConfig{Name: "MOVIE", SourceDir: "/source", LinkDir: "/link", RcloneRemote: "remote:movie"},
		key:        "/link/movie.mkv",
		sourcePath: "/source/movie.mkv",
		linkPath:   "/link/movie.mkv",
		remoteDir:  "remote:movie/",
		active:     true,
	}
	s.jobs[job.key] = job
	s.scheduler.MarkDirty(job.key)
	if !s.scheduler.TryStart(job.key) {
		t.Fatal("TryStart() = false, want true")
	}

	s.runUpload(context.Background(), job)

	if _, ok := s.failureCounts[job.key]; ok {
		t.Fatalf("failureCounts still contains %q after success", job.key)
	}
}

func TestProcessFileRetryWaitReplacementClearsFailureCount(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	sourceDir := filepath.Join(root, "source")
	linkDir := filepath.Join(root, "link")
	sourcePath := filepath.Join(sourceDir, "movie.mkv")
	if err := os.MkdirAll(sourceDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(sourcePath, []byte("old"), 0o644); err != nil {
		t.Fatal(err)
	}

	linkPath, err := watcher.LinkFile(sourceDir, linkDir, sourcePath)
	if err != nil {
		t.Fatalf("LinkFile() error = %v", err)
	}

	cfgJob := config.JobConfig{Name: "MOVIE", SourceDir: sourceDir, LinkDir: linkDir, RcloneRemote: "remote:movie"}
	s := newTestService()
	s.cfg.Extensions = []string{".mkv"}
	s.cfg.StableDuration = time.Millisecond
	s.cfg.PollInterval = time.Millisecond
	s.failureCounts[linkPath] = 2
	s.jobs[linkPath] = &jobRuntime{
		cfg:        cfgJob,
		key:        linkPath,
		sourcePath: sourcePath,
		linkPath:   linkPath,
		remoteDir:  "remote:movie/",
	}
	s.scheduler.MarkDirty(linkPath)
	if !s.scheduler.TryStart(linkPath) {
		t.Fatal("TryStart() = false, want true")
	}
	s.scheduler.FinishFailed(linkPath)
	s.retryDue[linkPath] = s.currentTime().Add(time.Minute)

	if err := os.Remove(sourcePath); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(sourcePath, []byte("new"), 0o644); err != nil {
		t.Fatal(err)
	}

	s.processFile(context.Background(), cfgJob, sourcePath)

	if _, ok := s.failureCounts[linkPath]; ok {
		t.Fatalf("failureCounts still contains %q after retry-wait replacement", linkPath)
	}
}

func TestProcessFileTerminalFailedReplacementResetsFailureCount(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	sourceDir := filepath.Join(root, "source")
	linkDir := filepath.Join(root, "link")
	sourcePath := filepath.Join(sourceDir, "movie.mkv")
	if err := os.MkdirAll(sourceDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(sourcePath, []byte("old"), 0o644); err != nil {
		t.Fatal(err)
	}

	linkPath, err := watcher.LinkFile(sourceDir, linkDir, sourcePath)
	if err != nil {
		t.Fatalf("LinkFile() error = %v", err)
	}

	cfgJob := config.JobConfig{Name: "MOVIE", SourceDir: sourceDir, LinkDir: linkDir, RcloneRemote: "remote:movie"}
	s := newTestService()
	s.cfg.Extensions = []string{".mkv"}
	s.cfg.StableDuration = time.Millisecond
	s.cfg.PollInterval = time.Millisecond
	s.cfg.RetryInterval = time.Minute
	s.cfg.MaxRetryCount = 2
	s.copyJob = func(context.Context, *jobRuntime) error { return errors.New("copy failed") }
	s.jobs[linkPath] = &jobRuntime{
		cfg:        cfgJob,
		key:        linkPath,
		sourcePath: sourcePath,
		linkPath:   linkPath,
		remoteDir:  "remote:movie/",
		active:     false,
	}
	s.failureCounts[linkPath] = 2

	if err := os.Remove(sourcePath); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(sourcePath, []byte("new"), 0o644); err != nil {
		t.Fatal(err)
	}

	s.processFile(context.Background(), cfgJob, sourcePath)

	if got := s.failureCounts[linkPath]; got != 0 {
		t.Fatalf("failureCounts[%q] = %d after replacement, want cleared before next upload", linkPath, got)
	}
	ready := s.scheduler.Ready()
	if len(ready) != 1 || ready[0] != linkPath {
		t.Fatalf("Ready() = %v, want [%q] after replacement", ready, linkPath)
	}
	if !s.scheduler.TryStart(linkPath) {
		t.Fatal("TryStart() = false, want true")
	}

	s.runUpload(context.Background(), s.jobs[linkPath])

	if got := s.failureCounts[linkPath]; got != 1 {
		t.Fatalf("failureCounts[%q] = %d after first failure of replacement, want 1", linkPath, got)
	}
	if _, ok := s.retryDue[linkPath]; !ok {
		t.Fatalf("retryDue[%q] missing, want replacement to retry after first failure", linkPath)
	}
}

func TestRunUploadRetryLimitNotificationErrorIsLoggedOnly(t *testing.T) {
	t.Parallel()

	var logBuf strings.Builder
	s := newTestService()
	s.logger = log.New(&logBuf, "", 0)
	s.cfg.RetryInterval = time.Minute
	s.cfg.MaxRetryCount = 1
	s.copyJob = func(context.Context, *jobRuntime) error { return errors.New("copy failed") }
	s.notifyFinalFailure = func(jobFailureNotification) error { return errors.New("telegram send failed") }

	job := &jobRuntime{
		cfg:        config.JobConfig{Name: "MOVIE", SourceDir: "/source", LinkDir: "/link", RcloneRemote: "remote:movie"},
		key:        "/link/movie.mkv",
		sourcePath: "/source/movie.mkv",
		linkPath:   "/link/movie.mkv",
		remoteDir:  "remote:movie/",
		active:     true,
	}
	s.jobs[job.key] = job
	s.scheduler.MarkDirty(job.key)
	if !s.scheduler.TryStart(job.key) {
		t.Fatal("TryStart() = false, want true")
	}

	s.runUpload(context.Background(), job)

	if !strings.Contains(logBuf.String(), "telegram send failed") {
		t.Fatalf("log output = %q, want telegram error log", logBuf.String())
	}
	if _, ok := s.retryDue[job.key]; ok {
		t.Fatalf("retryDue[%q] exists, want retry to stay stopped after notify error", job.key)
	}
}

func TestNewTelegramNotifierUsesBoundedHTTPTimeout(t *testing.T) {
	t.Parallel()

	n := newTelegramNotifier(config.TelegramConfig{
		Enabled:  true,
		BotToken: "token",
		ChatID:   "chat",
	}, config.ProxyConfig{})
	if n == nil {
		t.Fatal("newTelegramNotifier() = nil, want notifier")
	}
	if n.client == nil {
		t.Fatal("notifier client = nil, want http client")
	}
	if n.client == http.DefaultClient {
		t.Fatal("notifier client reused http.DefaultClient, want dedicated bounded client")
	}
	if n.client.Timeout != telegramNotifyTimeout {
		t.Fatalf("client.Timeout = %v, want %v", n.client.Timeout, telegramNotifyTimeout)
	}
}

func TestNewTelegramNotifierUsesConfiguredProxy(t *testing.T) {
	t.Parallel()

	n := newTelegramNotifier(config.TelegramConfig{
		Enabled:  true,
		BotToken: "token",
		ChatID:   "chat",
	}, config.ProxyConfig{
		Enabled:  true,
		Scheme:   "http",
		Host:     "127.0.0.1",
		Port:     7890,
		Username: "demo@user",
		Password: "p@ss:word",
	})
	if n == nil {
		t.Fatal("newTelegramNotifier() = nil, want notifier")
	}

	transport, ok := n.client.Transport.(*http.Transport)
	if !ok {
		t.Fatalf("client.Transport = %T, want *http.Transport", n.client.Transport)
	}
	if transport.Proxy == nil {
		t.Fatal("transport.Proxy = nil, want configured proxy")
	}

	req, err := http.NewRequest(http.MethodPost, "https://api.telegram.org/bottoken/sendMessage", nil)
	if err != nil {
		t.Fatal(err)
	}
	proxyURL, err := transport.Proxy(req)
	if err != nil {
		t.Fatalf("transport.Proxy() error = %v", err)
	}
	if proxyURL == nil {
		t.Fatal("transport.Proxy() = nil, want proxy URL")
	}

	want := "http://demo%40user:p%40ss%3Aword@127.0.0.1:7890"
	if proxyURL.String() != want {
		t.Fatalf("proxyURL = %q, want %q", proxyURL.String(), want)
	}
}

func TestRunUploadBindsRcloneOutputToJob(t *testing.T) {
	t.Parallel()

	s := newTestService()
	s.now = func() time.Time {
		return time.Date(2026, 4, 23, 10, 0, 8, 0, time.UTC)
	}
	linkPath := "/root/child/movie.mkv"
	job := &jobRuntime{
		cfg: config.JobConfig{
			Name:         "MOVIE",
			SourceDir:    "/source/child",
			LinkDir:      "/root/child",
			RcloneRemote: "remote:child",
		},
		key:        linkPath,
		sourcePath: "/source/child/movie.mkv",
		linkPath:   linkPath,
		remoteDir:  "remote:child/",
		active:     true,
	}
	s.jobs["/root/other/other.mkv"] = &jobRuntime{
		cfg:        config.JobConfig{Name: "OTHER", SourceDir: "/other", LinkDir: "/root/other", RcloneRemote: "remote:other"},
		key:        "/root/other/other.mkv",
		sourcePath: "/other/other.mkv",
		linkPath:   "/root/other/other.mkv",
		remoteDir:  "remote:other/",
	}
	s.jobs[job.key] = job
	s.copyJob = func(ctx context.Context, gotJob *jobRuntime) error {
		if gotJob != job {
			t.Fatalf("copyJob job = %#v, want %#v", gotJob, job)
		}
		s.handleRcloneOutputLine(gotJob, "2026/04/23 10:00:08 INFO  : nested/file.mkv: Copied (new)", s.now)
		return nil
	}
	s.cleanupLinkedFile = func(string, string) error { return nil }
	s.scheduler.MarkDirty(job.key)
	if !s.scheduler.TryStart(job.key) {
		t.Fatal("TryStart() = false, want true")
	}

	s.runUpload(context.Background(), job)

	if len(s.recentEvents) < 1 {
		t.Fatalf("len(recentEvents) = %d, want at least 1", len(s.recentEvents))
	}
	found := false
	for _, event := range s.recentEvents {
		if event.message == "nested/file.mkv: Copied (new)" {
			found = true
		}
	}
	if !found {
		t.Fatalf("recentEvents = %#v, want bound rclone event on target job", s.recentEvents)
	}
}

func TestRunUploadLogsRcloneCommand(t *testing.T) {
	t.Parallel()

	var buf strings.Builder
	s := newTestService()
	s.logger = log.New(&buf, "", 0)
	s.cfg.RcloneArgs = []string{"--stats=1s", "--stats-one-line", "-v"}
	s.copyJob = func(context.Context, *jobRuntime) error { return nil }
	s.cleanupLinkedFile = func(string, string) error { return nil }
	job := &jobRuntime{
		cfg: config.JobConfig{
			Name:         "MOVIE",
			SourceDir:    "/dld/upload/My Movie 2025",
			LinkDir:      "/dld/gd upload/My Movie 2025",
			RcloneRemote: "gd1:/sync/Movie/My Movie 2025",
		},
		key:        "/dld/gd upload/My Movie 2025/movie file.mkv",
		sourcePath: "/dld/upload/My Movie 2025/movie file.mkv",
		linkPath:   "/dld/gd upload/My Movie 2025/movie file.mkv",
		remoteDir:  "gd1:/sync/Movie/My Movie 2025",
		active:     true,
	}
	s.jobs[job.key] = job
	s.scheduler.MarkDirty(job.key)
	if !s.scheduler.TryStart(job.key) {
		t.Fatal("TryStart() = false, want true")
	}

	s.runUpload(context.Background(), job)

	got := buf.String()
	want := "run rclone command for MOVIE: rclone copy '/dld/gd upload/My Movie 2025/movie file.mkv' 'gd1:/sync/Movie/My Movie 2025/' --stats=1s --stats-one-line -v\n"
	if !strings.Contains(got, want) {
		t.Fatalf("log output = %q, want substring %q", got, want)
	}
}

func TestCopyWithRcloneUsesLinkedFileAndRemoteDir(t *testing.T) {
	root := t.TempDir()
	binaryPath := filepath.Join(root, "rclone")
	capturePath := filepath.Join(root, "captured-args.txt")
	script := "#!/bin/sh\nprintf '%s\\n' \"$@\" > \"$RCLONE_ARGS_CAPTURE\"\n"
	if err := os.WriteFile(binaryPath, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", root+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("RCLONE_ARGS_CAPTURE", capturePath)

	s := newTestService()
	s.cfg.RcloneArgs = []string{"--stats=1s", "--stats-one-line", "-v"}
	job := &jobRuntime{
		cfg: config.JobConfig{
			Name:         "MOVIE",
			SourceDir:    "/dld/upload/Movie-2025",
			LinkDir:      "/dld/gd_upload/Movie-2025",
			RcloneRemote: "gd1:/sync/Movie/Movie-2025",
		},
		key:        "/dld/gd_upload/Movie-2025/movie.mkv",
		sourcePath: "/dld/upload/Movie-2025/movie.mkv",
		linkPath:   "/dld/gd_upload/Movie-2025/movie.mkv",
		remoteDir:  "gd1:/sync/Movie/Movie-2025",
	}

	if err := s.copyWithRclone(context.Background(), job); err != nil {
		t.Fatalf("copyWithRclone() error = %v", err)
	}

	raw, err := os.ReadFile(capturePath)
	if err != nil {
		t.Fatalf("ReadFile(%q) error = %v", capturePath, err)
	}
	got := strings.Split(strings.TrimSpace(string(raw)), "\n")
	want := []string{
		"copy",
		"/dld/gd_upload/Movie-2025/movie.mkv",
		"gd1:/sync/Movie/Movie-2025/",
		"--stats=1s",
		"--stats-one-line",
		"-v",
	}
	if len(got) != len(want) {
		t.Fatalf("captured args = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("captured args = %v, want %v", got, want)
		}
	}
}

func TestRemoteDirWithTrailingSlashPreservesBareRemoteRoot(t *testing.T) {
	t.Parallel()

	if got, want := remoteDirWithTrailingSlash("remote:"), "remote:"; got != want {
		t.Fatalf("remoteDirWithTrailingSlash() = %q, want %q", got, want)
	}
}

func TestReleaseRetriesRecordsRequeueEvent(t *testing.T) {
	t.Parallel()

	job := config.JobConfig{Name: "MOVIE", SourceDir: "/source", LinkDir: "/link"}
	linkPath := "/link/movie.mkv"
	s := newTestService()
	s.now = func() time.Time {
		return time.Date(2026, 4, 23, 10, 0, 7, 0, time.UTC)
	}
	s.jobs[linkPath] = &jobRuntime{
		cfg:        job,
		key:        linkPath,
		sourcePath: "/source/movie.mkv",
		linkPath:   linkPath,
		remoteDir:  "remote:movie/",
	}
	s.scheduler.MarkDirty(linkPath)
	if !s.scheduler.TryStart(linkPath) {
		t.Fatal("TryStart() = false, want true")
	}
	s.scheduler.FinishFailed(linkPath)
	s.retryDue[linkPath] = s.currentTime().Add(-time.Second)

	s.releaseRetries()

	if len(s.recentEvents) != 1 {
		t.Fatalf("len(recentEvents) = %d, want 1", len(s.recentEvents))
	}
	if got := s.recentEvents[0].message; got != "[MOVIE] movie.mkv｜到达重试时间，重新排队" {
		t.Fatalf("recentEvents[0].message = %q, want retry requeue event", got)
	}
}

func TestReleaseRetriesDoesNotRecordEventBeforeDueTime(t *testing.T) {
	t.Parallel()

	job := config.JobConfig{Name: "MOVIE", SourceDir: "/source", LinkDir: "/link"}
	linkPath := "/link/movie.mkv"
	s := newTestService()
	s.now = func() time.Time {
		return time.Date(2026, 4, 23, 10, 0, 9, 0, time.UTC)
	}
	s.jobs[linkPath] = &jobRuntime{
		cfg:        job,
		key:        linkPath,
		sourcePath: "/source/movie.mkv",
		linkPath:   linkPath,
		remoteDir:  "remote:movie/",
	}
	s.scheduler.MarkDirty(linkPath)
	if !s.scheduler.TryStart(linkPath) {
		t.Fatal("TryStart() = false, want true")
	}
	s.scheduler.FinishFailed(linkPath)
	s.retryDue[linkPath] = s.currentTime().Add(time.Minute)

	s.releaseRetries()

	if len(s.recentEvents) != 0 {
		t.Fatalf("len(recentEvents) = %d, want 0", len(s.recentEvents))
	}
	if ready := s.scheduler.Ready(); len(ready) != 0 {
		t.Fatalf("Ready() = %v, want [] before retry due", ready)
	}
}

func newTestService() *Service {
	return &Service{
		cfg:                &config.Config{MaxParallelUploads: 5},
		logger:             log.New(io.Discard, "", 0),
		scheduler:          queue.New(queue.Options{MaxParallel: 5}),
		configJobs:         map[string]config.JobConfig{},
		jobs:               map[string]*jobRuntime{},
		scanLinkedFiles:    func(string, []string) ([]string, error) { return nil, nil },
		cleanupLinkedFile:  func(string, string) error { return nil },
		retryDue:           map[string]time.Time{},
		failureCounts:      map[string]int{},
		notifyFinalFailure: func(jobFailureNotification) error { return nil },
		wakeCh:             make(chan struct{}, 1),
		uiWakeCh:           make(chan struct{}, 1),
	}
}

func waitForReadyJob(t *testing.T, s *Service, want string) {
	t.Helper()

	timeout := time.After(time.Second)
	tick := time.NewTicker(10 * time.Millisecond)
	defer tick.Stop()

	for {
		if ready := s.scheduler.Ready(); len(ready) == 1 && ready[0] == want {
			return
		}

		select {
		case <-timeout:
			t.Fatalf("Ready() = %v, want latest same-path task queued", s.scheduler.Ready())
		case <-tick.C:
		}
	}
}

type recordingWriter struct {
	mu     sync.Mutex
	buf    strings.Builder
	writes chan struct{}
}

func newRecordingWriter() *recordingWriter {
	return &recordingWriter{
		writes: make(chan struct{}, 8),
	}
}

func (w *recordingWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()

	_, _ = w.buf.Write(p)
	select {
	case w.writes <- struct{}{}:
	default:
	}
	return len(p), nil
}

func (w *recordingWriter) String() string {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.buf.String()
}

func (w *recordingWriter) waitForWrites(t *testing.T, n int) {
	t.Helper()

	timeout := time.After(time.Second)
	for i := 0; i < n; i++ {
		select {
		case <-w.writes:
		case <-timeout:
			t.Fatalf("timed out waiting for write %d; current output = %q", i+1, w.String())
		}
	}
}

func (w *recordingWriter) assertNoWrite(t *testing.T) {
	t.Helper()

	select {
	case <-w.writes:
		t.Fatalf("expected no additional writes; current output = %q", w.String())
	case <-time.After(100 * time.Millisecond):
	}
}
