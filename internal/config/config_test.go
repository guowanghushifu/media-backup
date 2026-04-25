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

func TestLoadConfigDefaultsMaxRetryCountToZero(t *testing.T) {
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
	if cfg.MaxRetryCount != 0 {
		t.Fatalf("MaxRetryCount = %d, want 0", cfg.MaxRetryCount)
	}
}

func TestLoadConfigDefaultsDeleteSourceAfterUploadToFalse(t *testing.T) {
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
	if cfg.Jobs[0].DeleteSourceAfterUpload {
		t.Fatal("DeleteSourceAfterUpload = true, want default false")
	}
}

func TestLoadConfigParsesDeleteSourceAfterUpload(t *testing.T) {
	t.Parallel()

	path := writeTempConfig(t, `
jobs:
  - name: movies
    source_dir: /media/movies
    link_dir: /links/movies
    rclone_remote: remote:movies
    delete_source_after_upload: true
`)

	cfg, err := LoadConfig(path)
	if err != nil {
		t.Fatalf("LoadConfig returned error: %v", err)
	}
	if !cfg.Jobs[0].DeleteSourceAfterUpload {
		t.Fatal("DeleteSourceAfterUpload = false, want true")
	}
}

func TestLoadConfigRejectsNegativeMaxRetryCount(t *testing.T) {
	t.Parallel()

	path := writeTempConfig(t, `
max_retry_count: -1
jobs:
  - name: movies
    source_dir: /media/movies
    link_dir: /links/movies
    rclone_remote: remote:movies
`)

	_, err := LoadConfig(path)
	if err == nil {
		t.Fatal("LoadConfig error = nil, want max_retry_count validation error")
	}
	if !strings.Contains(err.Error(), "max_retry_count must be greater than or equal to 0") {
		t.Fatalf("error = %q, want substring %q", err.Error(), "max_retry_count must be greater than or equal to 0")
	}
}

