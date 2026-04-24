package main

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestInstallScriptDefaultInstallsServiceFromSiblingBinary(t *testing.T) {
	t.Parallel()

	tempDir := t.TempDir()
	realTempDir, err := filepath.EvalSymlinks(tempDir)
	if err != nil {
		t.Fatalf("EvalSymlinks(tempDir) error = %v", err)
	}
	unitDir := filepath.Join(tempDir, "systemd")
	if err := os.MkdirAll(unitDir, 0o755); err != nil {
		t.Fatalf("MkdirAll(unitDir) error = %v", err)
	}

	binaryPath := filepath.Join(tempDir, "media-backup-linux-amd64")
	if err := os.WriteFile(binaryPath, []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatalf("WriteFile(binaryPath) error = %v", err)
	}

	arm64Path := filepath.Join(tempDir, "media-backup-linux-arm64")
	if err := os.WriteFile(arm64Path, []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatalf("WriteFile(arm64Path) error = %v", err)
	}

	logPath := filepath.Join(tempDir, "systemctl.log")
	fakeSystemctl := filepath.Join(tempDir, "systemctl")
	if err := os.WriteFile(fakeSystemctl, []byte("#!/bin/sh\nprintf '%s\\n' \"$*\" >> \""+logPath+"\"\n"), 0o755); err != nil {
		t.Fatalf("WriteFile(fakeSystemctl) error = %v", err)
	}

	scriptPath := filepath.Join(tempDir, "install-systemd-service.sh")
	contents, err := os.ReadFile(filepath.Join("..", "..", "dist", "install-systemd-service.sh"))
	if err != nil {
		t.Fatalf("ReadFile(script) error = %v", err)
	}
	if err := os.WriteFile(scriptPath, contents, 0o755); err != nil {
		t.Fatalf("WriteFile(scriptPath) error = %v", err)
	}

	cmd := exec.Command("bash", scriptPath)
	cmd.Env = append(os.Environ(),
		"MEDIA_BACKUP_SKIP_ROOT_CHECK=1",
		"MEDIA_BACKUP_SYSTEMD_UNIT_DIR="+unitDir,
		"MEDIA_BACKUP_SYSTEMCTL_BIN="+fakeSystemctl,
		"MEDIA_BACKUP_UNAME_M=x86_64",
	)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		t.Fatalf("install script error = %v, stdout=%q stderr=%q", err, stdout.String(), stderr.String())
	}

	unitPath := filepath.Join(unitDir, "media-backup.service")
	unitContent, err := os.ReadFile(unitPath)
	if err != nil {
		t.Fatalf("ReadFile(unitPath) error = %v", err)
	}

	for _, want := range []string{
		"WorkingDirectory=" + realTempDir,
		"ExecStart=" + filepath.Join(realTempDir, "media-backup-linux-amd64"),
		"Restart=on-failure",
		"RestartSec=30",
		"StandardOutput=null",
		"StandardError=journal",
	} {
		if !strings.Contains(string(unitContent), want) {
			t.Fatalf("unit file missing %q in %q", want, string(unitContent))
		}
	}

	logContent, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("ReadFile(logPath) error = %v", err)
	}
	for _, want := range []string{
		"daemon-reload",
		"enable --now media-backup.service",
	} {
		if !strings.Contains(string(logContent), want) {
			t.Fatalf("systemctl log missing %q in %q", want, string(logContent))
		}
	}
	if !strings.Contains(stdout.String(), "installed media-backup.service") {
		t.Fatalf("stdout = %q, want install message", stdout.String())
	}
}

