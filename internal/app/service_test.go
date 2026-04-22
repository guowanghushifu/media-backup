package app

import (
	"io"
	"log"
	"strconv"
	"testing"
	"time"

	"github.com/wangdazhuo/media-backup/internal/config"
	"github.com/wangdazhuo/media-backup/internal/queue"
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

func newTestService() *Service {
	return &Service{
		cfg:       &config.Config{MaxParallelUploads: 5},
		logger:    log.New(io.Discard, "", 0),
		scheduler: queue.New(queue.Options{MaxParallel: 5}),
		jobs:      map[string]*jobRuntime{},
	}
}
