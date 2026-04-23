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

type Service struct {
	cfg               *config.Config
	logger            *log.Logger
	scheduler         *queue.Scheduler
	watcher           *fsnotify.Watcher
	uiWriter          io.Writer
	mkdirAll          func(string, os.FileMode) error
	addWatches        func(string) error
	scanExisting      func(string, string, []string, time.Duration) (int, error)
	scanLinkedFiles   func(string, []string) ([]string, error)
	copyJob           func(context.Context, *jobRuntime) error
	cleanupLinkedFile func(string, string) error
	startUpload       func(context.Context, *jobRuntime)
	afterMarkDirty    func(string)
	now               func() time.Time
	uiWidth           func() int

	mu           sync.Mutex
	configJobs   map[string]config.JobConfig
	jobs         map[string]*jobRuntime
	processing   map[string]struct{}
	recentEvents []recentEvent
	retryDue     map[string]time.Time
	wakeCh       chan struct{}
	uiWakeCh     chan struct{}
}

type uiRenderState struct {
	active     []ui.JobStatus
	events     []ui.EventRecord
	waiting    int
	width      int
	renderedAt time.Time
	rendered   bool
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
}

func NewService(cfg *config.Config, logger *log.Logger) (*Service, error) {
	fsWatcher, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, err
	}

	s := &Service{
		cfg:               cfg,
		logger:            logger,
		scheduler:         queue.New(queue.Options{MaxParallel: cfg.MaxParallelUploads, RetryInterval: cfg.RetryInterval}),
		watcher:           fsWatcher,
		uiWriter:          os.Stdout,
		mkdirAll:          os.MkdirAll,
		scanExisting:      watcher.ScanExistingAndLink,
		scanLinkedFiles:   watcher.ScanLinkedFiles,
		cleanupLinkedFile: watcher.CleanupLinkedFile,
		now:               time.Now,
		configJobs:        make(map[string]config.JobConfig, len(cfg.Jobs)),
		jobs:              make(map[string]*jobRuntime),
		processing:        map[string]struct{}{},
		retryDue:          map[string]time.Time{},
		wakeCh:            make(chan struct{}, 1),
		uiWakeCh:          make(chan struct{}, 1),
	}
	s.addWatches = s.addRecursiveWatches
	s.copyJob = s.copyWithRclone
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
	for _, cfgJob := range s.configJobList() {
		if err := s.mkdirAll(cfgJob.LinkDir, 0o755); err != nil {
			return err
		}
		if err := s.addWatches(cfgJob.SourceDir); err != nil {
			return err
		}
		scannedCount, err := s.scanExisting(cfgJob.SourceDir, cfgJob.LinkDir, s.cfg.Extensions, s.cfg.StableDuration)
		if err != nil {
			return err
		}
		scanLinkedFiles := s.scanLinkedFiles
		if scanLinkedFiles == nil {
			scanLinkedFiles = watcher.ScanLinkedFiles
		}
		linkFiles, err := scanLinkedFiles(cfgJob.LinkDir, s.cfg.Extensions)
		if err != nil {
			return err
		}
		for _, linkPath := range linkFiles {
			task, err := s.registerTaskByLinkPath(cfgJob, linkPath)
			if err != nil {
				return err
			}
			s.scheduler.MarkDirty(task.key)
			s.runAfterMarkDirty(task.key)
		}
		if len(linkFiles) > 0 {
			eventTask := &jobRuntime{cfg: cfgJob}
			if scannedCount > 0 {
				s.appendSchedulerEventNow(eventTask, fmt.Sprintf("启动扫描发现 %d 个文件，任务标记为待上传", scannedCount))
			} else {
				s.appendSchedulerEventNow(eventTask, fmt.Sprintf("链接目录发现 %d 个待上传文件，任务标记为待上传", len(linkFiles)))
			}
		}
	}
	return nil
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

	if err := watcher.WaitStable(path, s.cfg.StableDuration, s.cfg.PollInterval); err != nil {
		s.logger.Printf("wait stable %s: %v", path, err)
		return
	}
	linkPath, err := watcher.LinkFile(cfgJob.SourceDir, cfgJob.LinkDir, path)
	if err != nil {
		s.logger.Printf("link file %s: %v", path, err)
		return
	}

	task, err := s.registerTask(cfgJob, path, linkPath)
	if err != nil {
		s.logger.Printf("register task %s: %v", path, err)
		return
	}

	wasQueued := s.isJobReady(task.key)
	wasPendingRetry := s.isRetryWaiting(task.key)
	s.scheduler.MarkDirty(task.key)
	s.runAfterMarkDirty(task.key)
	if !wasQueued && !wasPendingRetry {
		s.appendSchedulerEventNow(task, "检测到新文件，任务标记为待上传")
	}
	s.signalWake()
}

func (s *Service) registerTask(cfgJob config.JobConfig, sourcePath, linkPath string) (*jobRuntime, error) {
	remoteDir, err := rclone.BuildRemoteDir(cfgJob.SourceDir, cfgJob.RcloneRemote, sourcePath)
	if err != nil {
		return nil, err
	}
	ready := s.isJobReady(linkPath)
	pendingRetry := s.isRetryWaiting(linkPath)

	s.mu.Lock()
	defer s.mu.Unlock()

	if s.jobs == nil {
		s.jobs = map[string]*jobRuntime{}
	}
	task := s.jobs[linkPath]
	if task == nil || task.active || (!ready && !pendingRetry) {
		task = &jobRuntime{}
		s.jobs[linkPath] = task
	}
	task.cfg = cfgJob
	task.key = linkPath
	task.sourcePath = sourcePath
	task.linkPath = linkPath
	task.remoteDir = remoteDir
	return task, nil
}

