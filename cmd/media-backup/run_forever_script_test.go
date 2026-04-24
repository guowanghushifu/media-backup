package main

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunForeverScriptSelectsArm64BinaryAndForwardsArguments(t *testing.T) {
	t.Parallel()

	tempDir := t.TempDir()
	logPath := filepath.Join(tempDir, "binary.log")
	writeExecutable(t, filepath.Join(tempDir, "media-backup-linux-arm64"), "#!/bin/sh\nprintf '%s\n' \"$0 $*\" >> \""+logPath+"\"\nexit 0\n")
	copyRunForeverScript(t, tempDir)

	cmd := exec.Command("bash", filepath.Join(tempDir, "run-forever.sh"), "-config", filepath.Join(tempDir, "config.yaml"))
	cmd.Env = append(os.Environ(), "MEDIA_BACKUP_UNAME_M=aarch64")

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		t.Fatalf("run-forever.sh error = %v, stdout=%q stderr=%q", err, stdout.String(), stderr.String())
	}

	logContent, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("ReadFile(logPath) error = %v", err)
	}
	if !strings.Contains(string(logContent), "media-backup-linux-arm64 -config "+filepath.Join(tempDir, "config.yaml")) {
		t.Fatalf("binary log = %q, want arm64 binary with forwarded config args", string(logContent))
	}
	if !strings.Contains(stdout.String(), "normal exit, not restarting") {
		t.Fatalf("stdout = %q, want normal exit message", stdout.String())
	}
}

func TestRunForeverScriptRestartsAfterUnexpectedExit(t *testing.T) {
	t.Parallel()

	tempDir := t.TempDir()
	statePath := filepath.Join(tempDir, "state")
	writeExecutable(t, filepath.Join(tempDir, "media-backup-linux-amd64"), "#!/bin/sh\nif [ ! -f \""+statePath+"\" ]; then\n  touch \""+statePath+"\"\n  exit 2\nfi\nexit 0\n")
	copyRunForeverScript(t, tempDir)

	cmd := exec.Command("bash", filepath.Join(tempDir, "run-forever.sh"))
	cmd.Env = append(os.Environ(), "MEDIA_BACKUP_UNAME_M=x86_64", "MEDIA_BACKUP_RESTART_DELAY=0")

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		t.Fatalf("run-forever.sh error = %v, stdout=%q stderr=%q", err, stdout.String(), stderr.String())
	}

	output := stdout.String()
	if strings.Count(output, "starting media-backup") != 2 {
		t.Fatalf("stdout = %q, want two start attempts", output)
	}
	if !strings.Contains(output, "exited with code 2") || !strings.Contains(output, "normal exit, not restarting") {
		t.Fatalf("stdout = %q, want restart after exit 2 then normal stop", output)
	}
}

func TestRunForeverScriptFailsForUnsupportedArchitecture(t *testing.T) {
	t.Parallel()

	tempDir := t.TempDir()
	copyRunForeverScript(t, tempDir)

	cmd := exec.Command("bash", filepath.Join(tempDir, "run-forever.sh"))
	cmd.Env = append(os.Environ(), "MEDIA_BACKUP_UNAME_M=riscv64")

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err == nil {
		t.Fatalf("run-forever.sh error = nil, want failure, stdout=%q stderr=%q", stdout.String(), stderr.String())
	}
	if !strings.Contains(stderr.String(), "unsupported architecture") {
		t.Fatalf("stderr = %q, want unsupported architecture message", stderr.String())
	}
}

func copyRunForeverScript(t *testing.T, dir string) {
	t.Helper()

	contents, err := os.ReadFile(filepath.Join("..", "..", "dist", "run-forever.sh"))
	if err != nil {
		t.Fatalf("ReadFile(run-forever.sh) error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "run-forever.sh"), contents, 0o755); err != nil {
		t.Fatalf("WriteFile(run-forever.sh) error = %v", err)
	}
}

func writeExecutable(t *testing.T, path, contents string) {
	t.Helper()

	if err := os.WriteFile(path, []byte(contents), 0o755); err != nil {
		t.Fatalf("WriteFile(%q) error = %v", path, err)
	}
}
