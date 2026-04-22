package watcher

import (
	"os"
	"path/filepath"
	"strings"
	"time"
)

func ScanAndLink(sourceDir, linkDir string, extensions []string, stableDuration time.Duration) (int, error) {
	var count int
	cleanLinkDir := filepath.Clean(linkDir)
	err := filepath.WalkDir(sourceDir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if filepath.Clean(path) == cleanLinkDir {
				return filepath.SkipDir
			}
			return nil
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
	for _, want := range extensions {
		if ext == strings.ToLower(want) {
			return true
		}
	}
	return false
}
