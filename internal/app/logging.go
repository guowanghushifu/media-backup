package app

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

const (
	logFilePrefix = "media-backup-"
	logFileSuffix = ".log"
	logDateLayout = "2006-01-02"
	logRetention  = 7
)

type dailyLogWriter struct {
	mu         sync.Mutex
	dir        string
	now        func() time.Time
	currentDay string
	file       *os.File
}

func NewDailyLogWriter(dir string, now func() time.Time) io.WriteCloser {
	if now == nil {
		now = time.Now
	}
	return &dailyLogWriter{
		dir: dir,
		now: now,
	}
}

func (w *dailyLogWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()

	if err := w.rotateIfNeededLocked(); err != nil {
		return 0, err
	}
	return w.file.Write(p)
}

func (w *dailyLogWriter) Close() error {
	w.mu.Lock()
	defer w.mu.Unlock()

	if w.file == nil {
		return nil
	}
	err := w.file.Close()
	w.file = nil
	w.currentDay = ""
	return err
}

func (w *dailyLogWriter) rotateIfNeededLocked() error {
	now := w.now()
	day := now.Format(logDateLayout)
	if w.file != nil && w.currentDay == day {
		return nil
	}

	if err := os.MkdirAll(w.dir, 0o755); err != nil {
		return err
	}

	file, err := os.OpenFile(filepath.Join(w.dir, logFilename(day)), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}

	if err := w.cleanupExpiredLocked(now); err != nil {
		_ = file.Close()
		return err
	}

	if w.file != nil {
		if err := w.file.Close(); err != nil {
			_ = file.Close()
			return err
		}
	}

	w.file = file
	w.currentDay = day
	return nil
}

func (w *dailyLogWriter) cleanupExpiredLocked(now time.Time) error {
	entries, err := os.ReadDir(w.dir)
	if err != nil {
		return err
	}

	cutoff := now.AddDate(0, 0, -(logRetention - 1))
	cutoffDay := time.Date(cutoff.Year(), cutoff.Month(), cutoff.Day(), 0, 0, 0, 0, cutoff.Location())

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		day, ok := parseLogDay(entry.Name())
		if !ok {
			continue
		}

		if day.Before(cutoffDay) {
			if err := os.Remove(filepath.Join(w.dir, entry.Name())); err != nil && !os.IsNotExist(err) {
				return err
			}
		}
	}

	return nil
}

func logFilename(day string) string {
	return logFilePrefix + day + logFileSuffix
}

func parseLogDay(name string) (time.Time, bool) {
	if !strings.HasPrefix(name, logFilePrefix) || !strings.HasSuffix(name, logFileSuffix) {
		return time.Time{}, false
	}

	day := strings.TrimSuffix(strings.TrimPrefix(name, logFilePrefix), logFileSuffix)
	parsed, err := time.Parse(logDateLayout, day)
	if err != nil {
		return time.Time{}, false
	}
	return parsed, true
}
