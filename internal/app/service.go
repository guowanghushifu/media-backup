package app

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"reflect"
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
const uiKeepaliveInterval = 3 * time.Second
const maxParallelProcessing = 64

var errUploadSuperseded = errors.New("upload superseded by newer same-path file")

type Service struct {
	cfg                *config.Config
	logger             *log.Logger
	scheduler          *queue.Scheduler
	watcher            *fsnotify.Watcher
	uiWriter           io.Writer
	mkdirAll           func(string, os.FileMode) error
	addWatches         func(string) error
	scanExisting       func(context.Context, string, string, []string, time.Duration, time.Duration) (int, error)
	scanExistingFiles  func(context.Context, string, string, []string, time.Duration, time.Duration) ([]watcher.LinkResult, error)
	scanLinkedFiles    func(string, []string) ([]string, error)
	copyJob            func(context.Context, *jobRuntime) error
	cleanupLinkedFile  func(string, string) error
	cleanupSourceFile  func(string, string) error
	validateLinkedFile func(string, string) error
	waitStable         func(context.Context, string, time.Duration, time.Duration) error
	startUpload        func(context.Context, *jobRuntime)
	afterMarkDirty     func(string)
	beforeProcessFile  func(string)
	now                func() time.Time
	uiWidth            func() int
	notifyFinalFailure func(jobFailureNotification) error

	mu            sync.Mutex
	completionMu  sync.Mutex
	configJobs    map[string]config.JobConfig
	jobs          map[string]*jobRuntime
	processing    map[string]struct{}
	recentEvents  []recentEvent
	retryDue      map[string]time.Time
	failureCounts map[string]int
	processSem    chan struct{}
	wakeCh        chan struct{}
	uiWakeCh      chan struct{}
}

type uiRenderState struct {
	active     []ui.JobStatus
	events     []ui.EventRecord
	waiting    int
	width      int
	renderedAt time.Time
	rendered   bool
}

type uiActiveRow struct {
	status   ui.JobStatus
	linkPath string
	jobKey   string
}

type recentEvent struct {
	at      time.Time
	message string
}

type jobRuntime struct {
	cfg        config.JobConfig
	key        string
	sourcePath string
	linkPath   string
	remoteDir  string
	summary    string
	active     bool
	cancel     context.CancelCauseFunc
}

type queueTaskState struct {
	wasQueued        bool
	clearedRetryWait bool
	replacedRunning  bool
	linkState        watcher.LinkState
}

func NewService(cfg *config.Config, logger *log.Logger) (*Service, error) {
	fsWatcher, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, err
	}
	notifier := newTelegramNotifier(cfg.Telegram, cfg.Proxy)

	s := &Service{
		cfg:                cfg,
		logger:             logger,
		scheduler:          queue.New(queue.Options{MaxParallel: cfg.MaxParallelUploads}),
		watcher:            fsWatcher,
		uiWriter:           os.Stdout,
		mkdirAll:           os.MkdirAll,
		scanExisting:       watcher.ScanExistingAndLinkContext,
		scanExistingFiles:  watcher.ScanExistingAndLinkFilesContext,
		scanLinkedFiles:    watcher.ScanLinkedFiles,
		cleanupLinkedFile:  watcher.CleanupLinkedFile,
		cleanupSourceFile:  watcher.CleanupSourceFile,
		validateLinkedFile: watcher.ValidateLinkedFile,
		now:                time.Now,
		configJobs:         make(map[string]config.JobConfig, len(cfg.Jobs)),
		jobs:               make(map[string]*jobRuntime),
		processing:         map[string]struct{}{},
		retryDue:           map[string]time.Time{},
		failureCounts:      map[string]int{},
		processSem:         make(chan struct{}, maxParallelProcessing),
		wakeCh:             make(chan struct{}, 1),
		uiWakeCh:           make(chan struct{}, 1),
	}
	s.addWatches = s.addRecursiveWatches
	s.copyJob = s.copyWithRclone
	s.waitStable = watcher.WaitStableContext
	s.notifyFinalFailure = func(event jobFailureNotification) error {
		if notifier == nil {
			return nil
		}
		ctx, cancel := context.WithTimeout(context.Background(), telegramNotifyTimeout)
		defer cancel()
		return notifier.NotifyFinalFailure(ctx, event)
	}
	s.uiWidth = func() int {
		return ui.DetectWidth(s.uiWriter)
	}
	s.startUpload = func(ctx context.Context, job *jobRuntime) {
		go s.runUpload(ctx, job)
	}

	for _, job := range cfg.Jobs {
		s.configJobs[job.SourceDir] = job
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

	if err := s.prepareStartup(); err != nil {
		cancel()
		wg.Wait()
		return err
	}

	wg.Add(3)
	go func() {
		defer wg.Done()
		s.eventLoop(runCtx)
	}()
	go func() {
		defer wg.Done()
		s.dispatchLoop(runCtx)
	}()
	go func() {
		defer wg.Done()
		s.delayedStartupCatchUp(runCtx)
	}()

	<-ctx.Done()
	cancel()
	wg.Wait()
	return ctx.Err()
}

