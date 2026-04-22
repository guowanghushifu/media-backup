package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestLoadConfigAppliesDefaults(t *testing.T) {
	t.Parallel()

	path := writeTempConfig(t, `
jobs:
  - name: movies
    source_dir: /media/movies
    link_dir: /links/movies
    rclone_remote: remote:movies
`)

	cfg, err := LoadConfig(path)
	if err != nil {
		t.Fatalf("LoadConfig returned error: %v", err)
	}

	if cfg.PollInterval != time.Second {
		t.Fatalf("PollInterval = %v, want %v", cfg.PollInterval, time.Second)
	}
	if cfg.StableDuration != time.Minute {
		t.Fatalf("StableDuration = %v, want %v", cfg.StableDuration, time.Minute)
	}
	if cfg.RetryInterval != 10*time.Minute {
		t.Fatalf("RetryInterval = %v, want %v", cfg.RetryInterval, 10*time.Minute)
	}
	if cfg.MaxParallelUploads != 5 {
		t.Fatalf("MaxParallelUploads = %d, want 5", cfg.MaxParallelUploads)
	}

	wantExtensions := []string{".mkv", ".mp4", ".m2ts", ".ts"}
	assertEqualStringSlice(t, "Extensions", cfg.Extensions, wantExtensions)

	wantRcloneArgs := []string{
		"--drive-chunk-size=256M",
		"--checkers=5",
		"--transfers=5",
		"--drive-stop-on-upload-limit",
		"--stats=1s",
		"--stats-one-line",
		"-v",
	}
	assertEqualStringSlice(t, "RcloneArgs", cfg.RcloneArgs, wantRcloneArgs)
}

func TestLoadConfigRejectsDuplicateSourceDir(t *testing.T) {
	t.Parallel()

	path := writeTempConfig(t, `
poll_interval: 2s
stable_duration: 45s
retry_interval: 5m
max_parallel_uploads: 2
jobs:
  - name: job-a
    source_dir: /media/shared
    link_dir: /links/a
    rclone_remote: remote:a
  - name: job-b
    source_dir: /media/shared
    link_dir: /links/b
    rclone_remote: remote:b
`)

	_, err := LoadConfig(path)
	if err == nil {
		t.Fatal("LoadConfig error = nil, want duplicate source_dir error")
	}
	if !strings.Contains(err.Error(), "duplicate source_dir") {
		t.Fatalf("error = %q, want substring %q", err.Error(), "duplicate source_dir")
	}
}

func TestLoadConfigRejectsDuplicateSourceDirWithSurroundingWhitespace(t *testing.T) {
	t.Parallel()

	path := writeTempConfig(t, `
jobs:
  - name: job-a
    source_dir: /media/shared
    link_dir: /links/a
    rclone_remote: remote:a
  - name: job-b
    source_dir: "  /media/shared  "
    link_dir: /links/b
    rclone_remote: remote:b
`)

	_, err := LoadConfig(path)
	if err == nil {
		t.Fatal("LoadConfig error = nil, want duplicate source_dir error")
	}
	if !strings.Contains(err.Error(), "duplicate source_dir") {
		t.Fatalf("error = %q, want substring %q", err.Error(), "duplicate source_dir")
	}
}

func TestLoadConfigNormalizesExtensionsToLowercase(t *testing.T) {
	t.Parallel()

	path := writeTempConfig(t, `
extensions:
  - .MKV
  - .Mp4
jobs:
  - name: movies
    source_dir: /media/movies
    link_dir: /links/movies
    rclone_remote: remote:movies
`)

	cfg, err := LoadConfig(path)
	if err != nil {
		t.Fatalf("LoadConfig returned error: %v", err)
	}

	want := []string{".mkv", ".mp4"}
	assertEqualStringSlice(t, "Extensions", cfg.Extensions, want)
}

func TestLoadConfigRejectsDuplicateLinkDir(t *testing.T) {
	t.Parallel()

	path := writeTempConfig(t, `
jobs:
  - name: job-a
    source_dir: /media/a
    link_dir: /links/shared
    rclone_remote: remote:a
  - name: job-b
    source_dir: /media/b
    link_dir: /links/shared
    rclone_remote: remote:b
`)

	_, err := LoadConfig(path)
	if err == nil {
		t.Fatal("LoadConfig error = nil, want duplicate link_dir error")
	}
	if !strings.Contains(err.Error(), "duplicate link_dir") {
		t.Fatalf("error = %q, want substring %q", err.Error(), "duplicate link_dir")
	}
}

func TestLoadConfigRejectsDuplicateLinkDirWithSurroundingWhitespace(t *testing.T) {
	t.Parallel()

	path := writeTempConfig(t, `
jobs:
  - name: job-a
    source_dir: /media/a
    link_dir: /links/shared
    rclone_remote: remote:a
  - name: job-b
    source_dir: /media/b
    link_dir: "  /links/shared  "
    rclone_remote: remote:b
`)

	_, err := LoadConfig(path)
	if err == nil {
		t.Fatal("LoadConfig error = nil, want duplicate link_dir error")
	}
	if !strings.Contains(err.Error(), "duplicate link_dir") {
		t.Fatalf("error = %q, want substring %q", err.Error(), "duplicate link_dir")
	}
}

func TestLoadConfigRejectsNoJobs(t *testing.T) {
	t.Parallel()

	path := writeTempConfig(t, `
poll_interval: 1s
`)

	_, err := LoadConfig(path)
	if err == nil {
		t.Fatal("LoadConfig error = nil, want no jobs validation error")
	}
	if !strings.Contains(err.Error(), "at least one job") {
		t.Fatalf("error = %q, want substring %q", err.Error(), "at least one job")
	}
}

func TestLoadConfigRejectsJobMissingRequiredField(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		jobYAML   string
		wantError string
	}{
		{
			name: "missing name",
			jobYAML: `
    source_dir: /media/a
    link_dir: /links/a
    rclone_remote: remote:a`,
			wantError: "job name is required",
		},
		{
			name: "missing source_dir",
			jobYAML: `
    name: job-a
    link_dir: /links/a
    rclone_remote: remote:a`,
			wantError: "job source_dir is required",
		},
		{
			name: "missing link_dir",
			jobYAML: `
    name: job-a
    source_dir: /media/a
    rclone_remote: remote:a`,
			wantError: "job link_dir is required",
		},
		{
			name: "missing rclone_remote",
			jobYAML: `
    name: job-a
    source_dir: /media/a
    link_dir: /links/a`,
			wantError: "job rclone_remote is required",
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			path := writeTempConfig(t, `
jobs:
  -`+tc.jobYAML+`
`)

			_, err := LoadConfig(path)
			if err == nil {
				t.Fatalf("LoadConfig error = nil, want %q", tc.wantError)
			}
			if !strings.Contains(err.Error(), tc.wantError) {
				t.Fatalf("error = %q, want substring %q", err.Error(), tc.wantError)
			}
		})
	}
}

func writeTempConfig(t *testing.T, body string) string {
	t.Helper()

	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(path, []byte(strings.TrimSpace(body)+"\n"), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}
	return path
}

func assertEqualStringSlice(t *testing.T, field string, got, want []string) {
	t.Helper()

	if len(got) != len(want) {
		t.Fatalf("%s length = %d, want %d (%v)", field, len(got), len(want), want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("%s[%d] = %q, want %q", field, i, got[i], want[i])
		}
	}
}
