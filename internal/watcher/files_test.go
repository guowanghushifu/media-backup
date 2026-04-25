package watcher

import (
	"context"
	"errors"
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

	result, err := LinkFile(sourceDir, linkDir, sourcePath)
	if err != nil {
		t.Fatalf("LinkFile() error = %v", err)
	}
	if result.State != LinkCreated {
		t.Fatalf("LinkFile() state = %v, want %v", result.State, LinkCreated)
	}
	linkPath := result.Path

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

	firstResult, err := LinkFile(sourceDir, linkDir, sourcePath)
	if err != nil {
		t.Fatalf("first LinkFile() error = %v", err)
	}
	if firstResult.State != LinkCreated {
		t.Fatalf("first LinkFile() state = %v, want %v", firstResult.State, LinkCreated)
	}
	firstLinkPath := firstResult.Path
	secondResult, err := LinkFile(sourceDir, linkDir, sourcePath)
	if err != nil {
		t.Fatalf("second LinkFile() error = %v", err)
	}
	if secondResult.State != LinkAlreadySameFile {
		t.Fatalf("second LinkFile() state = %v, want %v", secondResult.State, LinkAlreadySameFile)
	}
	secondLinkPath := secondResult.Path
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

func TestLinkFileReplacesStaleLinkWhenSourcePathGetsNewInode(t *testing.T) {
	t.Parallel()

	sourceDir := t.TempDir()
	linkDir := t.TempDir()
	sourcePath := filepath.Join(sourceDir, "movie.mkv")
	if err := os.WriteFile(sourcePath, []byte("old-bytes"), 0o644); err != nil {
		t.Fatalf("WriteFile original source: %v", err)
	}

	result, err := LinkFile(sourceDir, linkDir, sourcePath)
	if err != nil {
		t.Fatalf("first LinkFile() error = %v", err)
	}
	if result.State != LinkCreated {
		t.Fatalf("first LinkFile() state = %v, want %v", result.State, LinkCreated)
	}
	linkPath := result.Path

	if err := os.Remove(sourcePath); err != nil {
		t.Fatalf("Remove original source: %v", err)
	}
	if err := os.WriteFile(sourcePath, []byte("new-bytes"), 0o644); err != nil {
		t.Fatalf("WriteFile replacement source: %v", err)
	}

	relinkedResult, err := LinkFile(sourceDir, linkDir, sourcePath)
	if err != nil {
		t.Fatalf("second LinkFile() error = %v", err)
	}
	if relinkedResult.State != LinkReplacedDifferentFile {
		t.Fatalf("second LinkFile() state = %v, want %v", relinkedResult.State, LinkReplacedDifferentFile)
	}
	relinkedPath := relinkedResult.Path
	if relinkedPath != linkPath {
		t.Fatalf("second LinkFile() path = %q, want %q", relinkedPath, linkPath)
	}

	gotBytes, err := os.ReadFile(linkPath)
	if err != nil {
		t.Fatalf("ReadFile link: %v", err)
	}
	if string(gotBytes) != "new-bytes" {
		t.Fatalf("linked file content = %q, want %q", string(gotBytes), "new-bytes")
	}

	sourceInfo, err := os.Stat(sourcePath)
	if err != nil {
		t.Fatalf("Stat replacement source: %v", err)
	}
	linkInfo, err := os.Stat(linkPath)
	if err != nil {
		t.Fatalf("Stat relinked path: %v", err)
	}
	if !os.SameFile(sourceInfo, linkInfo) {
		t.Fatal("relinked target is not a hard link to replacement source")
	}
}

func TestLinkFileUsesSourceFileWhenLinkDirIsEmpty(t *testing.T) {
	t.Parallel()

	sourceDir := t.TempDir()
	sourcePath := filepath.Join(sourceDir, "movie.mkv")
	if err := os.WriteFile(sourcePath, []byte("video"), 0o644); err != nil {
		t.Fatalf("WriteFile source: %v", err)
	}

	result, err := LinkFile(sourceDir, "", sourcePath)
	if err != nil {
		t.Fatalf("LinkFile() error = %v", err)
	}
	if result.Path != sourcePath {
		t.Fatalf("LinkFile() path = %q, want source path %q", result.Path, sourcePath)
	}
	if result.State != LinkAlreadySameFile {
		t.Fatalf("LinkFile() state = %v, want %v", result.State, LinkAlreadySameFile)
	}
}

func TestLinkFileUsesSourceFileWhenLinkDirEqualsSourceDir(t *testing.T) {
	t.Parallel()

	sourceDir := t.TempDir()
	sourcePath := filepath.Join(sourceDir, "movie.mkv")
	if err := os.WriteFile(sourcePath, []byte("video"), 0o644); err != nil {
		t.Fatalf("WriteFile source: %v", err)
	}

	result, err := LinkFile(sourceDir, sourceDir, sourcePath)
	if err != nil {
		t.Fatalf("LinkFile() error = %v", err)
	}
	if result.Path != sourcePath {
		t.Fatalf("LinkFile() path = %q, want source path %q", result.Path, sourcePath)
	}
	if result.State != LinkAlreadySameFile {
		t.Fatalf("LinkFile() state = %v, want %v", result.State, LinkAlreadySameFile)
	}
}

func TestLinkFileRejectsSymlinkSource(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	sourceDir := filepath.Join(root, "source")
	linkDir := filepath.Join(root, "link")
	targetPath := filepath.Join(root, "outside.mkv")
	symlinkPath := filepath.Join(sourceDir, "movie.mkv")
	if err := os.MkdirAll(sourceDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(targetPath, []byte("outside"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(targetPath, symlinkPath); err != nil {
		t.Fatal(err)
	}

	_, err := LinkFile(sourceDir, linkDir, symlinkPath)
	if !errors.Is(err, ErrNonRegularFile) {
		t.Fatalf("LinkFile() error = %v, want ErrNonRegularFile", err)
	}
}

func TestLinkFileRejectsExistingSymlinkTarget(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	sourceDir := filepath.Join(root, "source")
	linkDir := filepath.Join(root, "link")
	sourcePath := filepath.Join(sourceDir, "movie.mkv")
	outsidePath := filepath.Join(root, "outside.mkv")
	linkPath := filepath.Join(linkDir, "movie.mkv")
	if err := os.MkdirAll(sourceDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(linkDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(sourcePath, []byte("source"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(outsidePath, []byte("outside"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outsidePath, linkPath); err != nil {
		t.Fatal(err)
	}

	_, err := LinkFile(sourceDir, linkDir, sourcePath)
	if !errors.Is(err, ErrNonRegularFile) {
		t.Fatalf("LinkFile() error = %v, want ErrNonRegularFile", err)
	}
	info, err := os.Lstat(linkPath)
	if err != nil {
		t.Fatalf("Lstat link target: %v", err)
	}
	if info.Mode()&os.ModeSymlink == 0 {
		t.Fatal("existing symlink target was replaced")
	}
}

func TestCleanupLinkedFileRemovesOnlyUploadedFileAndEmptyParents(t *testing.T) {
	t.Parallel()

	linkDir := t.TempDir()
	uploadedFile := filepath.Join(linkDir, "show", "season-1", "episode-1.mkv")
	keptFile := filepath.Join(linkDir, "show", "season-2", "episode-1.mkv")
	rootFile := filepath.Join(linkDir, "keep.txt")

	for _, file := range []string{uploadedFile, keptFile, rootFile} {
		if err := os.MkdirAll(filepath.Dir(file), 0o755); err != nil {
			t.Fatalf("MkdirAll(%q): %v", file, err)
		}
		if err := os.WriteFile(file, []byte("x"), 0o644); err != nil {
			t.Fatalf("WriteFile(%q): %v", file, err)
		}
	}

	if err := CleanupLinkedFile(linkDir, uploadedFile); err != nil {
		t.Fatalf("CleanupLinkedFile() error = %v", err)
	}

	if _, err := os.Stat(uploadedFile); !os.IsNotExist(err) {
		t.Fatalf("uploaded file still exists: err=%v", err)
	}
	if _, err := os.Stat(filepath.Dir(uploadedFile)); !os.IsNotExist(err) {
		t.Fatalf("empty parent dir still exists: err=%v", err)
	}
	if _, err := os.Stat(filepath.Dir(filepath.Dir(uploadedFile))); err != nil {
		t.Fatalf("non-empty ancestor dir was removed: %v", err)
	}

	if _, err := os.Stat(keptFile); err != nil {
		t.Fatalf("kept file missing: %v", err)
	}
	if _, err := os.Stat(filepath.Dir(keptFile)); err != nil {
		t.Fatalf("non-empty parent dir was removed: %v", err)
	}
	if _, err := os.Stat(linkDir); err != nil {
		t.Fatalf("link root missing: %v", err)
	}
}

func TestCleanupLinkedFileRejectsPathOutsideLinkDirWithoutDeleting(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	linkDir := filepath.Join(root, "links")
	externalDir := filepath.Join(root, "external")
	if err := os.MkdirAll(linkDir, 0o755); err != nil {
		t.Fatalf("MkdirAll link dir: %v", err)
	}
	if err := os.MkdirAll(externalDir, 0o755); err != nil {
		t.Fatalf("MkdirAll external dir: %v", err)
	}
	externalFile := filepath.Join(externalDir, "episode-1.mkv")
	if err := os.WriteFile(externalFile, []byte("x"), 0o644); err != nil {
		t.Fatalf("WriteFile external: %v", err)
	}

	if err := CleanupLinkedFile(linkDir, externalFile); err == nil {
		t.Fatal("CleanupLinkedFile() error = nil, want non-nil for outside path")
	}

	if _, err := os.Stat(externalFile); err != nil {
		t.Fatalf("external file was deleted unexpectedly: %v", err)
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

func TestWaitStableContextReturnsOnCancel(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "growing.file")
	if err := os.WriteFile(path, []byte("x"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := WaitStableContext(ctx, path, time.Hour, time.Hour)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("WaitStableContext() error = %v, want context.Canceled", err)
	}
}

func TestWaitStableContextRejectsSymlink(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	targetPath := filepath.Join(root, "target.mkv")
	symlinkPath := filepath.Join(root, "link.mkv")
	if err := os.WriteFile(targetPath, []byte("video"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(targetPath, symlinkPath); err != nil {
		t.Fatal(err)
	}

	err := WaitStableContext(context.Background(), symlinkPath, time.Millisecond, time.Millisecond)
	if !errors.Is(err, ErrNonRegularFile) {
		t.Fatalf("WaitStableContext() error = %v, want ErrNonRegularFile", err)
	}
}

func TestWaitStableContextReturnsAfterMaxWait(t *testing.T) {
	originalMaxWait := stableWaitMax
	stableWaitMax = 70 * time.Millisecond
	t.Cleanup(func() { stableWaitMax = originalMaxWait })

	dir := t.TempDir()
	path := filepath.Join(dir, "growing.file")
	if err := os.WriteFile(path, []byte("x"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	stop := make(chan struct{})
	done := make(chan struct{})
	go func() {
		defer close(done)
		ticker := time.NewTicker(10 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-stop:
				return
			case <-ticker.C:
				f, err := os.OpenFile(path, os.O_WRONLY|os.O_APPEND, 0)
				if err == nil {
					_, _ = f.Write([]byte("x"))
					_ = f.Close()
				}
			}
		}
	}()
	defer func() {
		close(stop)
		<-done
	}()

	stableFor := 40 * time.Millisecond
	start := time.Now()
	err := WaitStableContext(context.Background(), path, stableFor, 5*time.Millisecond)
	if !errors.Is(err, ErrWaitStableTimeout) {
		t.Fatalf("WaitStableContext() error = %v, want ErrWaitStableTimeout", err)
	}
	if elapsed := time.Since(start); elapsed < stableWaitMax {
		t.Fatalf("WaitStableContext() returned after %v, want at least %v", elapsed, stableWaitMax)
	}
}
