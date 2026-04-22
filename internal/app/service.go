package app

import (
	"context"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/guowanghushifu/media-backup/internal/config"
	"github.com/guowanghushifu/media-backup/internal/queue"
	"github.com/guowanghushifu/media-backup/internal/rclone"
	"github.com/guowanghushifu/media-backup/internal/ui"
	"github.com/guowanghushifu/media-backup/internal/watcher"
)

const maxRecentEvents = 10

type Service struct {
	cfg            *config.Config
	logger         *log.Logger
	scheduler      *queue.Scheduler
	watcher        *fsnotify.Watcher
	uiWriter       io.Writer
	mkdirAll       func(string, os.FileMode) error
	addWatches     func(string) error
	scanExisting   func(string, string, []string, time.Duration) (int, error)
	scanLinkDir    func(string, []string) (int, error)
	copyJob        func(context.Context, *jobRuntime) error
	cleanupLinkDir func(string) error
	startUpload    func(context.Context, *jobRuntime)
	afterMarkDirty func(string)
	now            func() time.Time

	mu           sync.Mutex
	jobs         map[string]*jobRuntime
	processing   map[string]struct{}
	recentEvents []recentEvent
	retryDue     map[string]time.Time
	wakeCh       chan struct{}
}

type recentEvent struct {
	at      time.Time
	message string
}

type jobRuntime struct {
	cfg            config.JobConfig
	key            string
	summary        string
	active         bool
	dirtyDuringRun bool
}

func NewService(cfg *config.Config, logger *log.Logger) (*Service, error) {
	fsWatcher, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, err
	}

	s := &Service{
		cfg:            cfg,
		logger:         logger,
		scheduler:      queue.New(queue.Options{MaxParallel: cfg.MaxParallelUploads, RetryInterval: cfg.RetryInterval}),
		watcher:        fsWatcher,
		uiWriter:       os.Stdout,
		mkdirAll:       os.MkdirAll,
		scanExisting:   watcher.ScanExistingAndLink,
		scanLinkDir:    countUploadableFiles,
		cleanupLinkDir: watcher.CleanupLinkDir,
		now:            time.Now,
		jobs:           make(map[string]*jobRuntime, len(cfg.Jobs)),
		processing:     map[string]struct{}{},
		retryDue:       map[string]time.Time{},
		wakeCh:         make(chan struct{}, 1),
	}
	s.addWatches = s.addRecursiveWatches
	s.copyJob = s.copyWithRclone
	s.startUpload = func(ctx context.Context, job *jobRuntime) {
		go s.runUpload(ctx, job)
	}

	for _, job := range cfg.Jobs {
		s.jobs[job.SourceDir] = &jobRuntime{cfg: job, key: job.SourceDir}
	}
	return s, nil
}

func (s *Service) Close() error {
	return s.watcher.Close()
}

func (s *Service) Run(ctx context.Context) error {
	defer s.Close()

	runCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		s.uiLoop(runCtx)
	}()

	if err := s.startupCatchUp(); err != nil {
		cancel()
		wg.Wait()
		return err
	}

	wg.Add(2)
	go func() {
		defer wg.Done()
		s.eventLoop(runCtx)
	}()
	go func() {
		defer wg.Done()
		s.dispatchLoop(runCtx)
	}()

	<-ctx.Done()
	cancel()
	wg.Wait()
	return ctx.Err()
}

func (s *Service) startupCatchUp() error {
	for _, job := range s.jobs {
		if err := s.mkdirAll(job.cfg.LinkDir, 0o755); err != nil {
			return err
		}
		if err := s.addWatches(job.cfg.SourceDir); err != nil {
			return err
		}
		count, err := s.scanExisting(job.cfg.SourceDir, job.cfg.LinkDir, s.cfg.Extensions, s.cfg.StableDuration)
		if err != nil {
			return err
		}
		if count > 0 {
			wasQueued := s.isJobReady(job.key)
			s.scheduler.MarkDirty(job.key)
			s.runAfterMarkDirty(job.key)
			if !wasQueued {
				s.appendSchedulerEventNow(job, fmt.Sprintf("启动扫描发现 %d 个文件，任务标记为待上传", count))
			}
			continue
		}

		linkCount, err := s.scanLinkDir(job.cfg.LinkDir, s.cfg.Extensions)
		if err != nil {
			return err
		}
		if linkCount > 0 {
			wasQueued := s.isJobReady(job.key)
			s.scheduler.MarkDirty(job.key)
			s.runAfterMarkDirty(job.key)
			if !wasQueued {
				s.appendSchedulerEventNow(job, fmt.Sprintf("链接目录发现 %d 个待上传文件，任务标记为待上传", linkCount))
			}
		}
	}
	return nil
}