func TestInstallScriptInstallsArm64BinaryWhenHostIsArm64(t *testing.T) {
	t.Parallel()

	tempDir := t.TempDir()
	realTempDir, err := filepath.EvalSymlinks(tempDir)
	if err != nil {
		t.Fatalf("EvalSymlinks(tempDir) error = %v", err)
	}
	unitDir := filepath.Join(tempDir, "systemd")
	if err := os.MkdirAll(unitDir, 0o755); err != nil {
		t.Fatalf("MkdirAll(unitDir) error = %v", err)
	}

	if err := os.WriteFile(filepath.Join(tempDir, "media-backup-linux-amd64"), []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatalf("WriteFile(amd64) error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(tempDir, "media-backup-linux-arm64"), []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatalf("WriteFile(arm64) error = %v", err)
	}

	logPath := filepath.Join(tempDir, "systemctl.log")
	fakeSystemctl := filepath.Join(tempDir, "systemctl")
	if err := os.WriteFile(fakeSystemctl, []byte("#!/bin/sh\nprintf '%s\\n' \"$*\" >> \""+logPath+"\"\n"), 0o755); err != nil {
		t.Fatalf("WriteFile(fakeSystemctl) error = %v", err)
	}

	scriptPath := filepath.Join(tempDir, "install-systemd-service.sh")
	contents, err := os.ReadFile(filepath.Join("..", "..", "dist", "install-systemd-service.sh"))
	if err != nil {
		t.Fatalf("ReadFile(script) error = %v", err)
	}
	if err := os.WriteFile(scriptPath, contents, 0o755); err != nil {
		t.Fatalf("WriteFile(scriptPath) error = %v", err)
	}

	cmd := exec.Command("bash", scriptPath, "-i")
	cmd.Env = append(os.Environ(),
		"MEDIA_BACKUP_SKIP_ROOT_CHECK=1",
		"MEDIA_BACKUP_SYSTEMD_UNIT_DIR="+unitDir,
		"MEDIA_BACKUP_SYSTEMCTL_BIN="+fakeSystemctl,
		"MEDIA_BACKUP_UNAME_M=aarch64",
	)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		t.Fatalf("install script error = %v, stdout=%q stderr=%q", err, stdout.String(), stderr.String())
	}

	unitPath := filepath.Join(unitDir, "media-backup.service")
	unitContent, err := os.ReadFile(unitPath)
	if err != nil {
		t.Fatalf("ReadFile(unitPath) error = %v", err)
	}

	wantExecStart := "ExecStart=" + filepath.Join(realTempDir, "media-backup-linux-arm64")
	if !strings.Contains(string(unitContent), wantExecStart) {
		t.Fatalf("unit file missing %q in %q", wantExecStart, string(unitContent))
	}
}

func TestInstallScriptFailsWhenRequiredArchBinaryMissing(t *testing.T) {
	t.Parallel()

	tempDir := t.TempDir()
	unitDir := filepath.Join(tempDir, "systemd")
	if err := os.MkdirAll(unitDir, 0o755); err != nil {
		t.Fatalf("MkdirAll(unitDir) error = %v", err)
	}

	if err := os.WriteFile(filepath.Join(tempDir, "media-backup-linux-amd64"), []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatalf("WriteFile(amd64) error = %v", err)
	}

	scriptPath := filepath.Join(tempDir, "install-systemd-service.sh")
	contents, err := os.ReadFile(filepath.Join("..", "..", "dist", "install-systemd-service.sh"))
	if err != nil {
		t.Fatalf("ReadFile(script) error = %v", err)
	}
	if err := os.WriteFile(scriptPath, contents, 0o755); err != nil {
		t.Fatalf("WriteFile(scriptPath) error = %v", err)
	}

	cmd := exec.Command("bash", scriptPath)
	cmd.Env = append(os.Environ(),
		"MEDIA_BACKUP_SKIP_ROOT_CHECK=1",
		"MEDIA_BACKUP_SYSTEMD_UNIT_DIR="+unitDir,
		"MEDIA_BACKUP_UNAME_M=arm64",
	)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err = cmd.Run()
	if err == nil {
		t.Fatalf("install script error = nil, want failure, stdout=%q stderr=%q", stdout.String(), stderr.String())
	}
	if !strings.Contains(stderr.String(), "media-backup-linux-arm64") {
		t.Fatalf("stderr = %q, want missing arm64 binary message", stderr.String())
	}
}

