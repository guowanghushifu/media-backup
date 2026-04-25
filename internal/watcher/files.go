package watcher

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

var errLinkFileOutsideLinkDir = errors.New("link file is outside link dir")

var ErrWaitStableTimeout = errors.New("file did not become stable before timeout")

var stableWaitGrace = time.Minute

type LinkState int

const (
	LinkCreated LinkState = iota
	LinkAlreadySameFile
	LinkReplacedDifferentFile
)

type LinkResult struct {
	Path  string
	State LinkState
}

func LinkFile(sourceDir, linkDir, sourceFile string) (LinkResult, error) {
	rel, err := filepath.Rel(sourceDir, sourceFile)
	if err != nil {
		return LinkResult{}, err
	}
	linkPath := filepath.Join(linkDir, rel)
	if err := os.MkdirAll(filepath.Dir(linkPath), 0o755); err != nil {
		return LinkResult{}, err
	}
	if err := os.Link(sourceFile, linkPath); err != nil {
		if errors.Is(err, os.ErrExist) {
			same, err := sameFile(sourceFile, linkPath)
			if err != nil {
				return LinkResult{}, err
			}
			if same {
				return LinkResult{Path: linkPath, State: LinkAlreadySameFile}, nil
			}
			if err := replaceHardLink(sourceFile, linkPath); err != nil {
				return LinkResult{}, err
			}
			return LinkResult{Path: linkPath, State: LinkReplacedDifferentFile}, nil
		}
		return LinkResult{}, err
	}
	return LinkResult{Path: linkPath, State: LinkCreated}, nil
}

func sameFile(sourceFile, targetPath string) (bool, error) {
	sourceInfo, err := os.Stat(sourceFile)
	if err != nil {
		return false, err
	}

	targetInfo, err := os.Stat(targetPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return false, nil
		}
		return false, err
	}

	return os.SameFile(sourceInfo, targetInfo), nil
}

func replaceHardLink(sourceFile, targetPath string) error {
	dir := filepath.Dir(targetPath)
	base := filepath.Base(targetPath)

	for attempt := 0; attempt < 10; attempt++ {
		tmpPath := filepath.Join(dir, "."+base+".tmp."+strconv.FormatInt(time.Now().UnixNano(), 36)+"."+strconv.Itoa(attempt))
		if err := os.Link(sourceFile, tmpPath); err != nil {
			if errors.Is(err, os.ErrExist) {
				continue
			}
			return err
		}

		if err := os.Rename(tmpPath, targetPath); err != nil {
			_ = os.Remove(tmpPath)
			return err
		}
		return nil
	}

	return os.ErrExist
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
	return WaitStableContext(context.Background(), path, stableFor, pollInterval)
}

func WaitStableContext(ctx context.Context, path string, stableFor time.Duration, pollInterval time.Duration) error {
	if pollInterval <= 0 {
		pollInterval = 100 * time.Millisecond
	}
	if err := ctx.Err(); err != nil {
		return err
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
	deadline := stableSince.Add(stableFor + stableWaitGrace)
	for {
		if !deadline.IsZero() && time.Now().Add(pollInterval).After(deadline) {
			pollInterval = time.Until(deadline)
			if pollInterval <= 0 {
				return fmt.Errorf("%w: %s after %s", ErrWaitStableTimeout, path, stableFor+stableWaitGrace)
			}
		}
		timer := time.NewTimer(pollInterval)
		select {
		case <-ctx.Done():
			if !timer.Stop() {
				<-timer.C
			}
			return ctx.Err()
		case <-timer.C:
		}

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
		if !time.Now().Before(deadline) {
			return fmt.Errorf("%w: %s after %s", ErrWaitStableTimeout, path, stableFor+stableWaitGrace)
		}
	}
}
