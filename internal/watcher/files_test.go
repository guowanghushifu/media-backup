package watcher

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestLinkFileCreatesParentDirectories(t *testing.T) {
	t.Parallel()

	sourceDir := t.TempDir()
	linkDir := t.TempDir()
	sourceFile := filepath.Join("show", "season-1", "episode-1.mkv")
	sourcePath := filepath.Join(sourceDir, sourceFile)
	if err := os.MkdirAll(filepath.Dir(sourcePath), 0o755); err != nil {
		t.Fatalf("MkdirAll source: %v", err)
	}
	if err := os.WriteFile(sourcePath, []byte("video"), 0o644); err != nil {
		t.Fatalf("WriteFile source: %v", err)
	}

	linkPath, err := LinkFile(sourceDir, linkDir, sourceFile)
	if err != nil {
		t.Fatalf("LinkFile() error = %v", err)
	}

	wantPath := filepath.Join(linkDir, sourceFile)
	if linkPath != wantPath {
		t.Fatalf("LinkFile() path = %q, want %q", linkPath, wantPath)
	}

	if _, err := os.Stat(filepath.Dir(linkPath)); err != nil {
		t.Fatalf("parent dir missing: %v", err)
	}

	sourceInfo, err := os.Stat(sourcePath)
	if err != nil {
		t.Fatalf("Stat source: %v", err)
	}
	linkInfo, err := os.Stat(linkPath)
	if err != nil {
		t.Fatalf("Stat link: %v", err)
	}
	if !os.SameFile(sourceInfo, linkInfo) {
		t.Fatal("link target is not a hard link to source")
	}
}

func TestCleanupLinkDirPreservesRoot(t *testing.T) {
	t.Parallel()

	linkDir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(linkDir, "a", "b"), 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(filepath.Join(linkDir, "a", "b", "file.tmp"), []byte("x"), 0o644); err != nil {
		t.Fatalf("WriteFile nested: %v", err)
	}
	if err := os.WriteFile(filepath.Join(linkDir, "root.tmp"), []byte("x"), 0o644); err != nil {
		t.Fatalf("WriteFile root: %v", err)
	}

	if err := CleanupLinkDir(linkDir); err != nil {
		t.Fatalf("CleanupLinkDir() error = %v", err)
	}

	info, err := os.Stat(linkDir)
	if err != nil {
		t.Fatalf("Stat root: %v", err)
	}
	if !info.IsDir() {
		t.Fatalf("root path is not directory: mode=%v", info.Mode())
	}

	entries, err := os.ReadDir(linkDir)
	if err != nil {
		t.Fatalf("ReadDir: %v", err)
	}
	if len(entries) != 0 {
		t.Fatalf("ReadDir() len = %d, want 0", len(entries))
	}
}

func TestWaitStableReturnsAfterSizeStopsChanging(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "growing.file")
	if err := os.WriteFile(path, []byte("x"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	stableFor := 80 * time.Millisecond
	pollInterval := 10 * time.Millisecond
	done := make(chan time.Time, 1)

	go func() {
		defer close(done)
		time.Sleep(20 * time.Millisecond)
		f, err := os.OpenFile(path, os.O_WRONLY|os.O_APPEND, 0)
		if err != nil {
			t.Errorf("OpenFile first append: %v", err)
			return
		}
		if _, err := f.Write([]byte("a")); err != nil {
			_ = f.Close()
			t.Errorf("Write first append: %v", err)
			return
		}
		if err := f.Close(); err != nil {
			t.Errorf("Close first append: %v", err)
			return
		}

		time.Sleep(20 * time.Millisecond)
		f, err = os.OpenFile(path, os.O_WRONLY|os.O_APPEND, 0)
		if err != nil {
			t.Errorf("OpenFile second append: %v", err)
			return
		}
		if _, err := f.Write([]byte("b")); err != nil {
			_ = f.Close()
			t.Errorf("Write second append: %v", err)
			return
		}
		if err := f.Close(); err != nil {
			t.Errorf("Close second append: %v", err)
			return
		}
		done <- time.Now()
	}()

	if err := WaitStable(path, stableFor, pollInterval); err != nil {
		t.Fatalf("WaitStable() error = %v", err)
	}

	returnedAt := time.Now()
	var lastWriteAt time.Time
	select {
	case lastWriteAt = <-done:
	default:
		t.Fatal("WaitStable() returned before file writes finished")
	}

	if returnedAt.Sub(lastWriteAt) < stableFor-15*time.Millisecond {
		t.Fatalf("WaitStable() returned too early: afterLastWrite=%v, stableFor=%v", returnedAt.Sub(lastWriteAt), stableFor)
	}
}