func (s *Service) eventLoop(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case event, ok := <-s.watcher.Events:
			if !ok {
				return
			}
			s.handleEvent(ctx, event)
		case err, ok := <-s.watcher.Errors:
			if !ok {
				return
			}
			s.logger.Printf("watch error: %v", err)
		}
	}
}

func (s *Service) handleEvent(ctx context.Context, event fsnotify.Event) {
	job := s.findJob(event.Name)
	if job == nil {
		return
	}

	info, err := os.Stat(event.Name)
	if err == nil && info.IsDir() {
		if event.Op&(fsnotify.Create|fsnotify.Rename) != 0 {
			if err := s.addRecursiveWatches(event.Name); err != nil {
				s.logger.Printf("add recursive watch %s: %v", event.Name, err)
				return
			}
			s.processTree(ctx, job, event.Name)
		}
		return
	}

	if event.Op&(fsnotify.Create|fsnotify.Write|fsnotify.Rename) == 0 {
		return
	}
	if !hasAllowedExtension(event.Name, s.cfg.Extensions) {
		return
	}
	go s.processFile(ctx, job, event.Name)
}

func (s *Service) processTree(ctx context.Context, job *jobRuntime, root string) {
	_ = filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			return nil
		}
		if !hasAllowedExtension(path, s.cfg.Extensions) {
			return nil
		}
		go s.processFile(ctx, job, path)
		return nil
	})
}

func (s *Service) processFile(ctx context.Context, job *jobRuntime, path string) {
	s.mu.Lock()
	if s.processing == nil {
		s.processing = map[string]struct{}{}
	}
	if _, ok := s.processing[path]; ok {
		s.mu.Unlock()
		return
	}
	s.processing[path] = struct{}{}
	s.mu.Unlock()
	defer func() {
		s.mu.Lock()
		delete(s.processing, path)
		s.mu.Unlock()
	}()

	if err := watcher.WaitStable(path, s.cfg.StableDuration, s.cfg.PollInterval); err != nil {
		s.logger.Printf("wait stable %s: %v", path, err)
		return
	}
	if _, err := watcher.LinkFile(job.cfg.SourceDir, job.cfg.LinkDir, path); err != nil {
		s.logger.Printf("link file %s: %v", path, err)
		return
	}

	s.mu.Lock()
	wasActive := job.active
	wasDirtyDuringRun := job.dirtyDuringRun
	if job.active {
		job.dirtyDuringRun = true
	}
	s.mu.Unlock()

	wasQueued := s.isJobReady(job.key)
	wasPendingRetry := s.isRetryWaiting(job.key)
	s.scheduler.MarkDirty(job.key)
	s.runAfterMarkDirty(job.key)
	if wasActive && !wasDirtyDuringRun {
		s.appendSchedulerEventNow(job, "检测到新文件，任务保持运行中，完成后将重新排队")
	}
	if !wasActive && !wasQueued && !wasPendingRetry {
		s.appendSchedulerEventNow(job, "检测到新文件，任务标记为待上传")
	}
	s.signalWake()
}

func (s *Service) dispatchLoop(ctx context.Context) {
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.releaseRetries()
			s.startReadyUploads(ctx)
		case <-s.wakeCh:
			s.releaseRetries()
			s.startReadyUploads(ctx)
		}
	}
}

