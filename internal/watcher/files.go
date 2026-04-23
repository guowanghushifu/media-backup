package watcher

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"time"
)

var errLinkFileOutsideLinkDir = errors.New("link file is outside link dir")

func LinkFile(sourceDir, linkDir, sourceFile string) (string, error) {
	rel, err := filepath.Rel(sourceDir, sourceFile)
	if err != nil {
		return "", err
	}
	linkPath := filepath.Join(linkDir, rel)
	if err := os.MkdirAll(filepath.Dir(linkPath), 0o755); err != nil {
		return "", err
	}
	if err := os.Link(sourceFile, linkPath); err != nil {
		if errors.Is(err, os.ErrExist) {
			return linkPath, nil
		}
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

func CleanupLinkedFile(linkDir, linkFile string) error {
	cleanRoot := filepath.Clean(linkDir)
	cleanFile := filepath.Clean(linkFile)

	rel, err := filepath.Rel(cleanRoot, cleanFile)
	if err != nil {
		return err
	}
	if rel == "." || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return errLinkFileOutsideLinkDir
	}

	if err := os.Remove(cleanFile); err != nil {
		return err
	}

	current := filepath.Dir(cleanFile)
	for {
		cleanCurrent := filepath.Clean(current)
		if cleanCurrent == cleanRoot {
			return nil
		}
		if !strings.HasPrefix(cleanCurrent, cleanRoot+string(filepath.Separator)) {
			return nil
		}

		entries, err := os.ReadDir(cleanCurrent)
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				current = filepath.Dir(cleanCurrent)
				continue
			}
			return err
		}
		if len(entries) != 0 {
			return nil
		}
		if err := os.Remove(cleanCurrent); err != nil {
			if errors.Is(err, os.ErrNotExist) {
				current = filepath.Dir(cleanCurrent)
				continue
			}
			return err
		}
		current = filepath.Dir(cleanCurrent)
	}
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