func TestInstallScriptReportsAlreadyInstalledWithoutReinstalling(t *testing.T) {
	t.Parallel()

	tempDir := t.TempDir()
	unitDir := filepath.Join(tempDir, "systemd")
	if err := os.MkdirAll(unitDir, 0o755); err != nil {
		t.Fatalf("MkdirAll(unitDir) error = %v", err)
	}

	binaryPath := filepath.Join(tempDir, "media-backup-linux-amd64")
	if err := os.WriteFile(binaryPath, []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatalf("WriteFile(binaryPath) error = %v", err)
	}

	logPath := filepath.Join(tempDir, "systemctl.log")
	fakeSystemctl := filepath.Join(tempDir, "systemctl")
	if err := os.WriteFile(fakeSystemctl, []byte("#!/bin/sh\nprintf '%s\\n' \"$*\" >> \""+logPath+"\"\n"), 0o755); err != nil {
		t.Fatalf("WriteFile(fakeSystemctl) error = %v", err)
	}

	scriptPath := filepath.Join(tempDir, "install-systemd-service.sh")
	contents, err := os.ReadFile(filepath.Join("..", "..", "dist", "install-systemd-service.sh"))
	if err != nil {
		t.Fatalf("ReadFile(script) error = %v", err)
	}
	if err := os.WriteFile(scriptPath, contents, 0o755); err != nil {
		t.Fatalf("WriteFile(scriptPath) error = %v", err)
	}

	unitPath := filepath.Join(unitDir, "media-backup.service")
	if err := os.WriteFile(unitPath, []byte("[Unit]\nDescription=existing\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(unitPath) error = %v", err)
	}

	cmd := exec.Command("bash", scriptPath, "-i")
	cmd.Env = append(os.Environ(),
		"MEDIA_BACKUP_SKIP_ROOT_CHECK=1",
		"MEDIA_BACKUP_SYSTEMD_UNIT_DIR="+unitDir,
		"MEDIA_BACKUP_SYSTEMCTL_BIN="+fakeSystemctl,
	)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		t.Fatalf("install script error = %v, stdout=%q stderr=%q", err, stdout.String(), stderr.String())
	}

	if !strings.Contains(stdout.String(), "already installed") {
		t.Fatalf("stdout = %q, want already installed message", stdout.String())
	}

	logContent, err := os.ReadFile(logPath)
	if err != nil && !os.IsNotExist(err) {
		t.Fatalf("ReadFile(logPath) error = %v", err)
	}
	if len(logContent) != 0 {
		t.Fatalf("systemctl log = %q, want no systemctl calls", string(logContent))
	}
}

func TestInstallScriptUninstallsService(t *testing.T) {
	t.Parallel()

	tempDir := t.TempDir()
	unitDir := filepath.Join(tempDir, "systemd")
	if err := os.MkdirAll(unitDir, 0o755); err != nil {
		t.Fatalf("MkdirAll(unitDir) error = %v", err)
	}

	logPath := filepath.Join(tempDir, "systemctl.log")
	fakeSystemctl := filepath.Join(tempDir, "systemctl")
	if err := os.WriteFile(fakeSystemctl, []byte("#!/bin/sh\nprintf '%s\\n' \"$*\" >> \""+logPath+"\"\n"), 0o755); err != nil {
		t.Fatalf("WriteFile(fakeSystemctl) error = %v", err)
	}

	scriptPath := filepath.Join(tempDir, "install-systemd-service.sh")
	contents, err := os.ReadFile(filepath.Join("..", "..", "dist", "install-systemd-service.sh"))
	if err != nil {
		t.Fatalf("ReadFile(script) error = %v", err)
	}
	if err := os.WriteFile(scriptPath, contents, 0o755); err != nil {
		t.Fatalf("WriteFile(scriptPath) error = %v", err)
	}

	unitPath := filepath.Join(unitDir, "media-backup.service")
	if err := os.WriteFile(unitPath, []byte("[Unit]\nDescription=existing\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(unitPath) error = %v", err)
	}

	cmd := exec.Command("bash", scriptPath, "-u")
	cmd.Env = append(os.Environ(),
		"MEDIA_BACKUP_SKIP_ROOT_CHECK=1",
		"MEDIA_BACKUP_SYSTEMD_UNIT_DIR="+unitDir,
		"MEDIA_BACKUP_SYSTEMCTL_BIN="+fakeSystemctl,
	)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		t.Fatalf("uninstall script error = %v, stdout=%q stderr=%q", err, stdout.String(), stderr.String())
	}

	if _, err := os.Stat(unitPath); !os.IsNotExist(err) {
		t.Fatalf("Stat(unitPath) error = %v, want not exist", err)
	}

	logContent, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("ReadFile(logPath) error = %v", err)
	}
	for _, want := range []string{
		"disable --now media-backup.service",
		"daemon-reload",
	} {
		if !strings.Contains(string(logContent), want) {
			t.Fatalf("systemctl log missing %q in %q", want, string(logContent))
		}
	}
	if !strings.Contains(stdout.String(), "uninstalled media-backup.service") {
		t.Fatalf("stdout = %q, want uninstall message", stdout.String())
	}
}

