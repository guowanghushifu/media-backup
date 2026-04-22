package watcher_test

import (
	"os"
	"path/filepath"
	"strings"
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

	linkedMedia := filepath.Join(linkDir, "movie", "feature.mkv")
	if _, err := os.Stat(linkedMedia); err != nil {
		t.Fatalf("expected linked media file missing at %q: %v", linkedMedia, err)
	}

	linkedText := filepath.Join(linkDir, "movie", "ignore.txt")
	if _, err := os.Stat(linkedText); !os.IsNotExist(err) {
		t.Fatalf("expected non-media file not linked at %q, got err=%v", linkedText, err)
	}
}

func TestScanSkipsLinkDirWhenNestedUnderSource(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	sourceDir := filepath.Join(root, "source")
	linkDir := filepath.Join(sourceDir, "zzzlinks")
	if err := os.MkdirAll(filepath.Join(sourceDir, "movie"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(linkDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sourceDir, "movie", "feature.mkv"), []byte("data"), 0o644); err != nil {
		t.Fatal(err)
	}

	count, err := watcher.ScanAndLink(sourceDir, linkDir, []string{".mkv"}, time.Millisecond)
	if err != nil {
		t.Fatalf("ScanAndLink() error = %v", err)
	}
	if count != 1 {
		t.Fatalf("ScanAndLink() count = %d, want 1", count)
	}

	linkedMedia := filepath.Join(linkDir, "movie", "feature.mkv")
	if _, err := os.Stat(linkedMedia); err != nil {
		t.Fatalf("expected linked media file missing at %q: %v", linkedMedia, err)
	}

	nestedSelfLink := filepath.Join(linkDir, "zzzlinks", "movie", "feature.mkv")
	if _, err := os.Stat(nestedSelfLink); !os.IsNotExist(err) {
		t.Fatalf("expected no nested self-linked file at %q, got err=%v", nestedSelfLink, err)
	}

	if err := filepath.WalkDir(linkDir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() && strings.Contains(path, string(filepath.Separator)+"zzzlinks"+string(filepath.Separator)+"zzzlinks") {
			t.Fatalf("unexpected recursive nested link dir found: %q", path)
		}
		return nil
	}); err != nil {
		t.Fatalf("WalkDir(linkDir): %v", err)
	}
}
