package watcher_test

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/wangdazhuo/media-backup/internal/watcher"
)

func TestScanLinksMatchingMediaFiles(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	sourceDir := filepath.Join(root, "source")
	linkDir := filepath.Join(root, "link")
	if err := os.MkdirAll(filepath.Join(sourceDir, "movie"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sourceDir, "movie", "feature.mkv"), []byte("data"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sourceDir, "movie", "ignore.txt"), []byte("data"), 0o644); err != nil {
		t.Fatal(err)
	}

	count, err := watcher.ScanAndLink(sourceDir, linkDir, []string{".mkv"}, time.Millisecond)
	if err != nil {
		t.Fatalf("ScanAndLink() error = %v", err)
	}
	if count != 1 {
		t.Fatalf("ScanAndLink() count = %d, want 1", count)
	}
}