func (s *Service) releaseRetries() {
	s.mu.Lock()
	now := s.currentTime()
	var due []string
	for key, at := range s.retryDue {
		if !now.Before(at) {
			due = append(due, key)
			delete(s.retryDue, key)
		}
	}
	s.mu.Unlock()

	for _, key := range due {
		if s.scheduler.RetryJob(key) {
			if job := s.jobs[key]; job != nil {
				s.appendSchedulerEventNow(job, "到达重试时间，重新排队")
			}
			s.signalWake()
		}
	}
}

func (s *Service) startReadyUploads(ctx context.Context) {
	for _, key := range s.scheduler.Ready() {
		if !s.scheduler.TryStart(key) {
			continue
		}
		job := s.jobs[key]
		s.mu.Lock()
		job.active = true
		job.dirtyDuringRun = false
		job.summary = "等待 rclone 输出"
		s.mu.Unlock()
		s.appendSchedulerEventNow(job, "调度开始上传")
		s.startUpload(ctx, job)
	}
}

func (s *Service) runUpload(ctx context.Context, job *jobRuntime) {
	s.logRcloneCommand(job)
	err := s.copyJob(ctx, job)
	if err != nil {
		s.mu.Lock()
		job.active = false
		job.summary = fmt.Sprintf("上传失败: %v", err)
		s.retryDue[job.key] = s.currentTime().Add(s.cfg.RetryInterval)
		s.mu.Unlock()
		s.scheduler.FinishFailed(job.key)
		s.appendSchedulerEventNow(job, "上传失败，进入重试等待")
		s.signalWake()
		return
	}

	s.mu.Lock()
	dirtyDuringRun := job.dirtyDuringRun
	job.active = false
	job.summary = "上传完成"
	s.mu.Unlock()

	if dirtyDuringRun {
		s.scheduler.Finish(job.key, false)
		s.appendSchedulerEventNow(job, "上传完成，检测到新增文件，重新排队")
		s.signalWake()
		return
	}

	if err := s.cleanupLinkDir(job.cfg.LinkDir); err != nil {
		s.logger.Printf("cleanup %s: %v", job.cfg.LinkDir, err)
		s.mu.Lock()
		job.summary = fmt.Sprintf("清理失败: %v", err)
		s.retryDue[job.key] = s.currentTime().Add(s.cfg.RetryInterval)
		s.mu.Unlock()
		s.scheduler.FinishFailed(job.key)
		s.appendSchedulerEventNow(job, "上传失败，进入重试等待")
		s.signalWake()
		return
	}

	s.scheduler.Finish(job.key, false)
	s.appendSchedulerEventNow(job, "上传完成，任务清空")
	s.signalWake()
}

func (s *Service) copyWithRclone(ctx context.Context, job *jobRuntime) error {
	exec := &rclone.CommandExecutor{
		Proxy: s.cfg.Proxy,
		OnOutput: func(line string) {
			s.handleRcloneOutputLine(job, line, time.Now)
		},
	}
	runner := rclone.NewRunner(exec)
	return runner.Copy(ctx, job.cfg.LinkDir, job.cfg.RcloneRemote, s.cfg.RcloneArgs)
}

func (s *Service) logRcloneCommand(job *jobRuntime) {
	if s.logger == nil || job == nil {
		return
	}
	args := append([]string{"rclone", "copy", job.cfg.LinkDir, job.cfg.RcloneRemote}, s.cfg.RcloneArgs...)
	s.logger.Printf("run rclone command for %s: %s", job.cfg.Name, strings.Join(args, " "))
}

func (s *Service) uiLoop(ctx context.Context) {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()

	s.runUILoop(ctx, ticker.C)
}

func (s *Service) runUILoop(ctx context.Context, ticks <-chan time.Time) {
	s.writeUI(ui.EnterAlternateScreen())
	defer s.writeUI("\n")
	defer s.writeUI(ui.LeaveAlternateScreen())
	s.renderDashboard(s.currentTime())

	for {
		select {
		case <-ctx.Done():
			return
		case now := <-ticks:
			s.renderDashboard(now)
		}
	}
}

func (s *Service) currentTime() time.Time {
	if s.now != nil {
		return s.now()
	}
	return time.Now()
}

