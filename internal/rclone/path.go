package rclone

import (
	"path/filepath"
	"strings"
)

func BuildRemoteDir(sourceDir, remoteRoot, sourceFile string) (string, error) {
	rel, err := filepath.Rel(sourceDir, sourceFile)
	if err != nil {
		return "", err
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
