package rclone

import (
	"errors"
	"path/filepath"
	"strings"
)

var errSourceFileOutsideSourceDir = errors.New("source file is outside source dir")

func BuildRemoteDir(sourceDir, remoteRoot, sourceFile string) (string, error) {
	cleanSourceDir := filepath.Clean(sourceDir)
	cleanSourceFile := filepath.Clean(sourceFile)

	rel, err := filepath.Rel(cleanSourceDir, cleanSourceFile)
	if err != nil {
		return "", err
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", errSourceFileOutsideSourceDir
	}

	relDir := filepath.Dir(rel)
	if relDir == "." {
		relDir = ""
	}

	remoteDir := joinRemotePath(remoteRoot, filepath.ToSlash(relDir))
	if !strings.HasSuffix(remoteDir, "/") {
		remoteDir += "/"
	}
	return remoteDir, nil
}

func joinRemotePath(remoteRoot, suffix string) string {
	cleanRoot := strings.TrimSuffix(remoteRoot, "/")
	cleanSuffix := strings.TrimPrefix(suffix, "/")

	if cleanSuffix == "" {
		return cleanRoot
	}
	if cleanRoot == "" {
		return cleanSuffix
	}
	return cleanRoot + "/" + cleanSuffix
}
