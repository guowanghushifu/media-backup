package rclone

import (
	"path/filepath"
	"testing"
)

func TestBuildRemoteDirPreservesNestedRelativePathWithTrailingSlash(t *testing.T) {
	t.Parallel()

	sourceDir := filepath.Join(string(filepath.Separator), "library")
	sourceFile := filepath.Join(sourceDir, "Movie", "Collection", "feature.mkv")

	got, err := BuildRemoteDir(sourceDir, "gd1:/sync", sourceFile)
	if err != nil {
		t.Fatalf("BuildRemoteDir() error = %v", err)
	}

	want := "gd1:/sync/Movie/Collection/"
	if got != want {
		t.Fatalf("BuildRemoteDir() = %q, want %q", got, want)
	}
}

func TestBuildRemoteDirKeepsRootRemoteForTopLevelFile(t *testing.T) {
	t.Parallel()

	sourceDir := filepath.Join(string(filepath.Separator), "library")
	sourceFile := filepath.Join(sourceDir, "feature.mkv")

	got, err := BuildRemoteDir(sourceDir, "gd1:/sync", sourceFile)
	if err != nil {
		t.Fatalf("BuildRemoteDir() error = %v", err)
	}

	want := "gd1:/sync/"
	if got != want {
		t.Fatalf("BuildRemoteDir() = %q, want %q", got, want)
	}
}

func TestBuildRemoteDirKeepsBareRootRemoteForTopLevelFile(t *testing.T) {
	t.Parallel()

	sourceDir := filepath.Join(string(filepath.Separator), "library")
	sourceFile := filepath.Join(sourceDir, "feature.mkv")

	got, err := BuildRemoteDir(sourceDir, "remote:", sourceFile)
	if err != nil {
		t.Fatalf("BuildRemoteDir() error = %v", err)
	}

	want := "remote:"
	if got != want {
		t.Fatalf("BuildRemoteDir() = %q, want %q", got, want)
	}
}

func TestBuildRemoteDirKeepsBareRootRemoteForNestedFile(t *testing.T) {
	t.Parallel()

	sourceDir := filepath.Join(string(filepath.Separator), "library")
	sourceFile := filepath.Join(sourceDir, "Movie", "Collection", "feature.mkv")

	got, err := BuildRemoteDir(sourceDir, "remote:", sourceFile)
	if err != nil {
		t.Fatalf("BuildRemoteDir() error = %v", err)
	}

	want := "remote:Movie/Collection/"
	if got != want {
		t.Fatalf("BuildRemoteDir() = %q, want %q", got, want)
	}
}

func TestBuildRemoteDirRejectsSourceFileOutsideSourceDir(t *testing.T) {
	t.Parallel()

	sourceDir := filepath.Join(string(filepath.Separator), "library")
	sourceFile := filepath.Join(string(filepath.Separator), "other", "feature.mkv")

	_, err := BuildRemoteDir(sourceDir, "gd1:/sync", sourceFile)
	if err == nil {
		t.Fatal("BuildRemoteDir() error = nil, want non-nil for outside source path")
	}
}
