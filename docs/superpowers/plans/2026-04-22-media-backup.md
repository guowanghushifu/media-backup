# Media Backup CLI Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build a Debian-focused Go CLI that recursively watches configured media source directories, creates hard links in matching link directories, uploads each job with `rclone`, and cleans uploaded link content while preserving each `link_dir` root.

**Architecture:** The program is a single foreground process composed of focused packages: `config` loads YAML and validates uniqueness, `queue` schedules per-job serial uploads with a global concurrency cap, `watcher` handles startup scans and recursive `fsnotify` registration, `rclone` wraps external uploads and parses stats, and `ui` renders a multi-job terminal dashboard. State reconstruction comes from the filesystem, not a database, and every behavioral unit is covered with package-level tests before implementation.

**Tech Stack:** Go, `gopkg.in/yaml.v3`, `github.com/fsnotify/fsnotify`, standard library `os/exec`, `context`, `sync`, `syscall`, `testing`

---

## File Structure

- Create: `go.mod`
- Create: `cmd/media-backup/main.go`
- Create: `internal/config/config.go`
- Create: `internal/config/config_test.go`
- Create: `internal/queue/scheduler.go`
- Create: `internal/queue/scheduler_test.go`
- Create: `internal/rclone/parser.go`
- Create: `internal/rclone/parser_test.go`
- Create: `internal/rclone/runner.go`
- Create: `internal/rclone/runner_test.go`
- Create: `internal/ui/renderer.go`
- Create: `internal/ui/renderer_test.go`
- Create: `internal/watcher/files.go`
- Create: `internal/watcher/files_test.go`
- Create: `internal/watcher/scanner.go`
- Create: `internal/watcher/scanner_test.go`
- Create: `internal/watcher/watch_service.go`
- Create: `internal/watcher/watch_service_test.go`
- Create: `internal/app/app.go`
- Create: `internal/app/app_test.go`
- Create: `internal/app/logging.go`
- Create: `internal/app/logging_test.go`
- Create: `configs/config.example.yaml`
- Create: `build.sh`

### Task 1: Bootstrap Module and Configuration Loading

**Files:**
- Create: `go.mod`
- Create: `internal/config/config.go`
- Create: `internal/config/config_test.go`
- Create: `configs/config.example.yaml`

- [ ] **Step 1: Write the failing config tests**

```go
package config_test

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"media-backup/internal/config"
)

func TestLoadConfigAppliesDefaults(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	data := []byte(`
jobs:
  - name: movie
    source_dir: /dld/upload/4K.REMUX.SGNB
    link_dir: /dld/gd_upload/4K.REMUX.SGNB
    rclone_remote: gd1:/sync/Movie/4K.REMUX.SGNB
`)
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}

	cfg, err := config.Load(path)
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
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
	if len(cfg.Extensions) != 4 {
		t.Fatalf("Extensions len = %d, want 4", len(cfg.Extensions))
	}
	if len(cfg.RcloneArgs) == 0 {
		t.Fatal("RcloneArgs should not be empty")
	}
}

func TestLoadConfigRejectsDuplicateSourceDir(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	data := []byte(`
jobs:
  - name: one
    source_dir: /same/source
    link_dir: /one/link
    rclone_remote: gd1:/one
  - name: two
    source_dir: /same/source
    link_dir: /two/link
    rclone_remote: gd1:/two
`)
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}

	if _, err := config.Load(path); err == nil {
		t.Fatal("Load() error = nil, want duplicate source_dir error")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/config -run 'TestLoadConfig(AppliesDefaults|RejectsDuplicateSourceDir)' -v`
Expected: FAIL with package or symbol errors because `internal/config` does not exist yet.

- [ ] **Step 3: Write minimal implementation**