func (s *Service) prepareStartup() error {
	errs := make([]error, 0)
	for _, cfgJob := range s.configJobList() {
		if !watcher.DirectUpload(cfgJob.SourceDir, cfgJob.LinkDir) {
			if err := s.mkdirAll(cfgJob.LinkDir, 0o755); err != nil {
				errs = append(errs, s.recordStartupPrepareError(cfgJob, "创建链接目录", err))
				continue
			}
		}
		if err := s.addWatches(cfgJob.SourceDir); err != nil {
			errs = append(errs, s.recordStartupPrepareError(cfgJob, "添加目录监控", err))
			continue
		}
	}
	return errors.Join(errs...)
}

func (s *Service) delayedStartupCatchUp(ctx context.Context) {
	delay := s.cfg.StableDuration
	if delay > 0 {
		timer := time.NewTimer(delay)
		select {
		case <-ctx.Done():
			if !timer.Stop() {
				<-timer.C
			}
			return
		case <-timer.C:
		}
	}
	if err := s.startupCatchUp(ctx); err != nil && ctx.Err() == nil {
		s.logger.Printf("startup catch-up: %v", err)
	}
}

func (s *Service) startupCatchUp(ctx context.Context) error {
	errs := make([]error, 0)
	for _, cfgJob := range s.configJobList() {
		directUpload := watcher.DirectUpload(cfgJob.SourceDir, cfgJob.LinkDir)
		uploadRoot := cfgJob.LinkDir
		if directUpload {
			uploadRoot = cfgJob.SourceDir
		}
		scannedCount := 0
		var uploadFiles []string
		if directUpload {
			results, err := s.scanExistingUploadFiles(ctx, cfgJob, uploadRoot)
			if err != nil {
				if isContextError(err) || ctx.Err() != nil {
					return err
				}
				errs = append(errs, s.recordStartupCatchUpError(cfgJob, "扫描源目录", err))
			}
			scannedCount = len(results)
			uploadFiles = make([]string, 0, len(results))
			for _, result := range results {
				uploadFiles = append(uploadFiles, result.Path)
			}
		} else {
			count, err := s.scanExisting(ctx, cfgJob.SourceDir, cfgJob.LinkDir, s.cfg.Extensions, s.cfg.StableDuration, s.cfg.PollInterval)
			if err != nil {
				if isContextError(err) || ctx.Err() != nil {
					return err
				}
				errs = append(errs, s.recordStartupCatchUpError(cfgJob, "扫描源目录", err))
			}
			scannedCount = count
			scanLinkedFiles := s.scanLinkedFiles
			if scanLinkedFiles == nil {
				scanLinkedFiles = watcher.ScanLinkedFiles
			}
			linkFiles, err := scanLinkedFiles(uploadRoot, s.cfg.Extensions)
			if err != nil {
				if isContextError(err) || ctx.Err() != nil {
					return err
				}
				errs = append(errs, s.recordStartupCatchUpError(cfgJob, "扫描链接目录", err))
			}
			uploadFiles = linkFiles
		}
		for _, uploadPath := range uploadFiles {
			var task *jobRuntime
			var err error
			if directUpload {
				task, err = s.registerTask(cfgJob, uploadPath, uploadPath)
			} else {
				task, err = s.registerTaskByLinkPath(cfgJob, uploadPath)
			}
			if err != nil {
				if isContextError(err) || ctx.Err() != nil {
					return err
				}
				errs = append(errs, s.recordStartupCatchUpError(cfgJob, "注册上传任务", err))
				continue
			}
			s.scheduler.MarkDirty(task.key)
			s.runAfterMarkDirty(task.key)
		}
		if len(uploadFiles) > 0 {
			eventTask := &jobRuntime{cfg: cfgJob}
			if scannedCount > 0 {
				s.appendSchedulerEventNow(eventTask, fmt.Sprintf("启动扫描发现 %d 个文件，任务标记为待上传", scannedCount))
			} else {
				s.appendSchedulerEventNow(eventTask, fmt.Sprintf("链接目录发现 %d 个待上传文件，任务标记为待上传", len(uploadFiles)))
			}
		}
	}
	return errors.Join(errs...)
}

