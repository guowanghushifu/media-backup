package watcher

import (
	"os"
	"path/filepath"
	"time"
)

func LinkFile(sourceDir, linkDir, sourceFile string) (string, error) {
	sourcePath := filepath.Join(sourceDir, sourceFile)
	linkPath := filepath.Join(linkDir, sourceFile)
	if err := os.MkdirAll(filepath.Dir(linkPath), 0o755); err != nil {
		return "", err
	}
	if err := os.Link(sourcePath, linkPath); err != nil {
		return "", err
	}
	return linkPath, nil
}

func CleanupLinkDir(linkDir string) error {
	entries, err := os.ReadDir(linkDir)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if err := os.RemoveAll(filepath.Join(linkDir, entry.Name())); err != nil {
			return err
		}
	}
	return nil
}

func WaitStable(path string, stableFor time.Duration, pollInterval time.Duration) error {
	if pollInterval <= 0 {
		pollInterval = 100 * time.Millisecond
	}

	info, err := os.Stat(path)
	if err != nil {
		return err
	}

	lastSize := info.Size()
	if stableFor <= 0 {
		return nil
	}

	stableSince := time.Now()
	for {
		time.Sleep(pollInterval)

		info, err := os.Stat(path)
		if err != nil {
			return err
		}

		size := info.Size()
		if size != lastSize {
			lastSize = size
			stableSince = time.Now()
			continue
		}
		if time.Since(stableSince) >= stableFor {
			return nil
		}
	}
}