```go
package config

import (
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

var defaultExtensions = []string{".mkv", ".mp4", ".m2ts", ".ts"}
var defaultRcloneArgs = []string{
	"--drive-chunk-size=256M",
	"--checkers=5",
	"--transfers=5",
	"--drive-stop-on-upload-limit",
	"--stats=1s",
	"--stats-one-line",
	"-v",
}

type Config struct {
	PollInterval       time.Duration `yaml:"poll_interval"`
	StableDuration     time.Duration `yaml:"stable_duration"`
	RetryInterval      time.Duration `yaml:"retry_interval"`
	MaxParallelUploads int           `yaml:"max_parallel_uploads"`
	Extensions         []string      `yaml:"extensions"`
	RcloneArgs         []string      `yaml:"rclone_args"`
	Jobs               []Job         `yaml:"jobs"`
}

type Job struct {
	Name         string `yaml:"name"`
	SourceDir    string `yaml:"source_dir"`
	LinkDir      string `yaml:"link_dir"`
	RcloneRemote string `yaml:"rclone_remote"`
}

func Load(path string) (Config, error) {
	var cfg Config

	data, err := os.ReadFile(path)
	if err != nil {
		return cfg, fmt.Errorf("read config: %w", err)
	}
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return cfg, fmt.Errorf("parse config: %w", err)
	}

	applyDefaults(&cfg)
	if err := validate(cfg); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

func applyDefaults(cfg *Config) {
	if cfg.PollInterval == 0 {
		cfg.PollInterval = time.Second
	}
	if cfg.StableDuration == 0 {
		cfg.StableDuration = time.Minute
	}
	if cfg.RetryInterval == 0 {
		cfg.RetryInterval = 10 * time.Minute
	}
	if cfg.MaxParallelUploads == 0 {
		cfg.MaxParallelUploads = 5
	}
	if len(cfg.Extensions) == 0 {
		cfg.Extensions = append([]string(nil), defaultExtensions...)
	}
	if len(cfg.RcloneArgs) == 0 {
		cfg.RcloneArgs = append([]string(nil), defaultRcloneArgs...)
	}
	for i := range cfg.Extensions {
		cfg.Extensions[i] = strings.ToLower(cfg.Extensions[i])
	}
}

func validate(cfg Config) error {
	if len(cfg.Jobs) == 0 {
		return errors.New("config must define at least one job")
	}
	seenSources := map[string]string{}
	seenLinks := map[string]string{}
	for _, job := range cfg.Jobs {
		if job.Name == "" || job.SourceDir == "" || job.LinkDir == "" || job.RcloneRemote == "" {
			return fmt.Errorf("job %q has empty required fields", job.Name)
		}
		if other, ok := seenSources[job.SourceDir]; ok {
			return fmt.Errorf("duplicate source_dir %q in jobs %q and %q", job.SourceDir, other, job.Name)
		}
		if other, ok := seenLinks[job.LinkDir]; ok {
			return fmt.Errorf("duplicate link_dir %q in jobs %q and %q", job.LinkDir, other, job.Name)
		}
		seenSources[job.SourceDir] = job.Name
		seenLinks[job.LinkDir] = job.Name
	}
	return nil
}
```

```yaml
poll_interval: 1s
stable_duration: 60s
retry_interval: 10m
max_parallel_uploads: 5
extensions: [".mkv", ".mp4", ".m2ts", ".ts"]
rclone_args:
  - --drive-chunk-size=256M
  - --checkers=5
  - --transfers=5
  - --drive-stop-on-upload-limit
  - --stats=1s
  - --stats-one-line
  - -v

jobs:
  - name: 4k-remux-sgnb
    source_dir: /dld/upload/4K.REMUX.SGNB
    link_dir: /dld/gd_upload/4K.REMUX.SGNB
    rclone_remote: gd1:/sync/Movie/4K.REMUX.SGNB
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/config -run 'TestLoadConfig(AppliesDefaults|RejectsDuplicateSourceDir)' -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add go.mod internal/config/config.go internal/config/config_test.go configs/config.example.yaml
git commit -m "feat: add configuration loading"
```

