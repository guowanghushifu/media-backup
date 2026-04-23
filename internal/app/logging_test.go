package app_test

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/guowanghushifu/media-backup/internal/app"
)

func TestDailyLogWriterCreatesCurrentDayFile(t *testing.T) {
	t.Parallel()

	dir := filepath.Join(t.TempDir(), "logs")
	now := time.Date(2026, 4, 23, 10, 0, 0, 0, time.UTC)

	writer := app.NewDailyLogWriter(dir, func() time.Time { return now })
	t.Cleanup(func() { _ = writer.Close() })

	if _, err := writer.Write([]byte("hello\n")); err != nil {
		t.Fatalf("Write() error = %v", err)
	}

	path := filepath.Join(dir, "media-backup-2026-04-23.log")
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("log file missing: %v", err)
	}
}

func TestDailyLogWriterRotatesWhenDateChanges(t *testing.T) {
	t.Parallel()

	dir := filepath.Join(t.TempDir(), "logs")
	current := time.Date(2026, 4, 23, 23, 59, 0, 0, time.UTC)

	writer := app.NewDailyLogWriter(dir, func() time.Time { return current })
	t.Cleanup(func() { _ = writer.Close() })

	if _, err := writer.Write([]byte("before\n")); err != nil {
		t.Fatalf("first Write() error = %v", err)
	}

	current = time.Date(2026, 4, 24, 0, 1, 0, 0, time.UTC)
	if _, err := writer.Write([]byte("after\n")); err != nil {
		t.Fatalf("second Write() error = %v", err)
	}

	for _, name := range []string{
		"media-backup-2026-04-23.log",
		"media-backup-2026-04-24.log",
	} {
		if _, err := os.Stat(filepath.Join(dir, name)); err != nil {
			t.Fatalf("expected rotated file %s: %v", name, err)
		}
	}
}

func TestDailyLogWriterKeepsLatestSevenDays(t *testing.T) {
	t.Parallel()

	dir := filepath.Join(t.TempDir(), "logs")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	for day := 14; day <= 23; day++ {
		name := fmt.Sprintf("media-backup-2026-04-%02d.log", day)
		if err := os.WriteFile(filepath.Join(dir, name), []byte("old\n"), 0o644); err != nil {
			t.Fatalf("WriteFile(%s) error = %v", name, err)
		}
	}
	if err := os.WriteFile(filepath.Join(dir, "notes.txt"), []byte("keep\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(notes.txt) error = %v", err)
	}

	writer := app.NewDailyLogWriter(dir, func() time.Time {
		return time.Date(2026, 4, 23, 10, 0, 0, 0, time.UTC)
	})
	t.Cleanup(func() { _ = writer.Close() })

	if _, err := writer.Write([]byte("today\n")); err != nil {
		t.Fatalf("Write() error = %v", err)
	}

	for day := 17; day <= 23; day++ {
		name := fmt.Sprintf("media-backup-2026-04-%02d.log", day)
		if _, err := os.Stat(filepath.Join(dir, name)); err != nil {
			t.Fatalf("expected kept log %s: %v", name, err)
		}
	}
	for day := 14; day <= 16; day++ {
		name := fmt.Sprintf("media-backup-2026-04-%02d.log", day)
		if _, err := os.Stat(filepath.Join(dir, name)); !os.IsNotExist(err) {
			t.Fatalf("expected old log %s to be removed, stat err = %v", name, err)
		}
	}
	if _, err := os.Stat(filepath.Join(dir, "notes.txt")); err != nil {
		t.Fatalf("expected unrelated file to remain: %v", err)
	}
}