func TestInstallScriptRestartsServiceWithThirtySecondDelay(t *testing.T) {
	t.Parallel()

	tempDir := t.TempDir()
	unitDir := filepath.Join(tempDir, "systemd")
	if err := os.MkdirAll(unitDir, 0o755); err != nil {
		t.Fatalf("MkdirAll(unitDir) error = %v", err)
	}

	systemctlLogPath := filepath.Join(tempDir, "systemctl.log")
	fakeSystemctl := filepath.Join(tempDir, "systemctl")
	if err := os.WriteFile(fakeSystemctl, []byte("#!/bin/sh\nprintf '%s\\n' \"$*\" >> \""+systemctlLogPath+"\"\n"), 0o755); err != nil {
		t.Fatalf("WriteFile(fakeSystemctl) error = %v", err)
	}

	sleepLogPath := filepath.Join(tempDir, "sleep.log")
	fakeSleep := filepath.Join(tempDir, "sleep")
	if err := os.WriteFile(fakeSleep, []byte("#!/bin/sh\nprintf '%s\\n' \"$*\" >> \""+sleepLogPath+"\"\n"), 0o755); err != nil {
		t.Fatalf("WriteFile(fakeSleep) error = %v", err)
	}

	scriptPath := filepath.Join(tempDir, "install-systemd-service.sh")
	contents, err := os.ReadFile(filepath.Join("..", "..", "dist", "install-systemd-service.sh"))
	if err != nil {
		t.Fatalf("ReadFile(script) error = %v", err)
	}
	if err := os.WriteFile(scriptPath, contents, 0o755); err != nil {
		t.Fatalf("WriteFile(scriptPath) error = %v", err)
	}

	unitPath := filepath.Join(unitDir, "media-backup.service")
	if err := os.WriteFile(unitPath, []byte("[Unit]\nDescription=existing\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(unitPath) error = %v", err)
	}

	cmd := exec.Command("bash", scriptPath, "-r")
	cmd.Env = append(os.Environ(),
		"MEDIA_BACKUP_SKIP_ROOT_CHECK=1",
		"MEDIA_BACKUP_SYSTEMD_UNIT_DIR="+unitDir,
		"MEDIA_BACKUP_SYSTEMCTL_BIN="+fakeSystemctl,
		"MEDIA_BACKUP_SLEEP_BIN="+fakeSleep,
	)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		t.Fatalf("restart script error = %v, stdout=%q stderr=%q", err, stdout.String(), stderr.String())
	}

	systemctlLog, err := os.ReadFile(systemctlLogPath)
	if err != nil {
		t.Fatalf("ReadFile(systemctlLogPath) error = %v", err)
	}
	for _, want := range []string{
		"stop media-backup.service",
		"start media-backup.service",
	} {
		if !strings.Contains(string(systemctlLog), want) {
			t.Fatalf("systemctl log missing %q in %q", want, string(systemctlLog))
		}
	}

	sleepLog, err := os.ReadFile(filepath.Join(tempDir, "sleep.log"))
	if err != nil {
		t.Fatalf("ReadFile(sleep.log) error = %v", err)
	}
	if got := strings.TrimSpace(string(sleepLog)); got != "30" {
		t.Fatalf("sleep log = %q, want %q", got, "30")
	}

	if !strings.Contains(stdout.String(), "restarted media-backup.service after 30s delay") {
		t.Fatalf("stdout = %q, want restart message", stdout.String())
	}
}

func TestInstallScriptRestartFailsWhenServiceIsNotInstalled(t *testing.T) {
	t.Parallel()

	tempDir := t.TempDir()
	unitDir := filepath.Join(tempDir, "systemd")
	if err := os.MkdirAll(unitDir, 0o755); err != nil {
		t.Fatalf("MkdirAll(unitDir) error = %v", err)
	}

	scriptPath := filepath.Join(tempDir, "install-systemd-service.sh")
	contents, err := os.ReadFile(filepath.Join("..", "..", "dist", "install-systemd-service.sh"))
	if err != nil {
		t.Fatalf("ReadFile(script) error = %v", err)
	}
	if err := os.WriteFile(scriptPath, contents, 0o755); err != nil {
		t.Fatalf("WriteFile(scriptPath) error = %v", err)
	}

	cmd := exec.Command("bash", scriptPath, "-r")
	cmd.Env = append(os.Environ(),
		"MEDIA_BACKUP_SKIP_ROOT_CHECK=1",
		"MEDIA_BACKUP_SYSTEMD_UNIT_DIR="+unitDir,
	)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err = cmd.Run()
	if err == nil {
		t.Fatalf("restart script error = nil, want failure, stdout=%q stderr=%q", stdout.String(), stderr.String())
	}
	if !strings.Contains(stderr.String(), "media-backup.service is not installed") {
		t.Fatalf("stderr = %q, want not-installed error", stderr.String())
	}
}