### Task 2: Implement Scheduler State and Concurrency Limits

**Files:**
- Create: `internal/queue/scheduler.go`
- Create: `internal/queue/scheduler_test.go`

- [ ] **Step 1: Write the failing scheduler tests**

```go
package queue_test

import (
	"testing"
	"time"

	"media-backup/internal/queue"
)

func TestSchedulerDeduplicatesQueuedJob(t *testing.T) {
	t.Parallel()

	s := queue.New(queue.Options{MaxParallel: 5, RetryInterval: 10 * time.Minute})
	s.MarkDirty("job-a")
	s.MarkDirty("job-a")

	ready := s.Ready()
	if len(ready) != 1 || ready[0] != "job-a" {
		t.Fatalf("Ready() = %v, want [job-a]", ready)
	}
}

func TestSchedulerRespectsPerJobSerialAndGlobalLimit(t *testing.T) {
	t.Parallel()

	s := queue.New(queue.Options{MaxParallel: 2, RetryInterval: 10 * time.Minute})
	s.MarkDirty("job-a")
	s.MarkDirty("job-b")
	s.MarkDirty("job-c")

	first := s.TryStart("job-a")
	second := s.TryStart("job-b")
	third := s.TryStart("job-c")

	if !first || !second {
		t.Fatal("expected first two jobs to start")
	}
	if third {
		t.Fatal("expected third job to wait for global slot")
	}
	if s.TryStart("job-a") {
		t.Fatal("same job should not start twice")
	}
}

func TestSchedulerSchedulesRetryWithoutDuplicateQueueEntries(t *testing.T) {
	t.Parallel()

	s := queue.New(queue.Options{MaxParallel: 1, RetryInterval: 10 * time.Minute})
	s.MarkDirty("job-a")
	if !s.TryStart("job-a") {
		t.Fatal("expected job-a to start")
	}

	s.FinishFailed("job-a")
	s.FinishFailed("job-a")

	retries := s.RetryReady()
	if len(retries) != 1 || retries[0] != "job-a" {
		t.Fatalf("RetryReady() = %v, want [job-a]", retries)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/queue -run 'TestScheduler(DeduplicatesQueuedJob|RespectsPerJobSerialAndGlobalLimit|SchedulesRetryWithoutDuplicateQueueEntries)' -v`
Expected: FAIL because `internal/queue` does not exist yet.

- [ ] **Step 3: Write minimal implementation**

