package watcher

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"time"
)

var waitForScanStable = func(ctx context.Context, path string, stableFor time.Duration, pollInterval time.Duration) error {
	return WaitStableContext(ctx, path, stableFor, pollInterval)
}

const defaultScanPollInterval = time.Second

func ScanAndLink(sourceDir, linkDir string, extensions []string, stableDuration time.Duration) (int, error) {
	return ScanAndLinkContext(context.Background(), sourceDir, linkDir, extensions, stableDuration, defaultScanPollInterval)
}

func ScanAndLinkContext(ctx context.Context, sourceDir, linkDir string, extensions []string, stableDuration time.Duration, pollInterval time.Duration) (int, error) {
	results, err := scanAndLink(sourceDir, linkDir, extensions, func(path string) error {
		return waitForScanStable(ctx, path, stableDuration, pollInterval)
	})
	return len(results), err
}

func ScanExistingAndLink(sourceDir, linkDir string, extensions []string, stableDuration time.Duration) (int, error) {
	return ScanExistingAndLinkContext(context.Background(), sourceDir, linkDir, extensions, stableDuration, defaultScanPollInterval)
}

func ScanExistingAndLinkContext(ctx context.Context, sourceDir, linkDir string, extensions []string, stableDuration time.Duration, pollInterval time.Duration) (int, error) {
	results, err := ScanExistingAndLinkFilesContext(ctx, sourceDir, linkDir, extensions, stableDuration, pollInterval)
	return len(results), err
}

func ScanExistingAndLinkFilesContext(ctx context.Context, sourceDir, linkDir string, extensions []string, stableDuration time.Duration, pollInterval time.Duration) ([]LinkResult, error) {
	return scanAndLink(sourceDir, linkDir, extensions, func(path string) error {
		info, err := os.Stat(path)
		if err != nil {
			return err
		}
		if stableDuration > 0 && time.Since(info.ModTime()) < stableDuration {
			return errSkipUnstableFile
		}
		return nil
	})
}

func ScanLinkedFiles(root string, extensions []string) ([]string, error) {
	paths := make([]string, 0)
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		if !hasExtension(path, extensions) {
			return nil
		}
		paths = append(paths, path)
		return nil
	})
	if err != nil {
		return nil, err
	}
	return paths, nil
}

func scanAndLink(sourceDir, linkDir string, extensions []string, beforeLink func(path string) error) ([]LinkResult, error) {
	results := make([]LinkResult, 0)
	cleanLinkDir := filepath.Clean(linkDir)
	err := filepath.WalkDir(sourceDir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if !DirectUpload(sourceDir, linkDir) && filepath.Clean(path) == cleanLinkDir {
				return filepath.SkipDir
			}
			return nil
		}
		if !hasExtension(path, extensions) {
			return nil
		}
		if beforeLink != nil {
			if err := beforeLink(path); err != nil {
				if errors.Is(err, errSkipUnstableFile) {
					return nil
				}
				return err
			}
		}
		result, err := LinkFile(sourceDir, linkDir, path)
		if err != nil {
			return err
		}
		results = append(results, result)
		return nil
	})
	return results, err
}

var errSkipUnstableFile = errors.New("skip unstable file")

func hasExtension(path string, extensions []string) bool {
	ext := strings.ToLower(filepath.Ext(path))
	for _, want := range extensions {
		if ext == strings.ToLower(want) {
			return true
		}
	}
	return false
}
