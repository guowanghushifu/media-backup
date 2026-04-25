package queue_test

import (
	"testing"

	"github.com/guowanghushifu/media-backup/internal/queue"
)

func TestSchedulerDeduplicatesQueuedJob(t *testing.T) {
	t.Parallel()

	s := queue.New(queue.Options{MaxParallel: 5})
	s.MarkDirty("job-a")
	s.MarkDirty("job-a")

	ready := s.Ready()
	if len(ready) != 1 || ready[0] != "job-a" {
		t.Fatalf("Ready() = %v, want [job-a]", ready)
	}
}

func TestSchedulerRespectsPerJobSerialAndGlobalLimit(t *testing.T) {
	t.Parallel()

	s := queue.New(queue.Options{MaxParallel: 2})
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

func TestSchedulerFinishFailedReleasesSlotWithoutRequeue(t *testing.T) {
	t.Parallel()

	s := queue.New(queue.Options{MaxParallel: 1})
	s.MarkDirty("job-a")
	if !s.TryStart("job-a") {
		t.Fatal("expected job-a to start")
	}

	s.FinishFailed("job-a")

	ready := s.Ready()
	if len(ready) != 0 {
		t.Fatalf("Ready() = %v, want [] until service marks retry due", ready)
	}
	s.MarkDirty("job-b")
	if !s.TryStart("job-b") {
		t.Fatal("expected another job to start after failed job releases slot")
	}
}

func TestSchedulerFinishWithDirtyTrueRequeuesJob(t *testing.T) {
	t.Parallel()

	s := queue.New(queue.Options{MaxParallel: 1})
	s.MarkDirty("job-a")
	if !s.TryStart("job-a") {
		t.Fatal("expected job-a to start")
	}

	s.Finish("job-a", true)

	ready := s.Ready()
	if len(ready) != 1 || ready[0] != "job-a" {
		t.Fatalf("Ready() = %v, want [job-a]", ready)
	}
}

func TestSchedulerReadyOrderingStable(t *testing.T) {
	t.Parallel()

	s := queue.New(queue.Options{MaxParallel: 3})
	s.MarkDirty("job-a")
	s.MarkDirty("job-b")
	s.MarkDirty("job-c")

	ready := s.Ready()
	if len(ready) != 3 || ready[0] != "job-a" || ready[1] != "job-b" || ready[2] != "job-c" {
		t.Fatalf("Ready() = %v, want [job-a job-b job-c]", ready)
	}
}

func TestSchedulerForgetDeletesTerminalJobState(t *testing.T) {
	t.Parallel()

	s := queue.New(queue.Options{MaxParallel: 1})
	s.MarkDirty("job-a")
	if !s.TryStart("job-a") {
		t.Fatal("expected job-a to start")
	}
	s.Finish("job-a", false)

	if !s.Forget("job-a") {
		t.Fatal("Forget(job-a) = false, want true for terminal job")
	}
	if s.Forget("job-a") {
		t.Fatal("Forget(job-a) = true after deletion, want false")
	}

	s.MarkDirty("job-a")
	ready := s.Ready()
	if len(ready) != 1 || ready[0] != "job-a" {
		t.Fatalf("Ready() = %v, want [job-a] after re-adding forgotten key", ready)
	}
}
