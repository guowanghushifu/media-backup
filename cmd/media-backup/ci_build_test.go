package main

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestCIBuildFailsWhenVersionUnset(t *testing.T) {
	t.Parallel()

	repoRoot := ciBuildRepoRoot(t)
	tempDir := t.TempDir()
	fakeGo := writeFakeGo(t, tempDir)

	cmd := exec.Command("bash", filepath.Join(repoRoot, "ci_build.sh"))
	cmd.Dir = repoRoot
	cmd.Env = append(os.Environ(),
		"GO_BIN="+fakeGo,
		"DIST_DIR="+filepath.Join(tempDir, "dist"),
		"BUILD_TMP_DIR="+filepath.Join(tempDir, "build-tmp"),
	)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	if err == nil {
		t.Fatalf("ci_build.sh error = nil, want failure, stdout=%q stderr=%q", stdout.String(), stderr.String())
	}
	if !strings.Contains(stderr.String(), "VERSION is required") {
		t.Fatalf("stderr = %q, want VERSION validation error", stderr.String())
	}
}

func TestCIBuildCreatesVersionedArchivesWithExpectedContents(t *testing.T) {
	t.Parallel()

	repoRoot := ciBuildRepoRoot(t)
	tempDir := t.TempDir()
	distDir := filepath.Join(tempDir, "dist")
	buildTmpDir := filepath.Join(tempDir, "build-tmp")
	fakeGo := writeFakeGo(t, tempDir)

	cmd := exec.Command("bash", filepath.Join(repoRoot, "ci_build.sh"))
	cmd.Dir = repoRoot
	cmd.Env = append(os.Environ(),
		"VERSION=v0.3.0",
		"GO_BIN="+fakeGo,
		"DIST_DIR="+distDir,
		"BUILD_TMP_DIR="+buildTmpDir,
	)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		t.Fatalf("ci_build.sh error = %v, stdout=%q stderr=%q", err, stdout.String(), stderr.String())
	}

	amd64Archive := filepath.Join(distDir, "media-backup_0.3.0_linux_amd64.tar.gz")
	arm64Archive := filepath.Join(distDir, "media-backup_0.3.0_linux_arm64.tar.gz")
	for _, archivePath := range []string{amd64Archive, arm64Archive} {
		if _, err := os.Stat(archivePath); err != nil {
			t.Fatalf("Stat(%q) error = %v", archivePath, err)
		}
	}

	amd64Entries := archiveEntries(t, amd64Archive)
	if _, ok := amd64Entries["media-backup-linux-amd64"]; !ok {
		t.Fatalf("amd64 archive entries = %#v, want amd64 binary", amd64Entries)
	}
	if _, ok := amd64Entries["install-systemd-service.sh"]; !ok {
		t.Fatalf("amd64 archive entries = %#v, want install script", amd64Entries)
	}
	if _, ok := amd64Entries["run-forever.sh"]; !ok {
		t.Fatalf("amd64 archive entries = %#v, want run-forever script", amd64Entries)
	}
	if _, ok := amd64Entries["media-backup-linux-arm64"]; ok {
		t.Fatalf("amd64 archive entries = %#v, want no arm64 binary", amd64Entries)
	}
	if mode := amd64Entries["media-backup-linux-amd64"]; mode&0o111 == 0 {
		t.Fatalf("amd64 binary mode = %#o, want executable", mode)
	}
	if mode := amd64Entries["install-systemd-service.sh"]; mode&0o111 == 0 {
		t.Fatalf("install script mode = %#o, want executable", mode)
	}
	if mode := amd64Entries["run-forever.sh"]; mode&0o111 == 0 {
		t.Fatalf("run-forever script mode = %#o, want executable", mode)
	}

	arm64Entries := archiveEntries(t, arm64Archive)
	if _, ok := arm64Entries["media-backup-linux-arm64"]; !ok {
		t.Fatalf("arm64 archive entries = %#v, want arm64 binary", arm64Entries)
	}
	if _, ok := arm64Entries["install-systemd-service.sh"]; !ok {
		t.Fatalf("arm64 archive entries = %#v, want install script", arm64Entries)
	}
	if _, ok := arm64Entries["run-forever.sh"]; !ok {
		t.Fatalf("arm64 archive entries = %#v, want run-forever script", arm64Entries)
	}
	if _, ok := arm64Entries["media-backup-linux-amd64"]; ok {
		t.Fatalf("arm64 archive entries = %#v, want no amd64 binary", arm64Entries)
	}
	if mode := arm64Entries["media-backup-linux-arm64"]; mode&0o111 == 0 {
		t.Fatalf("arm64 binary mode = %#o, want executable", mode)
	}
	if mode := arm64Entries["install-systemd-service.sh"]; mode&0o111 == 0 {
		t.Fatalf("install script mode = %#o, want executable", mode)
	}
	if mode := arm64Entries["run-forever.sh"]; mode&0o111 == 0 {
		t.Fatalf("run-forever script mode = %#o, want executable", mode)
	}
}

