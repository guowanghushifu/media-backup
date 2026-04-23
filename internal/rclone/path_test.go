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