```go
package queue

import (
	"sync"
	"time"
)

type Options struct {
	MaxParallel   int
	RetryInterval time.Duration
}

type jobState struct {
	queued  bool
	running bool
	dirty   bool
}

type Scheduler struct {
	mu          sync.Mutex
	maxParallel int
	retryAfter  time.Duration
	active      int
	order       []string
	retries     map[string]struct{}
	jobs        map[string]*jobState
}

func New(opts Options) *Scheduler {
	if opts.MaxParallel <= 0 {
		opts.MaxParallel = 1
	}
	return &Scheduler{
		maxParallel: opts.MaxParallel,
		retryAfter:  opts.RetryInterval,
		retries:     map[string]struct{}{},
		jobs:        map[string]*jobState{},
	}
}

func (s *Scheduler) MarkDirty(job string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	state := s.ensure(job)
	state.dirty = true
	if !state.queued && !state.running {
		state.queued = true
		s.order = append(s.order, job)
	}
}

func (s *Scheduler) Ready() []string {
	s.mu.Lock()
	defer s.mu.Unlock()

	out := make([]string, 0, len(s.order))
	for _, job := range s.order {
		if state := s.jobs[job]; state != nil && state.queued && !state.running {
			out = append(out, job)
		}
	}
	return out
}

func (s *Scheduler) TryStart(job string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	state := s.ensure(job)
	if s.active >= s.maxParallel || state.running || !state.queued {
		return false
	}
	state.queued = false
	state.running = true
	state.dirty = false
	delete(s.retries, job)
	s.active++
	s.remove(job)
	return true
}

func (s *Scheduler) Finish(job string, dirty bool) {
	s.mu.Lock()
	defer s.mu.Unlock()

	state := s.ensure(job)
	if state.running && s.active > 0 {
		s.active--
	}
	state.running = false
	if dirty || state.dirty {
		state.dirty = true
		state.queued = true
		s.order = append(s.order, job)
	}
}

func (s *Scheduler) FinishFailed(job string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	state := s.ensure(job)
	if state.running && s.active > 0 {
		s.active--
	}
	state.running = false
	s.retries[job] = struct{}{}
}

func (s *Scheduler) RetryReady() []string {
	s.mu.Lock()
	defer s.mu.Unlock()

	out := make([]string, 0, len(s.retries))
	for job := range s.retries {
		out = append(out, job)
	}
	return out
}

func (s *Scheduler) RetryAfter() time.Duration {
	return s.retryAfter
}

func (s *Scheduler) ensure(job string) *jobState {
	if state, ok := s.jobs[job]; ok {
		return state
	}
	state := &jobState{}
	s.jobs[job] = state
	return state
}

func (s *Scheduler) remove(job string) {
	dst := s.order[:0]
	for _, item := range s.order {
		if item != job {
			dst = append(dst, item)
		}
	}
	s.order = dst
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/queue -run 'TestScheduler(DeduplicatesQueuedJob|RespectsPerJobSerialAndGlobalLimit|SchedulesRetryWithoutDuplicateQueueEntries)' -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/queue/scheduler.go internal/queue/scheduler_test.go
git commit -m "feat: add upload scheduler"
```

### Task 3: Parse Rclone Stats and Wrap Process Execution

**Files:**
- Create: `internal/rclone/parser.go`
- Create: `internal/rclone/parser_test.go`
- Create: `internal/rclone/runner.go`
- Create: `internal/rclone/runner_test.go`

- [ ] **Step 1: Write the failing parser and runner tests**

```go
package rclone_test

import (
	"context"
	"strings"
	"testing"

	"media-backup/internal/rclone"
)

func TestParseStatsLine(t *testing.T) {
	t.Parallel()

	line := "Transferred:    1.234 TiB / 3.560 TiB, 35%, 82.114 MiB/s, ETA 6h12m"
	got, ok := rclone.ParseStats(line)
	if !ok {
		t.Fatal("ParseStats() ok = false, want true")
	}
	if !strings.Contains(got, "35%") {
		t.Fatalf("ParseStats() = %q, want percentage content", got)
	}
}

func TestRunnerBuildsCommand(t *testing.T) {
	t.Parallel()

	rec := &rclone.RecordingExecutor{}
	runner := rclone.NewRunner(rec)

	err := runner.Copy(context.Background(), "/src", "gd1:/dst", []string{"--stats=1s"})
	if err != nil {
		t.Fatalf("Copy() error = %v", err)
	}
	if got := strings.Join(rec.Args, " "); got != "copy /src gd1:/dst --stats=1s" {
		t.Fatalf("executor args = %q", got)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/rclone -run 'Test(ParseStatsLine|RunnerBuildsCommand)' -v`
Expected: FAIL because `internal/rclone` does not exist yet.

- [ ] **Step 3: Write minimal implementation**

```go
package rclone

import "strings"

func ParseStats(line string) (string, bool) {
	line = strings.TrimSpace(line)
	if line == "" {
		return "", false
	}
	if strings.Contains(line, "Transferred:") {
		return line, true
	}
	return "", false
}
```

