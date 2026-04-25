package watcher

import (
	"context"
	"errors"
	"fmt"
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
	errs := make([]error, 0)
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", path, err))
			return nil
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
		errs = append(errs, err)
	}
	return paths, errors.Join(errs...)
}

func scanAndLink(sourceDir, linkDir string, extensions []string, beforeLink func(path string) error) ([]LinkResult, error) {
	results := make([]LinkResult, 0)
	cleanLinkDir := filepath.Clean(linkDir)
	errs := make([]error, 0)
	err := filepath.WalkDir(sourceDir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", path, err))
			return nil
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
				if isContextError(err) {
					return err
				}
				errs = append(errs, fmt.Errorf("%s: %w", path, err))
				return nil
			}
		}
		result, err := LinkFile(sourceDir, linkDir, path)
		if err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", path, err))
			return nil
		}
		results = append(results, result)
		return nil
	})
	if err != nil {
		errs = append(errs, err)
	}
	return results, errors.Join(errs...)
}

var errSkipUnstableFile = errors.New("skip unstable file")

func isContextError(err error) bool {
	return errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded)
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