func TestCIBuildWithDefaultDistKeepsRunForeverInArchives(t *testing.T) {
	t.Parallel()

	repoRoot := ciBuildRepoRoot(t)
	tempRepo := t.TempDir()
	if err := os.MkdirAll(filepath.Join(tempRepo, "dist"), 0o755); err != nil {
		t.Fatalf("MkdirAll(dist) error = %v", err)
	}
	contents, err := os.ReadFile(filepath.Join(repoRoot, "ci_build.sh"))
	if err != nil {
		t.Fatalf("ReadFile(ci_build.sh) error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(tempRepo, "ci_build.sh"), contents, 0o755); err != nil {
		t.Fatalf("WriteFile(ci_build.sh) error = %v", err)
	}

	for _, name := range []string{"install-systemd-service.sh", "run-forever.sh"} {
		contents, err := os.ReadFile(filepath.Join(repoRoot, "dist", name))
		if err != nil {
			t.Fatalf("ReadFile(%q) error = %v", name, err)
		}
		if err := os.WriteFile(filepath.Join(tempRepo, "dist", name), contents, 0o755); err != nil {
			t.Fatalf("WriteFile(%q) error = %v", name, err)
		}
	}

	fakeGo := writeFakeGo(t, tempRepo)
	cmd := exec.Command("bash", filepath.Join(tempRepo, "ci_build.sh"))
	cmd.Dir = tempRepo
	cmd.Env = append(os.Environ(),
		"VERSION=v0.3.0",
		"GO_BIN="+fakeGo,
	)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		t.Fatalf("ci_build.sh error = %v, stdout=%q stderr=%q", err, stdout.String(), stderr.String())
	}

	entries := archiveEntries(t, filepath.Join(tempRepo, "dist", "media-backup_0.3.0_linux_amd64.tar.gz"))
	if _, ok := entries["run-forever.sh"]; !ok {
		t.Fatalf("archive entries = %#v, want run-forever script", entries)
	}
}

func ciBuildRepoRoot(t *testing.T) string {
	t.Helper()

	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatalf("filepath.Abs(repoRoot) error = %v", err)
	}
	return root
}

func writeFakeGo(t *testing.T, dir string) string {
	t.Helper()

	path := filepath.Join(dir, "fake-go")
	script := `#!/bin/sh
set -eu
out=""
prev=""
for arg in "$@"; do
	if [ "$prev" = "-o" ]; then
		out="$arg"
		break
	fi
	prev="$arg"
done
if [ -z "$out" ]; then
	echo "missing -o" >&2
	exit 1
fi
mkdir -p "$(dirname "$out")"
printf '#!/bin/sh\nexit 0\n' > "$out"
chmod +x "$out"
`
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatalf("WriteFile(fake-go) error = %v", err)
	}
	return path
}

func archiveEntries(t *testing.T, archivePath string) map[string]os.FileMode {
	t.Helper()

	file, err := os.Open(archivePath)
	if err != nil {
		t.Fatalf("os.Open(%q) error = %v", archivePath, err)
	}
	defer file.Close()

	gzipReader, err := gzip.NewReader(file)
	if err != nil {
		t.Fatalf("gzip.NewReader(%q) error = %v", archivePath, err)
	}
	defer gzipReader.Close()

	tarReader := tar.NewReader(gzipReader)
	entries := make(map[string]os.FileMode)
	for {
		header, err := tarReader.Next()
		if err != nil {
			if err == io.EOF {
				break
			}
			t.Fatalf("tarReader.Next(%q) error = %v", archivePath, err)
		}
		entries[header.Name] = header.FileInfo().Mode().Perm()
	}
	return entries
}