func (s *Service) registerTaskByLinkPath(cfgJob config.JobConfig, linkPath string) (*jobRuntime, error) {
	sourcePath, err := sourcePathFromLinkedPath(cfgJob.SourceDir, cfgJob.LinkDir, linkPath)
	if err != nil {
		return nil, err
	}
	return s.registerTask(cfgJob, sourcePath, linkPath)
}

func (s *Service) taskForKey(key string) *jobRuntime {
	s.mu.Lock()
	defer s.mu.Unlock()

	return s.jobs[key]
}

func (s *Service) activateTaskForUpload(key string) *jobRuntime {
	s.mu.Lock()
	defer s.mu.Unlock()

	job := s.jobs[key]
	if job == nil {
		return nil
	}
	job.active = true
	job.summary = "等待 rclone 输出"
	return job
}

func (s *Service) removeTaskIfCompleted(job *jobRuntime) {
	if job == nil {
		return
	}
	if !s.scheduler.Forget(job.key) {
		return
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	current := s.jobs[job.key]
	if current == nil || current != job || current.active {
		return
	}
	delete(s.jobs, job.key)
	delete(s.retryDue, job.key)
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
		if s.scheduler.RetryJob(key) {
			if job := s.taskForKey(key); job != nil {
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
		job := s.activateTaskForUpload(key)
		if job == nil {
			s.scheduler.Finish(key, false)
			s.scheduler.Forget(key)
			continue
		}
		s.appendSchedulerEventNow(job, "调度开始上传")
		s.startUpload(ctx, job)
	}
}

func (s *Service) runUpload(ctx context.Context, job *jobRuntime) {
	s.logRcloneCommand(job)
	uploadedLinkIdentity, identityErr := linkedFileIdentity(job.linkPath)
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
	job.active = false
	job.summary = "上传完成"
	s.mu.Unlock()

	if identityErr != nil {
		s.logger.Printf("stat linked file %s before upload: %v; skipping cleanup", job.linkPath, identityErr)
		s.finishUploadSuccess(job)
		return
	}

	sameLinkedFile, err := linkedFileMatchesIdentity(uploadedLinkIdentity, job.linkPath)
	if err != nil {
		s.logger.Printf("check linked file identity %s: %v", job.linkPath, err)
		s.mu.Lock()
		job.summary = fmt.Sprintf("清理失败: %v", err)
		s.retryDue[job.key] = s.currentTime().Add(s.cfg.RetryInterval)
		s.mu.Unlock()
		s.scheduler.FinishFailed(job.key)
		s.appendSchedulerEventNow(job, "上传失败，进入重试等待")
		s.signalWake()
		return
	}
	if !sameLinkedFile {
		s.finishUploadSuccess(job)
		return
	}

	if err := s.cleanupLinkedFile(job.cfg.LinkDir, job.linkPath); err != nil {
		if missing, statErr := linkedFileMissing(job.linkPath); statErr == nil && missing {
			s.logger.Printf("cleanup linked file %s (root %s): %v; linked file already removed, treating upload as complete", job.linkPath, job.cfg.LinkDir, err)
			s.finishUploadSuccess(job)
			return
		}
		s.logger.Printf("cleanup linked file %s (root %s): %v", job.linkPath, job.cfg.LinkDir, err)
		s.mu.Lock()
		job.summary = fmt.Sprintf("清理失败: %v", err)
		s.retryDue[job.key] = s.currentTime().Add(s.cfg.RetryInterval)
		s.mu.Unlock()
		s.scheduler.FinishFailed(job.key)
		s.appendSchedulerEventNow(job, "上传失败，进入重试等待")
		s.signalWake()
		return
	}

	s.finishUploadSuccess(job)
}

func (s *Service) finishUploadSuccess(job *jobRuntime) {
	s.scheduler.Finish(job.key, false)
	s.removeTaskIfCompleted(job)
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

func linkedFileMissing(path string) (bool, error) {
	_, err := os.Lstat(path)
	if err == nil {
		return false, nil
	}
	if errors.Is(err, os.ErrNotExist) {
		return true, nil
	}
	return false, err
}

func linkedFileIdentity(path string) (os.FileInfo, error) {
	return os.Stat(path)
}

func linkedFileMatchesIdentity(identity os.FileInfo, path string) (bool, error) {
	if identity == nil {
		return false, nil
	}

	current, err := os.Stat(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return false, nil
		}
		return false, err
	}
	return os.SameFile(identity, current), nil
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

	active := make([]ui.JobStatus, 0, len(s.jobs))
	for _, job := range s.jobs {
		if !job.active {
			continue
		}
		summary := job.summary
		if summary == "" {
			summary = "等待 rclone 输出"
		}
		active = append(active, ui.JobStatus{Name: filepath.Base(job.linkPath), Summary: summary})
	}
	sort.Slice(active, func(i, j int) bool {
		if active[i].Name == active[j].Name {
			return active[i].Summary < active[j].Summary
		}
		return active[i].Name < active[j].Name
	})
	events := make([]ui.EventRecord, 0, len(s.recentEvents))
	for i := len(s.recentEvents) - 1; i >= 0; i-- {
		event := s.recentEvents[i]
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
	s.signalUIWake()
}

func (s *Service) handleRcloneOutputLine(job *jobRuntime, line string, now func() time.Time) {
	s.logger.Println(line)
	if stats, ok := rclone.ParseStats(line); ok {
		s.mu.Lock()
		job.summary = stats
		s.mu.Unlock()
		s.signalUIWake()
		return
	}
	if payload, ok := rclone.ParseEvent(line); ok {
		s.appendRecentEventNow(payload, now)
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
