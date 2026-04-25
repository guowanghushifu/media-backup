package watcher

import (
	"context"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"
)

func TestScanLinksMatchingMediaFiles(t *testing.T) {
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

	count, err := ScanAndLink(sourceDir, linkDir, []string{".mkv"}, time.Millisecond)
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

	count, err := ScanAndLink(sourceDir, linkDir, []string{".mkv"}, time.Millisecond)
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

func TestScanExistingSkipsWaitForOldFiles(t *testing.T) {
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
	stableDuration := time.Minute
	oldEnough := time.Now().Add(-(stableDuration + 10*time.Second))
	if err := os.Chtimes(filepath.Join(sourceDir, "movie", "feature.mkv"), oldEnough, oldEnough); err != nil {
		t.Fatal(err)
	}

	originalWaitForScanStable := waitForScanStable
	t.Cleanup(func() {
		waitForScanStable = originalWaitForScanStable
	})

	waitCalls := 0
	waitForScanStable = func(ctx context.Context, path string, stableFor time.Duration, pollInterval time.Duration) error {
		waitCalls++
		return nil
	}

	pollInterval := 750 * time.Millisecond
	count, err := ScanExistingAndLinkContext(context.Background(), sourceDir, linkDir, []string{".mkv"}, stableDuration, pollInterval)
	if err != nil {
		t.Fatalf("ScanExistingAndLink() error = %v", err)
	}
	if count != 1 {
		t.Fatalf("ScanExistingAndLink() count = %d, want 1", count)
	}
	if waitCalls != 0 {
		t.Fatalf("ScanExistingAndLink() wait calls = %d, want 0", waitCalls)
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

func TestScanExistingSkipsRecentlyModifiedFiles(t *testing.T) {
	root := t.TempDir()
	sourceDir := filepath.Join(root, "source")
	linkDir := filepath.Join(root, "link")
	if err := os.MkdirAll(filepath.Join(sourceDir, "movie"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sourceDir, "movie", "feature.mkv"), []byte("data"), 0o644); err != nil {
		t.Fatal(err)
	}

	originalWaitForScanStable := waitForScanStable
	t.Cleanup(func() {
		waitForScanStable = originalWaitForScanStable
	})

	stableDuration := time.Minute
	recent := time.Now().Add(-5 * time.Second)
	if err := os.Chtimes(filepath.Join(sourceDir, "movie", "feature.mkv"), recent, recent); err != nil {
		t.Fatal(err)
	}

	waitCalls := 0
	waitForScanStable = func(ctx context.Context, path string, stableFor time.Duration, pollInterval time.Duration) error {
		waitCalls++
		return nil
	}

	pollInterval := 750 * time.Millisecond
	count, err := ScanExistingAndLinkContext(context.Background(), sourceDir, linkDir, []string{".mkv"}, stableDuration, pollInterval)
	if err != nil {
		t.Fatalf("ScanExistingAndLink() error = %v", err)
	}
	if count != 0 {
		t.Fatalf("ScanExistingAndLink() count = %d, want 0 for recent file", count)
	}
	if waitCalls != 0 {
		t.Fatalf("ScanExistingAndLink() wait calls = %d, want 0", waitCalls)
	}
	linkedMedia := filepath.Join(linkDir, "movie", "feature.mkv")
	if _, err := os.Stat(linkedMedia); !os.IsNotExist(err) {
		t.Fatalf("expected recent file not linked at %q, got err=%v", linkedMedia, err)
	}
}

func TestScanAndLinkStillWaitsForStabilityDuration(t *testing.T) {
	root := t.TempDir()
	sourceDir := filepath.Join(root, "source")
	linkDir := filepath.Join(root, "link")
	if err := os.MkdirAll(filepath.Join(sourceDir, "movie"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sourceDir, "movie", "feature.mkv"), []byte("data"), 0o644); err != nil {
		t.Fatal(err)
	}

	originalWaitForScanStable := waitForScanStable
	t.Cleanup(func() {
		waitForScanStable = originalWaitForScanStable
	})

	stableDuration := 80 * time.Millisecond
	var (
		waitCalls       int
		gotPath         string
		gotStableFor    time.Duration
		gotPollInterval time.Duration
	)
	waitForScanStable = func(ctx context.Context, path string, stableFor time.Duration, pollInterval time.Duration) error {
		waitCalls++
		gotPath = path
		gotStableFor = stableFor
		gotPollInterval = pollInterval
		return nil
	}

	pollInterval := 500 * time.Millisecond
	count, err := ScanAndLinkContext(context.Background(), sourceDir, linkDir, []string{".mkv"}, stableDuration, pollInterval)
	if err != nil {
		t.Fatalf("ScanAndLink() error = %v", err)
	}
	if count != 1 {
		t.Fatalf("ScanAndLink() count = %d, want 1", count)
	}
	if waitCalls != 1 {
		t.Fatalf("ScanAndLink() wait calls = %d, want 1", waitCalls)
	}
	expectedPath := filepath.Join(sourceDir, "movie", "feature.mkv")
	if gotPath != expectedPath {
		t.Fatalf("ScanAndLink() wait path = %q, want %q", gotPath, expectedPath)
	}
	if gotStableFor != stableDuration {
		t.Fatalf("ScanAndLink() stable duration = %v, want %v", gotStableFor, stableDuration)
	}
	if gotPollInterval != pollInterval {
		t.Fatalf("ScanAndLink() poll interval = %v, want %v", gotPollInterval, pollInterval)
	}

	linkedMedia := filepath.Join(linkDir, "movie", "feature.mkv")
	if _, err := os.Stat(linkedMedia); err != nil {
		t.Fatalf("expected linked media file missing at %q: %v", linkedMedia, err)
	}
}

func TestScanLinkedFilesReturnsEachUploadableFilePath(t *testing.T) {
	t.Parallel()

	linkDir := t.TempDir()
	files := []string{
		filepath.Join(linkDir, "movie", "feature.mkv"),
		filepath.Join(linkDir, "show", "season-1", "episode-1.mp4"),
		filepath.Join(linkDir, "show", "season-1", "notes.txt"),
	}
	for _, file := range files {
		if err := os.MkdirAll(filepath.Dir(file), 0o755); err != nil {
			t.Fatalf("MkdirAll(%q): %v", file, err)
		}
		if err := os.WriteFile(file, []byte("x"), 0o644); err != nil {
			t.Fatalf("WriteFile(%q): %v", file, err)
		}
	}

	got, err := ScanLinkedFiles(linkDir, []string{".mkv", ".mp4"})
	if err != nil {
		t.Fatalf("ScanLinkedFiles() error = %v", err)
	}

	sort.Strings(got)
	want := []string{
		filepath.Join(linkDir, "movie", "feature.mkv"),
		filepath.Join(linkDir, "show", "season-1", "episode-1.mp4"),
	}
	if len(got) != len(want) {
		t.Fatalf("ScanLinkedFiles() len = %d, want %d; got=%v", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("ScanLinkedFiles() path[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}
