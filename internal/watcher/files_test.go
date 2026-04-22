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
	sourcePath := filepath.Join(sourceDir, "show", "season-1", "episode-1.mkv")
	if err := os.MkdirAll(filepath.Dir(sourcePath), 0o755); err != nil {
		t.Fatalf("MkdirAll source: %v", err)
	}
	if err := os.WriteFile(sourcePath, []byte("video"), 0o644); err != nil {
		t.Fatalf("WriteFile source: %v", err)
	}

	linkPath, err := LinkFile(sourceDir, linkDir, sourcePath)
	if err != nil {
		t.Fatalf("LinkFile() error = %v", err)
	}

	rel, err := filepath.Rel(sourceDir, sourcePath)
	if err != nil {
		t.Fatalf("Rel source: %v", err)
	}
	wantPath := filepath.Join(linkDir, rel)
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

func TestLinkFileIsIdempotentWhenTargetExists(t *testing.T) {
	t.Parallel()

	sourceDir := t.TempDir()
	linkDir := t.TempDir()
	sourcePath := filepath.Join(sourceDir, "movie.mkv")
	if err := os.WriteFile(sourcePath, []byte("video"), 0o644); err != nil {
		t.Fatalf("WriteFile source: %v", err)
	}

	firstLinkPath, err := LinkFile(sourceDir, linkDir, sourcePath)
	if err != nil {
		t.Fatalf("first LinkFile() error = %v", err)
	}
	secondLinkPath, err := LinkFile(sourceDir, linkDir, sourcePath)
	if err != nil {
		t.Fatalf("second LinkFile() error = %v", err)
	}
	if secondLinkPath != firstLinkPath {
		t.Fatalf("second LinkFile() path = %q, want %q", secondLinkPath, firstLinkPath)
	}

	sourceInfo, err := os.Stat(sourcePath)
	if err != nil {
		t.Fatalf("Stat source: %v", err)
	}
	linkInfo, err := os.Stat(firstLinkPath)
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
	lastWriteDone := make(chan time.Time, 1)
	writeErr := make(chan error, 1)

	go func() {
		time.Sleep(20 * time.Millisecond)
		f, err := os.OpenFile(path, os.O_WRONLY|os.O_APPEND, 0)
		if err != nil {
			writeErr <- err
			return
		}
		if _, err := f.Write([]byte("a")); err != nil {
			_ = f.Close()
			writeErr <- err
			return
		}
		if err := f.Close(); err != nil {
			writeErr <- err
			return
		}

		time.Sleep(20 * time.Millisecond)
		f, err = os.OpenFile(path, os.O_WRONLY|os.O_APPEND, 0)
		if err != nil {
			writeErr <- err
			return
		}
		if _, err := f.Write([]byte("b")); err != nil {
			_ = f.Close()
			writeErr <- err
			return
		}
		if err := f.Close(); err != nil {
			writeErr <- err
			return
		}
		lastWriteDone <- time.Now()
	}()

	if err := WaitStable(path, stableFor, pollInterval); err != nil {
		t.Fatalf("WaitStable() error = %v", err)
	}

	returnedAt := time.Now()
	var (
		lastWriteAt time.Time
		gotWriteAt  bool
	)
	select {
	case lastWriteAt = <-lastWriteDone:
		gotWriteAt = true
	case err := <-writeErr:
		t.Fatalf("writer goroutine error: %v", err)
	case <-time.After(250 * time.Millisecond):
		t.Fatal("timed out waiting for writer goroutine")
	}

	if !gotWriteAt {
		t.Fatal("writer did not report completion")
	}

	select {
	case err := <-writeErr:
		t.Fatalf("writer goroutine error: %v", err)
	default:
	}

	if returnedAt.Sub(lastWriteAt) < stableFor-15*time.Millisecond {
		t.Fatalf("WaitStable() returned too early: afterLastWrite=%v, stableFor=%v", returnedAt.Sub(lastWriteAt), stableFor)
	}
}