func (s *Service) renderDashboard(now time.Time) {
	active, events, waiting := s.snapshotUI()
	content := ui.RenderDashboard(now, active, events, waiting, s.cfg.MaxParallelUploads)
	s.writeUI(ui.RewriteFrame(content))
}

func (s *Service) writeUI(content string) {
	writer := s.uiWriter
	if writer == nil {
		writer = os.Stdout
	}
	_, _ = io.WriteString(writer, content)
}

func (s *Service) snapshotUI() ([]ui.JobStatus, []ui.EventRecord, int) {
	s.mu.Lock()
	defer s.mu.Unlock()

	active := make([]ui.JobStatus, 0, len(s.jobs))
	for _, job := range s.jobs {
		if !job.active {
			continue
		}
		summary := job.summary
		if summary == "" {
			summary = "等待 rclone 输出"
		}
		active = append(active, ui.JobStatus{Name: job.cfg.Name, Summary: summary})
	}
	sort.Slice(active, func(i, j int) bool {
		if active[i].Name == active[j].Name {
			return active[i].Summary < active[j].Summary
		}
		return active[i].Name < active[j].Name
	})
	events := make([]ui.EventRecord, 0, len(s.recentEvents))
	for _, event := range s.recentEvents {
		events = append(events, ui.EventRecord{At: event.at, Message: event.message})
	}
	return active, events, len(s.scheduler.Ready())
}

func (s *Service) appendRecentEvent(at time.Time, message string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.appendRecentEventLocked(at, message)
}

func (s *Service) appendRecentEventNow(message string, now func() time.Time) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.appendRecentEventLocked(now(), message)
}

func (s *Service) appendSchedulerEvent(at time.Time, job *jobRuntime, message string) {
	if job == nil {
		return
	}
	s.appendRecentEvent(at, fmt.Sprintf("[%s] %s", job.cfg.Name, message))
}

func (s *Service) appendSchedulerEventNow(job *jobRuntime, message string) {
	s.appendSchedulerEvent(s.currentTime(), job, message)
}

func (s *Service) appendRecentEventLocked(at time.Time, message string) {
	s.recentEvents = append(s.recentEvents, recentEvent{at: at, message: message})
	if len(s.recentEvents) > maxRecentEvents {
		s.recentEvents = append([]recentEvent(nil), s.recentEvents[len(s.recentEvents)-maxRecentEvents:]...)
	}
}

func (s *Service) handleRcloneOutputLine(job *jobRuntime, line string, now func() time.Time) {
	s.logger.Println(line)
	if stats, ok := rclone.ParseStats(line); ok {
		s.mu.Lock()
		job.summary = stats
		s.mu.Unlock()
		return
	}
	if payload, ok := rclone.ParseEvent(line); ok {
		s.appendRecentEventNow(payload, now)
	}
}

func (s *Service) findJob(path string) *jobRuntime {
	cleanPath := filepath.Clean(path)
	for _, job := range s.jobs {
		source := filepath.Clean(job.cfg.SourceDir)
		if cleanPath == source || strings.HasPrefix(cleanPath, source+string(os.PathSeparator)) {
			return job
		}
	}
	return nil
}

func (s *Service) addRecursiveWatches(root string) error {
	return filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() {
			return nil
		}
		return s.watcher.Add(path)
	})
}

func (s *Service) signalWake() {
	select {
	case s.wakeCh <- struct{}{}:
	default:
	}
}

func (s *Service) isJobReady(key string) bool {
	for _, job := range s.scheduler.Ready() {
		if job == key {
			return true
		}
	}
	return false
}

func (s *Service) isRetryWaiting(key string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, ok := s.retryDue[key]
	return ok
}

func (s *Service) runAfterMarkDirty(key string) {
	if s.afterMarkDirty != nil {
		s.afterMarkDirty(key)
	}
}

func hasAllowedExtension(path string, extensions []string) bool {
	ext := strings.ToLower(filepath.Ext(path))
	for _, want := range extensions {
		if ext == strings.ToLower(want) {
			return true
		}
	}
	return false
}

func countUploadableFiles(root string, extensions []string) (int, error) {
	var count int
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		if hasAllowedExtension(path, extensions) {
			count++
		}
		return nil
	})
	return count, err
}