func TestLoadConfigAcceptsDisabledTelegramWithoutCredentials(t *testing.T) {
	t.Parallel()

	path := writeTempConfig(t, `
max_retry_count: 5
telegram:
  enabled: false
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
	if cfg.Telegram.Enabled {
		t.Fatal("Telegram.Enabled = true, want false")
	}
}

func TestLoadConfigRejectsEnabledTelegramMissingBotToken(t *testing.T) {
	t.Parallel()

	path := writeTempConfig(t, `
telegram:
  enabled: true
  chat_id: -1001234567890
jobs:
  - name: movies
    source_dir: /media/movies
    link_dir: /links/movies
    rclone_remote: remote:movies
`)

	_, err := LoadConfig(path)
	if err == nil {
		t.Fatal("LoadConfig error = nil, want telegram bot token validation error")
	}
	if !strings.Contains(err.Error(), "telegram bot_token is required") {
		t.Fatalf("error = %q, want substring %q", err.Error(), "telegram bot_token is required")
	}
}

func TestLoadConfigRejectsEnabledTelegramWhitespaceOnlyBotToken(t *testing.T) {
	t.Parallel()

	path := writeTempConfig(t, `
telegram:
  enabled: true
  bot_token: "   "
  chat_id: -1001234567890
jobs:
  - name: movies
    source_dir: /media/movies
    link_dir: /links/movies
    rclone_remote: remote:movies
`)

	_, err := LoadConfig(path)
	if err == nil {
		t.Fatal("LoadConfig error = nil, want telegram bot token validation error")
	}
	if !strings.Contains(err.Error(), "telegram bot_token is required") {
		t.Fatalf("error = %q, want substring %q", err.Error(), "telegram bot_token is required")
	}
}

func TestLoadConfigRejectsEnabledTelegramMissingChatID(t *testing.T) {
	t.Parallel()

	path := writeTempConfig(t, `
telegram:
  enabled: true
  bot_token: 123456:ABCDEF
jobs:
  - name: movies
    source_dir: /media/movies
    link_dir: /links/movies
    rclone_remote: remote:movies
`)

	_, err := LoadConfig(path)
	if err == nil {
		t.Fatal("LoadConfig error = nil, want telegram chat id validation error")
	}
	if !strings.Contains(err.Error(), "telegram chat_id is required") {
		t.Fatalf("error = %q, want substring %q", err.Error(), "telegram chat_id is required")
	}
}

func TestLoadConfigRejectsEnabledTelegramWhitespaceOnlyChatID(t *testing.T) {
	t.Parallel()

	path := writeTempConfig(t, `
telegram:
  enabled: true
  bot_token: 123456:ABCDEF
  chat_id: "   "
jobs:
  - name: movies
    source_dir: /media/movies
    link_dir: /links/movies
    rclone_remote: remote:movies
`)

	_, err := LoadConfig(path)
	if err == nil {
		t.Fatal("LoadConfig error = nil, want telegram chat id validation error")
	}
	if !strings.Contains(err.Error(), "telegram chat_id is required") {
		t.Fatalf("error = %q, want substring %q", err.Error(), "telegram chat_id is required")
	}
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

func TestLoadConfigAllowsEmptyLinkDirForDirectUpload(t *testing.T) {
	t.Parallel()

	path := writeTempConfig(t, `
jobs:
  - name: movies
    source_dir: /media/movies
    link_dir: ""
    rclone_remote: remote:movies
`)

	cfg, err := LoadConfig(path)
	if err != nil {
		t.Fatalf("LoadConfig returned error: %v", err)
	}
	if cfg.Jobs[0].LinkDir != "" {
		t.Fatalf("LinkDir = %q, want empty string", cfg.Jobs[0].LinkDir)
	}
}

func TestLoadConfigAllowsLinkDirEqualSourceDirForDirectUpload(t *testing.T) {
	t.Parallel()

	path := writeTempConfig(t, `
jobs:
  - name: movies
    source_dir: /media/movies
    link_dir: /media/movies
    rclone_remote: remote:movies
`)

	if _, err := LoadConfig(path); err != nil {
		t.Fatalf("LoadConfig returned error: %v", err)
	}
}

func TestLoadConfigRejectsLinkDirInsideSourceDir(t *testing.T) {
	t.Parallel()

	path := writeTempConfig(t, `
jobs:
  - name: movies
    source_dir: /media/movies
    link_dir: /media/movies/.upload-buffer
    rclone_remote: remote:movies
`)

	_, err := LoadConfig(path)
	if err == nil {
		t.Fatal("LoadConfig error = nil, want nested link_dir validation error")
	}
	if !strings.Contains(err.Error(), "link_dir must not be inside source_dir") {
		t.Fatalf("error = %q, want nested link_dir validation error", err.Error())
	}
}

func TestLoadConfigRejectsCrossNestedSourceAndLinkDirs(t *testing.T) {
	t.Parallel()

	path := writeTempConfig(t, `
jobs:
  - name: movies
    source_dir: /media/movies
    link_dir: /buffer/movies
    rclone_remote: remote:movies
  - name: remux
    source_dir: /buffer
    link_dir: ""
    rclone_remote: remote:remux
`)

	_, err := LoadConfig(path)
	if err == nil {
		t.Fatal("LoadConfig error = nil, want cross nested source/link validation error")
	}
	if !strings.Contains(err.Error(), "source_dir must not contain another job's link_dir") {
		t.Fatalf("error = %q, want cross nested source/link validation error", err.Error())
	}
}

func TestLoadConfigRejectsSourceDirInsideAnotherJobLinkDir(t *testing.T) {
	t.Parallel()

	path := writeTempConfig(t, `
jobs:
  - name: movies
    source_dir: /media/movies
    link_dir: /buffer/movies
    rclone_remote: remote:movies
  - name: remux
    source_dir: /buffer/movies/pending
    link_dir: ""
    rclone_remote: remote:remux
`)

	_, err := LoadConfig(path)
	if err == nil {
		t.Fatal("LoadConfig error = nil, want cross nested source/link validation error")
	}
	if !strings.Contains(err.Error(), "source_dir must not be inside another job's link_dir") {
		t.Fatalf("error = %q, want source-inside-link validation error", err.Error())
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
			wantError: "",
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
			if tc.wantError == "" {
				if err != nil {
					t.Fatalf("LoadConfig returned error: %v", err)
				}
				return
			}
			if err == nil {
				t.Fatalf("LoadConfig error = nil, want %q", tc.wantError)
			}
			if !strings.Contains(err.Error(), tc.wantError) {
				t.Fatalf("error = %q, want substring %q", err.Error(), tc.wantError)
			}
		})
	}
}

func TestLoadConfigParsesProxyConfig(t *testing.T) {
	t.Parallel()

	path := writeTempConfig(t, `
proxy:
  enabled: true
  scheme: http
  host: 127.0.0.1
  port: 7890
  username: demo
  password: s3cret
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

	if !cfg.Proxy.Enabled {
		t.Fatal("Proxy.Enabled = false, want true")
	}
	if cfg.Proxy.Scheme != "http" {
		t.Fatalf("Proxy.Scheme = %q, want %q", cfg.Proxy.Scheme, "http")
	}
	if cfg.Proxy.Host != "127.0.0.1" {
		t.Fatalf("Proxy.Host = %q, want %q", cfg.Proxy.Host, "127.0.0.1")
	}
	if cfg.Proxy.Port != 7890 {
		t.Fatalf("Proxy.Port = %d, want %d", cfg.Proxy.Port, 7890)
	}
	if cfg.Proxy.Username != "demo" {
		t.Fatalf("Proxy.Username = %q, want %q", cfg.Proxy.Username, "demo")
	}
	if cfg.Proxy.Password != "s3cret" {
		t.Fatalf("Proxy.Password = %q, want %q", cfg.Proxy.Password, "s3cret")
	}
}

func TestLoadConfigAllowsDisabledProxy(t *testing.T) {
	t.Parallel()

	path := writeTempConfig(t, `
proxy:
  enabled: false
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
	if cfg.Proxy.Enabled {
		t.Fatal("Proxy.Enabled = true, want false")
	}
}

func TestLoadConfigRejectsInvalidProxyConfig(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		proxyYAML string
		wantError string
	}{
		{
			name: "missing host",
			proxyYAML: `
proxy:
  enabled: true
  scheme: http
  port: 7890`,
			wantError: "proxy host is required",
		},
		{
			name: "missing port",
			proxyYAML: `
proxy:
  enabled: true
  scheme: http
  host: 127.0.0.1`,
			wantError: "proxy port must be greater than 0",
		},
		{
			name: "unsupported scheme",
			proxyYAML: `
proxy:
  enabled: true
  scheme: socks5
  host: 127.0.0.1
  port: 7890`,
			wantError: "proxy scheme must be http or https",
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			path := writeTempConfig(t, tc.proxyYAML+`
jobs:
  - name: movies
    source_dir: /media/movies
    link_dir: /links/movies
    rclone_remote: remote:movies
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