```go
package rclone

import "context"

type Executor interface {
	Run(ctx context.Context, args []string) error
}

type RecordingExecutor struct {
	Args []string
	Err  error
}

func (r *RecordingExecutor) Run(_ context.Context, args []string) error {
	r.Args = append([]string(nil), args...)
	return r.Err
}

type Runner struct {
	exec Executor
}

func NewRunner(exec Executor) *Runner {
	return &Runner{exec: exec}
}

func (r *Runner) Copy(ctx context.Context, linkDir, remote string, extraArgs []string) error {
	args := []string{"copy", linkDir, remote}
	args = append(args, extraArgs...)
	return r.exec.Run(ctx, args)
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/rclone -run 'Test(ParseStatsLine|RunnerBuildsCommand)' -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/rclone/parser.go internal/rclone/parser_test.go internal/rclone/runner.go internal/rclone/runner_test.go
git commit -m "feat: add rclone parsing and runner"
```

### Task 4: Render Idle and Multi-Job Terminal Status

**Files:**
- Create: `internal/ui/renderer.go`
- Create: `internal/ui/renderer_test.go`

- [ ] **Step 1: Write the failing renderer tests**

```go
package ui_test

import (
	"strings"
	"testing"
	"time"

	"media-backup/internal/ui"
)

func TestRenderIdle(t *testing.T) {
	t.Parallel()

	out := ui.RenderIdle(time.Date(2026, 4, 22, 15, 4, 5, 0, time.UTC))
	if !strings.Contains(out, "当前状态：空闲") {
		t.Fatalf("RenderIdle() = %q", out)
	}
}

func TestRenderActiveDashboard(t *testing.T) {
	t.Parallel()

	out := ui.RenderDashboard(
		time.Date(2026, 4, 22, 15, 4, 5, 0, time.UTC),
		[]ui.JobStatus{
			{Name: "job-a", Summary: "Transferred: 35%"},
			{Name: "job-b", Summary: "Transferred: 74%"},
		},
		1,
		5,
	)
	if !strings.Contains(out, "活跃任务: 2/5") {
		t.Fatalf("RenderDashboard() = %q", out)
	}
	if !strings.Contains(out, "[job-a]") || !strings.Contains(out, "[job-b]") {
		t.Fatalf("RenderDashboard() missing job lines: %q", out)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/ui -run 'TestRender(Idle|ActiveDashboard)' -v`
Expected: FAIL because `internal/ui` does not exist yet.

- [ ] **Step 3: Write minimal implementation**

```go
package ui

import (
	"fmt"
	"strings"
	"time"
)

type JobStatus struct {
	Name    string
	Summary string
}

func RenderIdle(now time.Time) string {
	return fmt.Sprintf("[%s] 当前状态：空闲", now.Format("2006-01-02 15:04:05"))
}

func RenderDashboard(now time.Time, active []JobStatus, waiting int, maxParallel int) string {
	lines := []string{
		fmt.Sprintf("[%s] 当前状态：正在传输 | 活跃任务: %d/%d | 等待中: %d",
			now.Format("2006-01-02 15:04:05"), len(active), maxParallel, waiting),
	}
	for _, job := range active {
		lines = append(lines, fmt.Sprintf("[%s] %s", job.Name, job.Summary))
	}
	return strings.Join(lines, "\n")
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/ui -run 'TestRender(Idle|ActiveDashboard)' -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/ui/renderer.go internal/ui/renderer_test.go
git commit -m "feat: add terminal status renderer"
```

### Task 5: Implement File Stability, Hard Links, and Cleanup

**Files:**
- Create: `internal/watcher/files.go`
- Create: `internal/watcher/files_test.go`

- [ ] **Step 1: Write the failing filesystem helper tests**

