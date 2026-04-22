package watcher_test

import (
	"reflect"
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
	if got, want := reg.Paths(), []string{"/source", "/source/movie"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("registered paths = %v, want %v", got, want)
	}
}