func (s *Service) recordStartupCatchUpError(cfgJob config.JobConfig, stage string, err error) error {
	wrapped := fmt.Errorf("[%s] startup catch-up %s failed: %w", cfgJob.Name, stage, err)
	s.logger.Print(wrapped)
	s.appendSchedulerEventNow(&jobRuntime{cfg: cfgJob}, fmt.Sprintf("启动补偿扫描失败：%s", err))
	return wrapped
}

func (s *Service) recordStartupPrepareError(cfgJob config.JobConfig, stage string, err error) error {
	wrapped := fmt.Errorf("[%s] startup prepare %s failed: %w", cfgJob.Name, stage, err)
	s.logger.Print(wrapped)
	s.appendSchedulerEventNow(&jobRuntime{cfg: cfgJob}, fmt.Sprintf("启动准备失败：%s", err))
	return wrapped
}

func isContextError(err error) bool {
	return errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded)
}

func (s *Service) scanExistingUploadFiles(ctx context.Context, cfgJob config.JobConfig, uploadRoot string) ([]watcher.LinkResult, error) {
	if s.scanExistingFiles != nil {
		return s.scanExistingFiles(ctx, cfgJob.SourceDir, uploadRoot, s.cfg.Extensions, s.cfg.StableDuration, s.cfg.PollInterval)
	}
	return watcher.ScanExistingAndLinkFilesContext(ctx, cfgJob.SourceDir, uploadRoot, s.cfg.Extensions, s.cfg.StableDuration, s.cfg.PollInterval)
}

func (s *Service) configJobList() []config.JobConfig {
	jobs := make([]config.JobConfig, 0, len(s.configJobs))
	for _, job := range s.configJobs {
		jobs = append(jobs, job)
	}
	return jobs
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
	cfgJob, ok := s.findJob(event.Name)
	if !ok {
		return
	}

	info, err := os.Stat(event.Name)
	if err == nil && info.IsDir() {
		if event.Op&(fsnotify.Create|fsnotify.Rename) != 0 {
			if err := s.addRecursiveWatches(event.Name); err != nil {
				s.logger.Printf("add recursive watch %s: %v", event.Name, err)
				return
			}
			s.processTree(ctx, cfgJob, event.Name)
		}
		return
	}

	if event.Op&(fsnotify.Create|fsnotify.Write|fsnotify.Rename) == 0 {
		return
	}
	if !hasAllowedExtension(event.Name, s.cfg.Extensions) {
		return
	}
	go s.processFile(ctx, cfgJob, event.Name)
}

func (s *Service) processTree(ctx context.Context, cfgJob config.JobConfig, root string) {
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
		go s.processFile(ctx, cfgJob, path)
		return nil
	})
}

