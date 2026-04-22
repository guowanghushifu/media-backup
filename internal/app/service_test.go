package app

import (
	"context"
	"errors"
	"io"
	"log"
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
	s.jobs["/movie"] = &jobRuntime{
		cfg:     config.JobConfig{Name: "MOVIE"},
		key:     "/movie",
		summary: "movie summary",
		active:  true,
	}
	s.jobs["/remux"] = &jobRuntime{
		cfg:     config.JobConfig{Name: "4K-REMUX"},
		key:     "/remux",
		summary: "remux summary",
		active:  true,
	}

	for i := 0; i < 50; i++ {
		active, _, _ := s.snapshotUI()
		if len(active) != 2 {
			t.Fatalf("len(active) = %d, want 2", len(active))
		}
		if active[0].Name != "4K-REMUX" || active[1].Name != "MOVIE" {
			t.Fatalf("active order = [%s %s], want [4K-REMUX MOVIE]", active[0].Name, active[1].Name)
		}
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
	job := &jobRuntime{cfg: config.JobConfig{Name: "movie"}, key: "/source"}

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
	job := &jobRuntime{cfg: config.JobConfig{Name: "movie"}, key: "/source"}

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

func TestRunUILoopUsesAlternateScreenLifecycle(t *testing.T) {
	t.Parallel()

	start := time.Date(2026, 4, 22, 17, 4, 9, 0, time.UTC)
	now := time.Date(2026, 4, 22, 17, 4, 10, 0, time.UTC)
	s := newTestService()
	writer := newRecordingWriter()
	s.uiWriter = writer
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
		ui.RewriteFrame(ui.RenderDashboard(start, active, events, waiting, s.cfg.MaxParallelUploads)) +
		ui.RewriteFrame(ui.RenderDashboard(now, active, events, waiting, s.cfg.MaxParallelUploads)) +
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
	s.scanExisting = func(string, string, []string, time.Duration) (int, error) {
		close(scanStarted)
		<-releaseScan
		close(scanReturned)
		return 0, nil
	}

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
		ui.RewriteFrame(ui.RenderDashboard(start, nil, nil, 0, s.cfg.MaxParallelUploads))
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

	ctx, cancel := context.WithCancel(context.Background())
	s.scanExisting = func(gotSourceDir, gotLinkDir string, gotExtensions []string, gotStableDuration time.Duration) (int, error) {
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
	if len(ready) != 1 || ready[0] != sourceDir {
		t.Fatalf("Ready() = %v, want [%s]", ready, sourceDir)
	}
}

func TestStartupCatchUpRecordsSchedulerEventWhenExistingFilesQueued(t *testing.T) {
	t.Parallel()

	job := config.JobConfig{Name: "MOVIE", SourceDir: "/source", LinkDir: "/link"}
	s := &Service{
		cfg:       &config.Config{StableDuration: time.Minute},
		logger:    log.New(io.Discard, "", 0),
		scheduler: queue.New(queue.Options{MaxParallel: 1}),
		jobs:      map[string]*jobRuntime{job.SourceDir: {cfg: job, key: job.SourceDir}},
		now: func() time.Time {
			return time.Date(2026, 4, 23, 10, 0, 0, 0, time.UTC)
		},
	}
	s.mkdirAll = func(string, os.FileMode) error { return nil }
	s.addWatches = func(string) error { return nil }
	s.scanExisting = func(string, string, []string, time.Duration) (int, error) { return 3, nil }

	if err := s.startupCatchUp(); err != nil {
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
		jobs:      map[string]*jobRuntime{job.SourceDir: {cfg: job, key: job.SourceDir}},
		now: func() time.Time {
			return time.Date(2026, 4, 23, 10, 0, 1, 0, time.UTC)
		},
	}

	s.processFile(context.Background(), s.jobs[job.SourceDir], path)
	s.processFile(context.Background(), s.jobs[job.SourceDir], path)

	if len(s.recentEvents) != 1 {
		t.Fatalf("len(recentEvents) = %d, want 1", len(s.recentEvents))
	}
	if got := s.recentEvents[0].message; got != "[MOVIE] 检测到新文件，任务标记为待上传" {
		t.Fatalf("recentEvents[0].message = %q, want runtime queue event", got)
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
		jobs:      map[string]*jobRuntime{job.SourceDir: {cfg: job, key: job.SourceDir}},
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

	s.processFile(context.Background(), s.jobs[job.SourceDir], path)

	if len(s.recentEvents) != 1 {
		t.Fatalf("len(recentEvents) = %d, want 1", len(s.recentEvents))
	}
	if got := s.recentEvents[0].message; got != "[MOVIE] 检测到新文件，任务标记为待上传" {
		t.Fatalf("recentEvents[0].message = %q, want runtime queue event even if dispatch wins race", got)
	}
}

func TestProcessFileDoesNotRecordQueueEventWhilePendingRetry(t *testing.T) {
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
		jobs:      map[string]*jobRuntime{job.SourceDir: {cfg: job, key: job.SourceDir}},
		retryDue:  map[string]time.Time{},
		wakeCh:    make(chan struct{}, 1),
		now: func() time.Time {
			return time.Date(2026, 4, 23, 10, 0, 8, 0, time.UTC)
		},
	}

	s.scheduler.MarkDirty(job.SourceDir)
	if !s.scheduler.TryStart(job.SourceDir) {
		t.Fatal("TryStart() = false, want true")
	}
	s.scheduler.FinishFailed(job.SourceDir)
	s.retryDue[job.SourceDir] = s.currentTime().Add(time.Minute)

	s.processFile(context.Background(), s.jobs[job.SourceDir], path)

	if len(s.recentEvents) != 0 {
		t.Fatalf("len(recentEvents) = %d, want 0", len(s.recentEvents))
	}
	if ready := s.scheduler.Ready(); len(ready) != 0 {
		t.Fatalf("Ready() = %v, want [] while pending retry", ready)
	}
}

func TestProcessFileRecordsRerunEventOnceForActiveJob(t *testing.T) {
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
		jobs: map[string]*jobRuntime{
			job.SourceDir: {cfg: job, key: job.SourceDir, active: true},
		},
		now: func() time.Time {
			return time.Date(2026, 4, 23, 10, 0, 2, 0, time.UTC)
		},
	}

	s.processFile(context.Background(), s.jobs[job.SourceDir], path)
	s.processFile(context.Background(), s.jobs[job.SourceDir], path)

	if len(s.recentEvents) != 1 {
		t.Fatalf("len(recentEvents) = %d, want 1", len(s.recentEvents))
	}
	if got := s.recentEvents[0].message; got != "[MOVIE] 检测到新文件，任务保持运行中，完成后将重新排队" {
		t.Fatalf("recentEvents[0].message = %q, want rerun event", got)
	}
}

func TestStartReadyUploadsRecordsDispatchStartEvent(t *testing.T) {
	t.Parallel()

	job := config.JobConfig{Name: "MOVIE", SourceDir: "/source", LinkDir: "/link"}
	s := newTestService()
	s.jobs[job.SourceDir] = &jobRuntime{cfg: job, key: job.SourceDir}
	s.now = func() time.Time {
		return time.Date(2026, 4, 23, 10, 0, 3, 0, time.UTC)
	}
	s.startUpload = func(context.Context, *jobRuntime) {}
	s.scheduler.MarkDirty(job.SourceDir)

	s.startReadyUploads(context.Background())

	if len(s.recentEvents) != 1 {
		t.Fatalf("len(recentEvents) = %d, want 1", len(s.recentEvents))
	}
	if got := s.recentEvents[0].message; got != "[MOVIE] 调度开始上传" {
		t.Fatalf("recentEvents[0].message = %q, want dispatch start event", got)
	}
}

func TestRunUploadRecordsCompletionAndFailureEvents(t *testing.T) {
	t.Parallel()

	makeService := func(at time.Time) (*Service, *jobRuntime) {
		s := newTestService()
		s.cfg.RetryInterval = time.Minute
		s.now = func() time.Time { return at }
		s.copyJob = func(context.Context, *jobRuntime) error { return nil }
		s.cleanupLinkDir = func(string) error { return nil }
		job := &jobRuntime{
			cfg:    config.JobConfig{Name: "MOVIE", LinkDir: "/link"},
			key:    "/source",
			active: true,
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
		if got := s.recentEvents[0].message; got != "[MOVIE] 上传完成，任务清空" {
			t.Fatalf("recentEvents[0].message = %q, want success event", got)
		}
	})

	t.Run("success with dirty reruns", func(t *testing.T) {
		s, job := makeService(time.Date(2026, 4, 23, 10, 0, 5, 0, time.UTC))
		job.dirtyDuringRun = true

		s.runUpload(context.Background(), job)

		if len(s.recentEvents) != 1 {
			t.Fatalf("len(recentEvents) = %d, want 1", len(s.recentEvents))
		}
		if got := s.recentEvents[0].message; got != "[MOVIE] 上传完成，检测到新增文件，重新排队" {
			t.Fatalf("recentEvents[0].message = %q, want rerun completion event", got)
		}
	})

	t.Run("copy failure enters retry", func(t *testing.T) {
		s, job := makeService(time.Date(2026, 4, 23, 10, 0, 6, 0, time.UTC))
		s.copyJob = func(context.Context, *jobRuntime) error { return errors.New("copy failed") }

		s.runUpload(context.Background(), job)

		if len(s.recentEvents) != 1 {
			t.Fatalf("len(recentEvents) = %d, want 1", len(s.recentEvents))
		}
		if got := s.recentEvents[0].message; got != "[MOVIE] 上传失败，进入重试等待" {
			t.Fatalf("recentEvents[0].message = %q, want failure event", got)
		}
	})

	t.Run("cleanup failure enters retry", func(t *testing.T) {
		s, job := makeService(time.Date(2026, 4, 23, 10, 0, 7, 0, time.UTC))
		s.cleanupLinkDir = func(string) error { return errors.New("cleanup failed") }

		s.runUpload(context.Background(), job)

		if len(s.recentEvents) != 1 {
			t.Fatalf("len(recentEvents) = %d, want 1", len(s.recentEvents))
		}
		if got := s.recentEvents[0].message; got != "[MOVIE] 上传失败，进入重试等待" {
			t.Fatalf("recentEvents[0].message = %q, want cleanup failure retry event", got)
		}
	})
}

func TestRunUploadBindsRcloneOutputToJob(t *testing.T) {
	t.Parallel()

	s := newTestService()
	s.now = func() time.Time {
		return time.Date(2026, 4, 23, 10, 0, 8, 0, time.UTC)
	}
	job := &jobRuntime{
		cfg:    config.JobConfig{Name: "MOVIE", LinkDir: "/root/child"},
		key:    "/source",
		active: true,
	}
	s.jobs["/other"] = &jobRuntime{
		cfg: config.JobConfig{Name: "OTHER", LinkDir: "/root"},
		key: "/other",
	}
	s.jobs[job.key] = job
	s.copyJob = func(ctx context.Context, gotJob *jobRuntime) error {
		if gotJob != job {
			t.Fatalf("copyJob job = %#v, want %#v", gotJob, job)
		}
		s.handleRcloneOutputLine(gotJob, "2026/04/23 10:00:08 INFO  : nested/file.mkv: Copied (new)", s.now)
		return nil
	}
	s.cleanupLinkDir = func(string) error { return nil }
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

func TestReleaseRetriesRecordsRequeueEvent(t *testing.T) {
	t.Parallel()

	job := config.JobConfig{Name: "MOVIE", SourceDir: "/source", LinkDir: "/link"}
	s := newTestService()
	s.now = func() time.Time {
		return time.Date(2026, 4, 23, 10, 0, 7, 0, time.UTC)
	}
	s.jobs[job.SourceDir] = &jobRuntime{cfg: job, key: job.SourceDir}
	s.scheduler.MarkDirty(job.SourceDir)
	if !s.scheduler.TryStart(job.SourceDir) {
		t.Fatal("TryStart() = false, want true")
	}
	s.scheduler.FinishFailed(job.SourceDir)
	s.retryDue[job.SourceDir] = s.currentTime().Add(-time.Second)

	s.releaseRetries()

	if len(s.recentEvents) != 1 {
		t.Fatalf("len(recentEvents) = %d, want 1", len(s.recentEvents))
	}
	if got := s.recentEvents[0].message; got != "[MOVIE] 到达重试时间，重新排队" {
		t.Fatalf("recentEvents[0].message = %q, want retry requeue event", got)
	}
}

func TestReleaseRetriesDoesNotRecordEventBeforeDueTime(t *testing.T) {
	t.Parallel()

	job := config.JobConfig{Name: "MOVIE", SourceDir: "/source", LinkDir: "/link"}
	s := newTestService()
	s.now = func() time.Time {
		return time.Date(2026, 4, 23, 10, 0, 9, 0, time.UTC)
	}
	s.jobs[job.SourceDir] = &jobRuntime{cfg: job, key: job.SourceDir}
	s.scheduler.MarkDirty(job.SourceDir)
	if !s.scheduler.TryStart(job.SourceDir) {
		t.Fatal("TryStart() = false, want true")
	}
	s.scheduler.FinishFailed(job.SourceDir)
	s.retryDue[job.SourceDir] = s.currentTime().Add(time.Minute)

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
		cfg:       &config.Config{MaxParallelUploads: 5},
		logger:    log.New(io.Discard, "", 0),
		scheduler: queue.New(queue.Options{MaxParallel: 5}),
		jobs:      map[string]*jobRuntime{},
		retryDue:  map[string]time.Time{},
		wakeCh:    make(chan struct{}, 1),
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
