package main

import (
	"bytes"
	"errors"
	"log"
	"os"
	"testing"
	"time"
)

func TestResolveConfigPathUsesFlagValue(t *testing.T) {
	t.Parallel()

	got, err := resolveConfigPath("/custom/config.yaml", func() (string, error) {
		return "/ignored/binary", nil
	}, func(string) (os.FileInfo, error) {
		t.Fatal("stat should not be called when flag value is provided")
		return nil, nil
	})
	if err != nil {
		t.Fatalf("resolveConfigPath() error = %v", err)
	}
	if got != "/custom/config.yaml" {
		t.Fatalf("resolveConfigPath() = %q, want %q", got, "/custom/config.yaml")
	}
}

func TestResolveConfigPathPrefersExecutableDirectoryConfig(t *testing.T) {
	t.Parallel()

	got, err := resolveConfigPath("", func() (string, error) {
		return "/opt/media-backup/media-backup", nil
	}, func(path string) (os.FileInfo, error) {
		if path != "/opt/media-backup/config.yaml" {
			t.Fatalf("stat path = %q, want %q", path, "/opt/media-backup/config.yaml")
		}
		return fakeFileInfo{}, nil
	})
	if err != nil {
		t.Fatalf("resolveConfigPath() error = %v", err)
	}
	if got != "/opt/media-backup/config.yaml" {
		t.Fatalf("resolveConfigPath() = %q, want %q", got, "/opt/media-backup/config.yaml")
	}
}

func TestResolveConfigPathReturnsNotFoundWhenExecutableDirectoryConfigMissing(t *testing.T) {
	t.Parallel()

	_, err := resolveConfigPath("", func() (string, error) {
		return "/opt/media-backup/media-backup", nil
	}, func(path string) (os.FileInfo, error) {
		if path != "/opt/media-backup/config.yaml" {
			t.Fatalf("stat path = %q, want %q", path, "/opt/media-backup/config.yaml")
		}
		return nil, os.ErrNotExist
	})
	if err == nil {
		t.Fatal("resolveConfigPath() error = nil, want not found error")
	}
}

func TestResolveConfigPathReturnsExecutableError(t *testing.T) {
	t.Parallel()

	wantErr := errors.New("boom")
	_, err := resolveConfigPath("", func() (string, error) {
		return "", wantErr
	}, func(string) (os.FileInfo, error) {
		t.Fatal("stat should not be called when executable lookup fails")
		return nil, nil
	})
	if !errors.Is(err, wantErr) {
		t.Fatalf("resolveConfigPath() error = %v, want %v", err, wantErr)
	}
}

func TestResolveConfigPathReturnsStatError(t *testing.T) {
	t.Parallel()

	wantErr := errors.New("stat failed")
	_, err := resolveConfigPath("", func() (string, error) {
		return "/opt/media-backup/media-backup", nil
	}, func(string) (os.FileInfo, error) {
		return nil, wantErr
	})
	if !errors.Is(err, wantErr) {
		t.Fatalf("resolveConfigPath() error = %v, want %v", err, wantErr)
	}
}

func TestResolveLogDirUsesExecutableDirectory(t *testing.T) {
	t.Parallel()

	got, err := resolveLogDir(func() (string, error) {
		return "/opt/media-backup/media-backup", nil
	})
	if err != nil {
		t.Fatalf("resolveLogDir() error = %v", err)
	}
	if got != "/opt/media-backup/logs" {
		t.Fatalf("resolveLogDir() = %q, want %q", got, "/opt/media-backup/logs")
	}
}

func TestResolveLogDirReturnsExecutableError(t *testing.T) {
	t.Parallel()

	wantErr := errors.New("boom")
	_, err := resolveLogDir(func() (string, error) {
		return "", wantErr
	})
	if !errors.Is(err, wantErr) {
		t.Fatalf("resolveLogDir() error = %v, want %v", err, wantErr)
	}
}

func TestLogConfigPathWritesResolvedPath(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	logger := log.New(&buf, "", 0)

	logConfigPath(logger, "/opt/media-backup/config.yaml")

	got := buf.String()
	want := "using config file: /opt/media-backup/config.yaml\n"
	if got != want {
		t.Fatalf("log output = %q, want %q", got, want)
	}
}

type fakeFileInfo struct{}

func (fakeFileInfo) Name() string       { return "config.yaml" }
func (fakeFileInfo) Size() int64        { return 0 }
func (fakeFileInfo) Mode() os.FileMode  { return 0 }
func (fakeFileInfo) ModTime() time.Time { return time.Time{} }
func (fakeFileInfo) IsDir() bool        { return false }
func (fakeFileInfo) Sys() any           { return nil }