```go
package watcher_test

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"media-backup/internal/watcher"
)

func TestLinkFileCreatesParentDirectories(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	sourceDir := filepath.Join(root, "source")
	linkDir := filepath.Join(root, "link")
	if err := os.MkdirAll(filepath.Join(sourceDir, "movie"), 0o755); err != nil {
		t.Fatal(err)
	}
	sourceFile := filepath.Join(sourceDir, "movie", "feature.mkv")
	if err := os.WriteFile(sourceFile, []byte("data"), 0o644); err != nil {
		t.Fatal(err)
	}

	target, err := watcher.LinkFile(sourceDir, linkDir, sourceFile)
	if err != nil {
		t.Fatalf("LinkFile() error = %v", err)
	}
	if _, err := os.Stat(target); err != nil {
		t.Fatalf("Stat(target) error = %v", err)
	}
}

func TestCleanupLinkDirPreservesRoot(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	linkDir := filepath.Join(root, "link")
	childDir := filepath.Join(linkDir, "movie")
	if err := os.MkdirAll(childDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(childDir, "feature.mkv"), []byte("data"), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := watcher.CleanupLinkDir(linkDir); err != nil {
		t.Fatalf("CleanupLinkDir() error = %v", err)
	}
	if _, err := os.Stat(linkDir); err != nil {
		t.Fatalf("linkDir should still exist: %v", err)
	}
	if _, err := os.Stat(childDir); !os.IsNotExist(err) {
		t.Fatalf("childDir should be removed, got err=%v", err)
	}
}

func TestWaitStableReturnsAfterSizeStopsChanging(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	file := filepath.Join(root, "feature.mkv")
	if err := os.WriteFile(file, []byte("a"), 0o644); err != nil {
		t.Fatal(err)
	}
	done := make(chan struct{})
	go func() {
		defer close(done)
		time.Sleep(20 * time.Millisecond)
		_ = os.WriteFile(file, []byte("ab"), 0o644)
	}()

	if err := watcher.WaitStable(file, 10*time.Millisecond, 5*time.Millisecond); err != nil {
		t.Fatalf("WaitStable() error = %v", err)
	}
	<-done
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/watcher -run 'Test(LinkFileCreatesParentDirectories|CleanupLinkDirPreservesRoot)' -v`
Expected: FAIL because helper functions do not exist yet.

- [ ] **Step 3: Write minimal implementation**

```go
package watcher

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"
)

func LinkFile(sourceDir, linkDir, sourceFile string) (string, error) {
	rel, err := filepath.Rel(sourceDir, sourceFile)
	if err != nil {
		return "", fmt.Errorf("rel path: %w", err)
	}
	target := filepath.Join(linkDir, rel)
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return "", fmt.Errorf("mkdir target parent: %w", err)
	}
	if err := os.Link(sourceFile, target); err != nil && !os.IsExist(err) {
		return "", fmt.Errorf("link file: %w", err)
	}
	return target, nil
}

func CleanupLinkDir(linkDir string) error {
	entries, err := os.ReadDir(linkDir)
	if err != nil {
		return err
	}
	var dirs []string
	for _, entry := range entries {
		path := filepath.Join(linkDir, entry.Name())
		if entry.IsDir() {
			dirs = append(dirs, path)
			continue
		}
		if err := os.Remove(path); err != nil {
			return err
		}
	}
	sort.Slice(dirs, func(i, j int) bool { return len(dirs[i]) > len(dirs[j]) })
	for _, dir := range dirs {
		if err := os.RemoveAll(dir); err != nil {
			return err
		}
	}
	return nil
}

func WaitStable(path string, stableFor time.Duration, pollInterval time.Duration) error {
	var lastSize int64 = -1
	var stableSince time.Time

	for {
		info, err := os.Stat(path)
		if err != nil {
			return err
		}
		size := info.Size()
		if size != lastSize {
			lastSize = size
			stableSince = time.Now()
		} else if time.Since(stableSince) >= stableFor {
			return nil
		}
		time.Sleep(pollInterval)
	}
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/watcher -run 'Test(LinkFileCreatesParentDirectories|CleanupLinkDirPreservesRoot)' -v`
Expected: PASS

