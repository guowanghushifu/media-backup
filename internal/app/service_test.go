package app

import (
	"context"
	"io"
	"log"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/wangdazhuo/media-backup/internal/config"
	"github.com/wangdazhuo/media-backup/internal/queue"
	"github.com/wangdazhuo/media-backup/internal/ui"
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

	now := time.Date(2026, 4, 22, 17, 4, 10, 0, time.UTC)
	s := newTestService()
	writer := newRecordingWriter()
	s.uiWriter = writer
	ticks := make(chan time.Time)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})

	go func() {
		s.runUILoop(ctx, ticks)
		close(done)
	}()

	writer.waitForWrites(t, 1)

	ticks <- now
	writer.waitForWrites(t, 1)

	cancel()
	<-done

	active, events, waiting := s.snapshotUI()
	want := ui.EnterAlternateScreen() +
		ui.RewriteFrame(ui.RenderDashboard(now, active, events, waiting, s.cfg.MaxParallelUploads)) +
		ui.LeaveAlternateScreen() +
		"\n"
	if got := writer.String(); got != want {
		t.Fatalf("runUILoop() output = %q, want %q", got, want)
	}
}

func newTestService() *Service {
	return &Service{
		cfg:       &config.Config{MaxParallelUploads: 5},
		logger:    log.New(io.Discard, "", 0),
		scheduler: queue.New(queue.Options{MaxParallel: 5}),
		jobs:      map[string]*jobRuntime{},
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
