package app

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/wangdazhuo/media-backup/internal/config"
	"github.com/wangdazhuo/media-backup/internal/queue"
	"github.com/wangdazhuo/media-backup/internal/rclone"
	"github.com/wangdazhuo/media-backup/internal/ui"
	"github.com/wangdazhuo/media-backup/internal/watcher"
)

type Service struct {
	cfg       *config.Config
	logger    *log.Logger
	scheduler *queue.Scheduler
	watcher   *fsnotify.Watcher

	mu         sync.Mutex
	jobs       map[string]*jobRuntime
	processing map[string]struct{}
	retryDue   map[string]time.Time
	wakeCh     chan struct{}
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
		cfg:        cfg,
		logger:     logger,
		scheduler:  queue.New(queue.Options{MaxParallel: cfg.MaxParallelUploads, RetryInterval: cfg.RetryInterval}),
		watcher:    fsWatcher,
		jobs:       make(map[string]*jobRuntime, len(cfg.Jobs)),
		processing: map[string]struct{}{},
		retryDue:   map[string]time.Time{},
		wakeCh:     make(chan struct{}, 1),
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

	for _, job := range s.jobs {
		if err := os.MkdirAll(job.cfg.LinkDir, 0o755); err != nil {
			return err
		}
		if err := s.addRecursiveWatches(job.cfg.SourceDir); err != nil {
			return err
		}
		count, err := watcher.ScanAndLink(job.cfg.SourceDir, job.cfg.LinkDir, s.cfg.Extensions, s.cfg.StableDuration)
		if err != nil {
			return err
		}
		if count > 0 {
			s.scheduler.MarkDirty(job.key)
		}
	}

	var wg sync.WaitGroup
	wg.Add(3)
	go func() {
		defer wg.Done()
		s.eventLoop(ctx)
	}()
	go func() {
		defer wg.Done()
		s.dispatchLoop(ctx)
	}()
	go func() {
		defer wg.Done()
		s.uiLoop(ctx)
	}()

	<-ctx.Done()
	wg.Wait()
	return ctx.Err()
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
	if job.active {
		job.dirtyDuringRun = true
	}
	s.mu.Unlock()

	s.scheduler.MarkDirty(job.key)
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
	now := time.Now()
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
		go s.runUpload(ctx, job)
	}
}

func (s *Service) runUpload(ctx context.Context, job *jobRuntime) {
	exec := &rclone.CommandExecutor{
		OnOutput: func(line string) {
			s.logger.Println(line)
			if stats, ok := rclone.ParseStats(line); ok {
				s.mu.Lock()
				job.summary = stats
				s.mu.Unlock()
			}
		},
	}
	runner := rclone.NewRunner(exec)
	err := runner.Copy(ctx, job.cfg.LinkDir, job.cfg.RcloneRemote, s.cfg.RcloneArgs)
	if err != nil {
		s.mu.Lock()
		job.active = false
		job.summary = fmt.Sprintf("上传失败: %v", err)
		s.retryDue[job.key] = time.Now().Add(s.cfg.RetryInterval)
		s.mu.Unlock()
		s.scheduler.FinishFailed(job.key)
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
		s.signalWake()
		return
	}

	if err := watcher.CleanupLinkDir(job.cfg.LinkDir); err != nil {
		s.logger.Printf("cleanup %s: %v", job.cfg.LinkDir, err)
		s.mu.Lock()
		job.summary = fmt.Sprintf("清理失败: %v", err)
		s.retryDue[job.key] = time.Now().Add(s.cfg.RetryInterval)
		s.mu.Unlock()
		s.scheduler.FinishFailed(job.key)
		s.signalWake()
		return
	}

	s.scheduler.Finish(job.key, false)
	s.signalWake()
}

func (s *Service) uiLoop(ctx context.Context) {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case now := <-ticker.C:
			active, waiting := s.snapshotUI()
			if len(active) == 0 {
				fmt.Println(ui.RenderIdle(now))
				continue
			}
			fmt.Println(ui.RenderDashboard(now, active, waiting, s.cfg.MaxParallelUploads))
		}
	}
}

func (s *Service) snapshotUI() ([]ui.JobStatus, int) {
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
	return active, len(s.scheduler.Ready())
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

func hasAllowedExtension(path string, extensions []string) bool {
	ext := strings.ToLower(filepath.Ext(path))
	for _, want := range extensions {
		if ext == strings.ToLower(want) {
			return true
		}
	}
	return false
}