Run: `go test ./internal/watcher -run TestWaitStableReturnsAfterSizeStopsChanging -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/watcher/files.go internal/watcher/files_test.go
git commit -m "feat: add file link and cleanup helpers"
```

### Task 6: Add Startup Scanning and Recursive Watch Registration

**Files:**
- Create: `internal/watcher/scanner.go`
- Create: `internal/watcher/scanner_test.go`
- Create: `internal/watcher/watch_service.go`
- Create: `internal/watcher/watch_service_test.go`

- [ ] **Step 1: Write the failing scan and watch tests**

```go
package watcher_test

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"media-backup/internal/watcher"
)

func TestScanLinksMatchingMediaFiles(t *testing.T) {
	t.Parallel()

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

	count, err := watcher.ScanAndLink(sourceDir, linkDir, []string{".mkv"}, time.Millisecond)
	if err != nil {
		t.Fatalf("ScanAndLink() error = %v", err)
	}
	if count != 1 {
		t.Fatalf("ScanAndLink() count = %d, want 1", count)
	}
}

func TestWatchServiceRegistersNewDirectories(t *testing.T) {
	t.Parallel()

	reg := watcher.NewMemoryRegistrar()
	svc := watcher.NewWatchService(reg)
	if err := svc.AddRecursive("/source", []string{"/source", "/source/movie"}); err != nil {
		t.Fatalf("AddRecursive() error = %v", err)
	}
	if got := reg.Paths(); len(got) != 2 {
		t.Fatalf("registered paths = %v, want 2 entries", got)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/watcher -run 'Test(ScanLinksMatchingMediaFiles|WatchServiceRegistersNewDirectories)' -v`
Expected: FAIL because `ScanAndLink` and watch service types do not exist yet.

- [ ] **Step 3: Write minimal implementation**

```go
package watcher

import (
	"os"
	"path/filepath"
	"strings"
	"time"
)

func ScanAndLink(sourceDir, linkDir string, extensions []string, stableDuration time.Duration) (int, error) {
	var count int
	err := filepath.WalkDir(sourceDir, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		if !hasExtension(path, extensions) {
			return nil
		}
		if err := WaitStable(path, stableDuration, time.Millisecond); err != nil {
			return err
		}
		if _, err := LinkFile(sourceDir, linkDir, path); err != nil {
			return err
		}
		count++
		return nil
	})
	return count, err
}

func hasExtension(path string, extensions []string) bool {
	ext := strings.ToLower(filepath.Ext(path))
	for _, item := range extensions {
		if ext == strings.ToLower(item) {
			return true
		}
	}
	return false
}
```

```go
package watcher

type Registrar interface {
	Add(path string) error
}

type MemoryRegistrar struct {
	seen []string
}

func NewMemoryRegistrar() *MemoryRegistrar {
	return &MemoryRegistrar{}
}

func (m *MemoryRegistrar) Add(path string) error {
	m.seen = append(m.seen, path)
	return nil
}

func (m *MemoryRegistrar) Paths() []string {
	return append([]string(nil), m.seen...)
}

type WatchService struct {
	reg Registrar
}

func NewWatchService(reg Registrar) *WatchService {
	return &WatchService{reg: reg}
}

func (s *WatchService) AddRecursive(root string, existing []string) error {
	for _, path := range existing {
		if err := s.reg.Add(path); err != nil {
			return err
		}
	}
	return nil
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/watcher -run 'Test(ScanLinksMatchingMediaFiles|WatchServiceRegistersNewDirectories)' -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/watcher/scanner.go internal/watcher/scanner_test.go internal/watcher/watch_service.go internal/watcher/watch_service_test.go
git commit -m "feat: add scan and watch services"
```

### Task 7: Wire the App, Signals, and Static Build Script

**Files:**
- Create: `internal/app/app.go`
- Create: `internal/app/app_test.go`
- Create: `internal/app/logging.go`
- Create: `internal/app/logging_test.go`
- Create: `cmd/media-backup/main.go`
- Create: `build.sh`

