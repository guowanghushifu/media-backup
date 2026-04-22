package app

import (
	"io"
	"log"
	"testing"
	"time"

	"github.com/wangdazhuo/media-backup/internal/config"
	"github.com/wangdazhuo/media-backup/internal/queue"
)

func TestSnapshotUIHidesExpiredEvents(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 4, 22, 17, 4, 10, 0, time.UTC)
	job := &jobRuntime{
		cfg:     config.JobConfig{Name: "movie"},
		key:     "/source",
		summary: "832 MiB / 1000 MiB, 83%, 29.793 MiB/s, ETA 5s",
		event:   "THIS_IS_TEST/uploadtest.bin: Copied (new)",
		active:  true,
	}

	s := &Service{
		cfg:       &config.Config{MaxParallelUploads: 5},
		logger:    log.New(io.Discard, "", 0),
		scheduler: queue.New(queue.Options{MaxParallel: 5}),
		jobs:      map[string]*jobRuntime{job.key: job},
	}

	job.eventAt = now.Add(-recentEventTTL + time.Second)
	active, waiting := s.snapshotUI(now)
	if waiting != 0 {
		t.Fatalf("waiting = %d, want 0", waiting)
	}
	if len(active) != 1 {
		t.Fatalf("len(active) = %d, want 1", len(active))
	}
	if active[0].Event == "" {
		t.Fatal("recent event should be visible")
	}

	job.eventAt = now.Add(-recentEventTTL - time.Second)
	active, _ = s.snapshotUI(now)
	if active[0].Event != "" {
		t.Fatalf("expired event = %q, want empty", active[0].Event)
	}
}