func (s *Service) processFile(ctx context.Context, cfgJob config.JobConfig, path string) {
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

	waitStable := s.waitStable
	if waitStable == nil {
		waitStable = watcher.WaitStableContext
	}
	if err := waitStable(ctx, path, s.cfg.StableDuration, s.cfg.PollInterval); err != nil {
		s.logger.Printf("wait stable %s: %v", path, err)
		return
	}
	if !s.acquireProcessSlot(ctx) {
		return
	}
	defer s.releaseProcessSlot()
	if s.beforeProcessFile != nil {
		s.beforeProcessFile(path)
	}

	task, state, err := s.linkAndQueueTask(cfgJob, path)
	if err != nil {
		s.logger.Printf("queue task %s: %v", path, err)
		return
	}
	switch {
	case state.replacedRunning:
		s.appendSchedulerEventNow(task, supersededUploadMessage(state.linkState))
	case state.clearedRetryWait:
		s.appendSchedulerEventNow(task, "检测到同路径文件更新，已清除重试等待并重新排队")
	case !state.wasQueued:
		s.appendSchedulerEventNow(task, "检测到新文件，任务标记为待上传")
	}
	s.signalWake()
}

func supersededUploadMessage(linkState watcher.LinkState) string {
	if linkState == watcher.LinkReplacedDifferentFile {
		return "检测到同路径文件被替换，已更新硬链接并重新排队"
	}
	return "检测到上传中文件继续写入，已取消旧上传并重新排队"
}

func (s *Service) acquireProcessSlot(ctx context.Context) bool {
	if s.processSem == nil {
		return true
	}
	select {
	case s.processSem <- struct{}{}:
		return true
	case <-ctx.Done():
		return false
	}
}

func (s *Service) releaseProcessSlot() {
	if s.processSem == nil {
		return
	}
	<-s.processSem
}

func (s *Service) registerTask(cfgJob config.JobConfig, sourcePath, linkPath string) (*jobRuntime, error) {
	s.completionMu.Lock()
	defer s.completionMu.Unlock()

	task, _, err := s.registerTaskLocked(cfgJob, sourcePath, linkPath)
	if err != nil {
		return nil, err
	}
	return task, nil
}

func (s *Service) registerTaskLocked(cfgJob config.JobConfig, sourcePath, linkPath string) (*jobRuntime, queueTaskState, error) {
	remoteDir, err := rclone.BuildRemoteDir(cfgJob.SourceDir, cfgJob.RcloneRemote, sourcePath)
	if err != nil {
		return nil, queueTaskState{}, err
	}
	state := queueTaskState{
		wasQueued:        s.isJobReady(linkPath),
		clearedRetryWait: s.isRetryWaiting(linkPath),
	}

	var cancelRunning context.CancelCauseFunc

	s.mu.Lock()
	if s.jobs == nil {
		s.jobs = map[string]*jobRuntime{}
	}
	current := s.jobs[linkPath]
	task := current
	if current == nil || current.active {
		task = &jobRuntime{}
		s.jobs[linkPath] = task
	}
	if current != nil && current.active {
		state.replacedRunning = true
		cancelRunning = current.cancel
	}
	task.cfg = cfgJob
	task.key = linkPath
	task.sourcePath = sourcePath
	task.linkPath = linkPath
	task.remoteDir = remoteDir
	task.summary = ""
	task.active = false
	task.cancel = nil
	if state.clearedRetryWait {
		delete(s.retryDue, linkPath)
	}
	if state.clearedRetryWait || state.replacedRunning {
		delete(s.failureCounts, linkPath)
	}
	s.mu.Unlock()

	if cancelRunning != nil {
		cancelRunning(errUploadSuperseded)
	}
	return task, state, nil
}

func (s *Service) registerTaskByLinkPath(cfgJob config.JobConfig, linkPath string) (*jobRuntime, error) {
	sourcePath, err := sourcePathFromLinkedPath(cfgJob.SourceDir, cfgJob.LinkDir, linkPath)
	if err != nil {
		return nil, err
	}
	validateLinkedFile := s.validateLinkedFile
	if validateLinkedFile == nil {
		validateLinkedFile = watcher.ValidateLinkedFile
	}
	if err := validateLinkedFile(sourcePath, linkPath); err != nil {
		return nil, err
	}
	return s.registerTask(cfgJob, sourcePath, linkPath)
}

