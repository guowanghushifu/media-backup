package watcher_test

import (
	"testing"

	"github.com/wangdazhuo/media-backup/internal/watcher"
)

func TestWatchServiceRegistersNewDirectories(t *testing.T) {
	t.Parallel()

	reg := watcher.NewMemoryRegistrar()
	svc := watcher.NewWatchService(reg)
	if err := svc.AddRecursive("/source", []string{"/source", "/source/movie"}); err != nil {
		t.Fatalf("AddRecursive() error = %v", err)
	}
	if got := reg.Paths(); len(got) != 2 {
		t.Fatalf("registered paths = %v, want 2 entries", got)
	}
}
