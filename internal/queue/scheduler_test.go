package queue_test

import (
	"testing"
	"time"

	"github.com/wangdazhuo/media-backup/internal/queue"
)

func TestSchedulerDeduplicatesQueuedJob(t *testing.T) {
	t.Parallel()

	s := queue.New(queue.Options{MaxParallel: 5, RetryInterval: 10 * time.Minute})
	s.MarkDirty("job-a")
	s.MarkDirty("job-a")

	ready := s.Ready()
	if len(ready) != 1 || ready[0] != "job-a" {
		t.Fatalf("Ready() = %v, want [job-a]", ready)
	}
}

func TestSchedulerRespectsPerJobSerialAndGlobalLimit(t *testing.T) {
	t.Parallel()

	s := queue.New(queue.Options{MaxParallel: 2, RetryInterval: 10 * time.Minute})
	s.MarkDirty("job-a")
	s.MarkDirty("job-b")
	s.MarkDirty("job-c")

	first := s.TryStart("job-a")
	second := s.TryStart("job-b")
	third := s.TryStart("job-c")

	if !first || !second {
		t.Fatal("expected first two jobs to start")
	}
	if third {
		t.Fatal("expected third job to wait for global slot")
	}
	if s.TryStart("job-a") {
		t.Fatal("same job should not start twice")
	}
}

func TestSchedulerSchedulesRetryWithoutDuplicateQueueEntries(t *testing.T) {
	t.Parallel()

	s := queue.New(queue.Options{MaxParallel: 1, RetryInterval: 10 * time.Minute})
	s.MarkDirty("job-a")
	if !s.TryStart("job-a") {
		t.Fatal("expected job-a to start")
	}

	s.FinishFailed("job-a")
	s.FinishFailed("job-a")

	retries := s.RetryReady()
	if len(retries) != 1 || retries[0] != "job-a" {
		t.Fatalf("RetryReady() = %v, want [job-a]", retries)
	}
}
