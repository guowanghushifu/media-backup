package watcher

import (
	"os"
	"path/filepath"
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
	waitForScanStable = func(path string, stableFor time.Duration, pollInterval time.Duration) error {
		waitCalls++
		return nil
	}

	count, err := ScanExistingAndLink(sourceDir, linkDir, []string{".mkv"}, stableDuration)
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

func TestScanExistingWaitsForRecentlyModifiedFiles(t *testing.T) {
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

	var (
		waitCalls       int
		gotPath         string
		gotStableFor    time.Duration
		gotPollInterval time.Duration
	)
	waitForScanStable = func(path string, stableFor time.Duration, pollInterval time.Duration) error {
		waitCalls++
		gotPath = path
		gotStableFor = stableFor
		gotPollInterval = pollInterval
		return nil
	}

	count, err := ScanExistingAndLink(sourceDir, linkDir, []string{".mkv"}, stableDuration)
	if err != nil {
		t.Fatalf("ScanExistingAndLink() error = %v", err)
	}
	if count != 1 {
		t.Fatalf("ScanExistingAndLink() count = %d, want 1", count)
	}
	if waitCalls != 1 {
		t.Fatalf("ScanExistingAndLink() wait calls = %d, want 1", waitCalls)
	}
	expectedPath := filepath.Join(sourceDir, "movie", "feature.mkv")
	if gotPath != expectedPath {
		t.Fatalf("ScanExistingAndLink() wait path = %q, want %q", gotPath, expectedPath)
	}
	if gotStableFor != stableDuration {
		t.Fatalf("ScanExistingAndLink() stable duration = %v, want %v", gotStableFor, stableDuration)
	}
	if gotPollInterval != time.Millisecond {
		t.Fatalf("ScanExistingAndLink() poll interval = %v, want %v", gotPollInterval, time.Millisecond)
	}

	linkedMedia := filepath.Join(linkDir, "movie", "feature.mkv")
	if _, err := os.Stat(linkedMedia); err != nil {
		t.Fatalf("expected linked media file missing at %q: %v", linkedMedia, err)
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
	waitForScanStable = func(path string, stableFor time.Duration, pollInterval time.Duration) error {
		waitCalls++
		gotPath = path
		gotStableFor = stableFor
		gotPollInterval = pollInterval
		return nil
	}

	count, err := ScanAndLink(sourceDir, linkDir, []string{".mkv"}, stableDuration)
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
	if gotPollInterval != time.Millisecond {
		t.Fatalf("ScanAndLink() poll interval = %v, want %v", gotPollInterval, time.Millisecond)
	}

	linkedMedia := filepath.Join(linkDir, "movie", "feature.mkv")
	if _, err := os.Stat(linkedMedia); err != nil {
		t.Fatalf("expected linked media file missing at %q: %v", linkedMedia, err)
	}
}