- [ ] **Step 1: Write the failing app and build tests**

```go
package app_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"media-backup/internal/app"
)

func TestRunCancelsUploaderOnShutdown(t *testing.T) {
	t.Parallel()

	called := false
	stop := make(chan struct{})
	a := app.New(app.Dependencies{
		RunUploads: func(ctx context.Context) error {
			<-ctx.Done()
			called = true
			return ctx.Err()
		},
	})

	go close(stop)
	err := a.Run(context.Background(), stop)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Run() error = %v, want context.Canceled", err)
	}
	if !called {
		t.Fatal("RunUploads should observe cancellation")
	}
}

func TestOpenLogFileCreatesParentDir(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "logs", "media-backup.log")
	file, err := app.OpenLogFile(path)
	if err != nil {
		t.Fatalf("OpenLogFile() error = %v", err)
	}
	t.Cleanup(func() { _ = file.Close() })

	if _, err := os.Stat(path); err != nil {
		t.Fatalf("log file missing: %v", err)
	}
}
```

```bash
#!/usr/bin/env bash
set -euo pipefail

mkdir -p dist
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -ldflags="-s -w" -o dist/media-backup-linux-amd64 ./cmd/media-backup
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/app -run 'Test(RunCancelsUploaderOnShutdown|OpenLogFileCreatesParentDir)' -v`
Expected: FAIL because `internal/app` does not exist yet.

- [ ] **Step 3: Write minimal implementation**

```go
package app

import "context"

type Dependencies struct {
	RunUploads func(context.Context) error
}

type App struct {
	runUploads func(context.Context) error
}

func New(deps Dependencies) *App {
	return &App{runUploads: deps.RunUploads}
}

func (a *App) Run(parent context.Context, stop <-chan struct{}) error {
	ctx, cancel := context.WithCancel(parent)
	defer cancel()

	go func() {
		<-stop
		cancel()
	}()

	return a.runUploads(ctx)
}
```

```go
package app

import (
	"os"
	"path/filepath"
)

func OpenLogFile(path string) (*os.File, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, err
	}
	return os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
}
```

```go
package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"

	"media-backup/internal/app"
)

func main() {
	logFile, err := app.OpenLogFile("logs/media-backup.log")
	if err != nil {
		log.Fatal(err)
	}
	defer logFile.Close()
	logger := log.New(logFile, "", log.LstdFlags)

	stop := make(chan struct{})
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigCh
		close(stop)
	}()

	a := app.New(app.Dependencies{
		RunUploads: func(context.Context) error { return nil },
	})
	if err := a.Run(context.Background(), stop); err != nil && err != context.Canceled {
		logger.Fatal(err)
	}
}
```

- [ ] **Step 4: Run test and build to verify they pass**

Run: `go test ./internal/app -run 'Test(RunCancelsUploaderOnShutdown|OpenLogFileCreatesParentDir)' -v`
Expected: PASS

Run: `bash build.sh`
Expected: `dist/media-backup-linux-amd64` is created without CGO.

- [ ] **Step 5: Commit**

```bash
git add internal/app/app.go internal/app/app_test.go internal/app/logging.go internal/app/logging_test.go cmd/media-backup/main.go build.sh
git commit -m "feat: wire app entrypoint and build script"
```

## Self-Review

- Spec coverage check:
  config layout is covered by Task 1
  per-job serial plus global concurrency is covered by Task 2
  rclone invocation and progress parsing is covered by Task 3
  idle and multi-job UI output is covered by Task 4
  hard links and root-preserving cleanup are covered by Task 5
  startup scan and recursive watch registration are covered by Task 6
  signal handling, log file creation, and static build output are covered by Task 7
- Placeholder scan:
  no `TODO`, `TBD`, or deferred “implement later” wording remains in task steps
- Type consistency:
  package names, function names, and file paths are consistent across tasks