func (s *Service) linkAndQueueTask(cfgJob config.JobConfig, sourcePath string) (*jobRuntime, queueTaskState, error) {
	s.completionMu.Lock()
	defer s.completionMu.Unlock()

	linkResult, err := watcher.LinkFile(cfgJob.SourceDir, cfgJob.LinkDir, sourcePath)
	if err != nil {
		return nil, queueTaskState{}, err
	}
	linkPath := linkResult.Path
	resetFailuresForReplacement := s.shouldResetFailureCountForSamePathUpdate(linkPath)
	task, state, err := s.registerTaskLocked(cfgJob, sourcePath, linkPath)
	if err != nil {
		return nil, queueTaskState{}, err
	}
	state.linkState = linkResult.State
	if resetFailuresForReplacement {
		s.clearFailureCount(linkPath)
	}
	s.scheduler.MarkDirty(task.key)
	s.runAfterMarkDirty(task.key)
	return task, state, nil
}

func (s *Service) taskForKey(key string) *jobRuntime {
	s.mu.Lock()
	defer s.mu.Unlock()

	return s.jobs[key]
}

func (s *Service) activateTaskForUpload(parent context.Context, key string) (*jobRuntime, context.Context) {
	s.mu.Lock()
	defer s.mu.Unlock()

	job := s.jobs[key]
	if job == nil {
		return nil, nil
	}
	uploadCtx, cancel := context.WithCancelCause(parent)
	job.active = true
	job.summary = "等待 rclone 输出"
	job.cancel = cancel
	return job, uploadCtx
}

func (s *Service) removeTaskIfCompleted(job *jobRuntime) bool {
	if job == nil {
		return false
	}
	if !s.scheduler.Forget(job.key) {
		return false
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	current := s.jobs[job.key]
	if current == nil {
		delete(s.retryDue, job.key)
		return true
	}
	if current != job || current.active {
		return false
	}
	delete(s.jobs, job.key)
	delete(s.retryDue, job.key)
	return true
}

func sourcePathFromLinkedPath(sourceDir, linkDir, linkPath string) (string, error) {
	rel, err := filepath.Rel(filepath.Clean(linkDir), filepath.Clean(linkPath))
	if err != nil {
		return "", err
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) {
		return "", fmt.Errorf("linked file %q is outside link dir %q", linkPath, linkDir)
	}
	return filepath.Join(sourceDir, rel), nil
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
		if job := s.taskForKey(key); job != nil {
			s.scheduler.MarkDirty(key)
			s.appendSchedulerEventNow(job, "到达重试时间，重新排队")
			s.signalWake()
		}
	}
}

func (s *Service) startReadyUploads(ctx context.Context) {
	for _, key := range s.scheduler.Ready() {
		if !s.scheduler.TryStart(key) {
			continue
		}
		job, uploadCtx := s.activateTaskForUpload(ctx, key)
		if job == nil {
			s.scheduler.Finish(key, false)
			s.scheduler.Forget(key)
			continue
		}
		s.appendSchedulerEventNow(job, "调度开始上传")
		s.startUpload(uploadCtx, job)
	}
}

