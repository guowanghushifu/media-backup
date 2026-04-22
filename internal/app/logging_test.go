package app_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/wangdazhuo/media-backup/internal/app"
)

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