func (s *Service) runUpload(ctx context.Context, job *jobRuntime) {
	s.logRcloneCommand(job)
	err := s.copyJob(ctx, job)
	if err != nil {
		if errors.Is(context.Cause(ctx), errUploadSuperseded) {
			s.finishUploadSuperseded(job)
			return
		}
		s.finishUploadFailure(job, fmt.Sprintf("上传失败: %v", err))
		return
	}
	if errors.Is(context.Cause(ctx), errUploadSuperseded) {
		s.finishUploadSuperseded(job)
		return
	}

	s.completionMu.Lock()
	defer s.completionMu.Unlock()

	s.mu.Lock()
	job.active = false
	job.cancel = nil
	job.summary = "上传完成"
	s.mu.Unlock()

	if job.cfg.DeleteSourceAfterUpload {
		if err := s.cleanupUploadedSourceFile(job); err != nil {
			s.logger.Printf("cleanup source file %s (root %s): %v", job.sourcePath, job.cfg.SourceDir, err)
			s.finishUploadFailureLocked(job, fmt.Sprintf("清理源文件失败: %v", err))
			return
		}
	}

	if !watcher.DirectUpload(job.cfg.SourceDir, job.cfg.LinkDir) {
		if err := s.cleanupLinkedFile(job.cfg.LinkDir, job.linkPath); err != nil {
			if errors.Is(err, os.ErrNotExist) {
				s.logger.Printf("cleanup linked file %s (root %s): %v; linked file already removed, treating upload as complete", job.linkPath, job.cfg.LinkDir, err)
				s.finishUploadSuccessLocked(job)
				return
			}
			if _, statErr := os.Lstat(job.linkPath); errors.Is(statErr, os.ErrNotExist) {
				s.logger.Printf("cleanup linked file %s (root %s): %v; linked file already removed, treating upload as complete", job.linkPath, job.cfg.LinkDir, err)
				s.finishUploadSuccessLocked(job)
				return
			}
			s.logger.Printf("cleanup linked file %s (root %s): %v", job.linkPath, job.cfg.LinkDir, err)
			s.finishUploadFailureLocked(job, fmt.Sprintf("清理失败: %v", err))
			return
		}
	}

	s.finishUploadSuccessLocked(job)
}

func (s *Service) cleanupUploadedSourceFile(job *jobRuntime) error {
	cleanupSourceFile := s.cleanupSourceFile
	if cleanupSourceFile == nil {
		cleanupSourceFile = watcher.CleanupSourceFile
	}
	if err := cleanupSourceFile(job.cfg.SourceDir, job.sourcePath); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			s.logger.Printf("cleanup source file %s (root %s): %v; source file already removed, treating source cleanup as complete", job.sourcePath, job.cfg.SourceDir, err)
			return nil
		}
		if _, statErr := os.Lstat(job.sourcePath); errors.Is(statErr, os.ErrNotExist) {
			s.logger.Printf("cleanup source file %s (root %s): %v; source file already removed, treating source cleanup as complete", job.sourcePath, job.cfg.SourceDir, err)
			return nil
		}
		return err
	}
	return nil
}

func (s *Service) finishUploadFailure(job *jobRuntime, summary string) {
	s.completionMu.Lock()
	defer s.completionMu.Unlock()

	s.finishUploadFailureLocked(job, summary)
}

func (s *Service) clearFailureCount(key string) {
	if key == "" {
		return
	}
	s.mu.Lock()
	delete(s.failureCounts, key)
	s.mu.Unlock()
}

func (s *Service) incrementFailureCount(key string) int {
	if key == "" {
		return 0
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.failureCounts == nil {
		s.failureCounts = map[string]int{}
	}
	s.failureCounts[key]++
	return s.failureCounts[key]
}

func (s *Service) shouldResetFailureCountForSamePathUpdate(key string) bool {
	if key == "" {
		return false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.jobs[key]; !ok {
		return false
	}
	return s.failureCounts[key] > 0
}

func (s *Service) finishUploadFailureLocked(job *jobRuntime, summary string) {
	if job == nil {
		return
	}

	failures := s.incrementFailureCount(job.key)

	s.mu.Lock()
	job.active = false
	job.cancel = nil
	job.summary = summary
	current := s.jobs[job.key]
	if current != job {
		delete(s.failureCounts, job.key)
	}
	maxRetryCount := s.cfg.MaxRetryCount
	shouldRetry := current == job && (maxRetryCount == 0 || failures < maxRetryCount)
	if shouldRetry {
		s.retryDue[job.key] = s.currentTime().Add(s.cfg.RetryInterval)
	}
	s.mu.Unlock()

	if current == job && shouldRetry {
		s.scheduler.FinishFailed(job.key)
		s.appendSchedulerEventNow(job, "上传失败，进入重试等待")
	} else if current == job {
		s.scheduler.Finish(job.key, false)
		s.appendSchedulerEventNow(job, "上传失败，达到最大重试次数，停止重试")
		s.removeTerminalFailedTask(job)
		if err := s.notifyFinalFailure(jobFailureNotification{
			JobName:    job.cfg.Name,
			LinkPath:   job.linkPath,
			RetryCount: failures,
			LastError:  summary,
		}); err != nil && s.logger != nil {
			s.logger.Printf("notify telegram final failure for %s: %v", job.linkPath, err)
		}
	} else {
		s.scheduler.Finish(job.key, false)
	}
	s.signalWake()
}

func (s *Service) removeTerminalFailedTask(job *jobRuntime) bool {
	if job == nil {
		return false
	}
	if !s.scheduler.Forget(job.key) {
		return false
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	if current := s.jobs[job.key]; current != nil && current != job {
		return false
	}
	delete(s.jobs, job.key)
	delete(s.retryDue, job.key)
	delete(s.failureCounts, job.key)
	return true
}

func (s *Service) finishUploadSuperseded(job *jobRuntime) {
	s.completionMu.Lock()
	defer s.completionMu.Unlock()

	if job == nil {
		return
	}

	s.mu.Lock()
	job.active = false
	job.cancel = nil
	job.summary = "已被新文件替换"
	s.mu.Unlock()

	s.scheduler.Finish(job.key, true)
	s.signalWake()
}

func (s *Service) finishUploadSuccess(job *jobRuntime) {
	s.completionMu.Lock()
	defer s.completionMu.Unlock()

	s.finishUploadSuccessLocked(job)
}

func (s *Service) finishUploadSuccessLocked(job *jobRuntime) {
	s.clearFailureCount(job.key)
	s.scheduler.Finish(job.key, false)
	message := "上传完成，任务保留"
	if s.removeTaskIfCompleted(job) {
		message = "上传完成，任务清空"
	}
	s.appendSchedulerEventNow(job, message)
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
	return runner.CopyFile(ctx, job.linkPath, remoteDirWithTrailingSlash(job.remoteDir), s.cfg.RcloneArgs)
}

func (s *Service) logRcloneCommand(job *jobRuntime) {
	if s.logger == nil || job == nil {
		return
	}
	args := append([]string{"rclone", "copy", job.linkPath, remoteDirWithTrailingSlash(job.remoteDir)}, s.cfg.RcloneArgs...)
	s.logger.Printf("run rclone command for %s: %s", job.cfg.Name, formatShellCommand(args))
}

func remoteDirWithTrailingSlash(remoteDir string) string {
	if remoteDir == "" || strings.HasSuffix(remoteDir, "/") || strings.HasSuffix(remoteDir, ":") {
		return remoteDir
	}
	return remoteDir + "/"
}

func formatShellCommand(args []string) string {
	quoted := make([]string, 0, len(args))
	for _, arg := range args {
		quoted = append(quoted, shellQuoteArg(arg))
	}
	return strings.Join(quoted, " ")
}

func shellQuoteArg(arg string) string {
	if arg == "" {
		return "''"
	}
	if isShellSafeArg(arg) {
		return arg
	}
	return "'" + strings.ReplaceAll(arg, "'", `'\''`) + "'"
}

func isShellSafeArg(arg string) bool {
	for _, r := range arg {
		switch {
		case r >= 'a' && r <= 'z':
		case r >= 'A' && r <= 'Z':
		case r >= '0' && r <= '9':
		case strings.ContainsRune("@%_+=:,./-", r):
		default:
			return false
		}
	}
	return true
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
	state := uiRenderState{}
	state = s.renderDashboardIfNeeded(s.currentTime(), state, true)

	for {
		select {
		case <-ctx.Done():
			return
		case now := <-ticks:
			state = s.renderDashboardIfNeeded(now, state, false)
		case <-s.uiWakeCh:
			state = s.renderDashboardIfNeeded(s.currentTime(), state, false)
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
	width := ui.DetectWidth(s.uiWriter)
	if s.uiWidth != nil {
		width = s.uiWidth()
	}
	content := ui.RenderDashboardWithWidth(now, active, events, waiting, s.cfg.MaxParallelUploads, width)
	s.writeUI(ui.RewriteFrame(content))
}

func (s *Service) renderDashboardIfNeeded(now time.Time, previous uiRenderState, force bool) uiRenderState {
	active, events, waiting := s.snapshotUI()
	width := ui.DetectWidth(s.uiWriter)
	if s.uiWidth != nil {
		width = s.uiWidth()
	}

	fullRefresh := force || !previous.rendered || width != previous.width
	stateChanged := !previous.rendered ||
		waiting != previous.waiting ||
		!reflect.DeepEqual(active, previous.active) ||
		!reflect.DeepEqual(events, previous.events)
	keepaliveDue := !previous.rendered || !now.Before(previous.renderedAt.Add(uiKeepaliveInterval))

	if !fullRefresh && !stateChanged && !keepaliveDue {
		return previous
	}

	content := ui.RenderDashboardWithWidth(now, active, events, waiting, s.cfg.MaxParallelUploads, width)
	if fullRefresh {
		s.writeUI(ui.RewriteFrame(content))
	} else {
		s.writeUI(ui.RefreshFrame(content))
	}

	return uiRenderState{
		active:     active,
		events:     events,
		waiting:    waiting,
		width:      width,
		renderedAt: now,
		rendered:   true,
	}
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

	rows := s.snapshotUIRowsLocked()
	active := make([]ui.JobStatus, 0, len(rows))
	for _, row := range rows {
		active = append(active, row.status)
	}
	events := make([]ui.EventRecord, 0, len(s.recentEvents))
	for i := len(s.recentEvents) - 1; i >= 0; i-- {
		event := s.recentEvents[i]
		events = append(events, ui.EventRecord{At: event.at, Message: event.message})
	}
	return active, events, len(s.scheduler.Ready())
}

func (s *Service) snapshotUIRows() []uiActiveRow {
	s.mu.Lock()
	defer s.mu.Unlock()

	return s.snapshotUIRowsLocked()
}

func (s *Service) snapshotUIRowsLocked() []uiActiveRow {
	rows := make([]uiActiveRow, 0, len(s.jobs))
	for _, job := range s.jobs {
		if !job.active {
			continue
		}
		summary := job.summary
		if summary == "" {
			summary = "等待 rclone 输出"
		}
		rows = append(rows, uiActiveRow{
			status: ui.JobStatus{
				Name:    filepath.Base(job.linkPath),
				Summary: summary,
			},
			linkPath: job.linkPath,
			jobKey:   job.key,
		})
	}
	sort.Slice(rows, func(i, j int) bool {
		if rows[i].status.Name != rows[j].status.Name {
			return rows[i].status.Name < rows[j].status.Name
		}
		if rows[i].status.Summary != rows[j].status.Summary {
			return rows[i].status.Summary < rows[j].status.Summary
		}
		if rows[i].jobKey != rows[j].jobKey {
			return rows[i].jobKey < rows[j].jobKey
		}
		return rows[i].linkPath < rows[j].linkPath
	})
	return rows
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
	if name := filepath.Base(job.linkPath); name != "" && name != "." {
		s.appendRecentEvent(at, fmt.Sprintf("[%s] %s｜%s", job.cfg.Name, name, message))
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
	s.signalUIWake()
}

func (s *Service) handleRcloneOutputLine(job *jobRuntime, line string, now func() time.Time) {
	s.logger.Println(line)
	if stats, ok := rclone.ParseStats(line); ok {
		s.mu.Lock()
		job.summary = stats
		s.mu.Unlock()
		s.signalUIWake()
	}
}

func (s *Service) findJob(path string) (config.JobConfig, bool) {
	cleanPath := filepath.Clean(path)
	for _, job := range s.configJobList() {
		source := filepath.Clean(job.SourceDir)
		if cleanPath == source || strings.HasPrefix(cleanPath, source+string(os.PathSeparator)) {
			return job, true
		}
	}
	return config.JobConfig{}, false
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

func (s *Service) signalUIWake() {
	select {
	case s.uiWakeCh <- struct{}{}:
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
