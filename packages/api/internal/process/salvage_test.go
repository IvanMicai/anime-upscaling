package process

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// signalErr returns a real *exec.ExitError from a process killed by SIGABRT,
// matching what video2x produces when glslang's PoolAlloc assertion fires.
func signalErr(t *testing.T) error {
	t.Helper()
	err := exec.Command("sh", "-c", "kill -ABRT $$").Run()
	if err == nil {
		t.Fatal("expected SIGABRT error from helper")
	}
	return err
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
}

func TestSalvageSignaledRun_SuccessMarkerAndOutput(t *testing.T) {
	dir := t.TempDir()
	logFile := filepath.Join(dir, "gpu0.log")
	tempOut := filepath.Join(dir, "out.mkv")
	writeFile(t, logFile, "[info] processing\n[info] Video processed successfully\n[info] summary\n")
	writeFile(t, tempOut, "fake encoded bytes")

	if !salvageSignaledRun(signalErr(t), logFile, tempOut) {
		t.Fatal("expected salvage when log shows success and output exists")
	}
}

func TestSalvageSignaledRun_NotSignaled(t *testing.T) {
	dir := t.TempDir()
	logFile := filepath.Join(dir, "gpu0.log")
	tempOut := filepath.Join(dir, "out.mkv")
	writeFile(t, logFile, "[info] Video processed successfully\n")
	writeFile(t, tempOut, "fake")

	if salvageSignaledRun(errors.New("plain error"), logFile, tempOut) {
		t.Fatal("expected no salvage when error is not a signal")
	}
}

func TestSalvageSignaledRun_NoSuccessMarker(t *testing.T) {
	dir := t.TempDir()
	logFile := filepath.Join(dir, "gpu0.log")
	tempOut := filepath.Join(dir, "out.mkv")
	writeFile(t, logFile, "[info] processing started\n[error] glslang assertion\n")
	writeFile(t, tempOut, "partial bytes")

	if salvageSignaledRun(signalErr(t), logFile, tempOut) {
		t.Fatal("expected no salvage when log lacks success marker")
	}
}

func TestSalvageSignaledRun_MissingOutput(t *testing.T) {
	dir := t.TempDir()
	logFile := filepath.Join(dir, "gpu0.log")
	tempOut := filepath.Join(dir, "out.mkv")
	writeFile(t, logFile, "[info] Video processed successfully\n")

	if salvageSignaledRun(signalErr(t), logFile, tempOut) {
		t.Fatal("expected no salvage when output file does not exist")
	}
}

func TestSalvageSignaledRun_EmptyOutput(t *testing.T) {
	dir := t.TempDir()
	logFile := filepath.Join(dir, "gpu0.log")
	tempOut := filepath.Join(dir, "out.mkv")
	writeFile(t, logFile, "[info] Video processed successfully\n")
	writeFile(t, tempOut, "")

	if salvageSignaledRun(signalErr(t), logFile, tempOut) {
		t.Fatal("expected no salvage when output file is empty")
	}
}

func TestSalvageSignaledRun_MissingLogFile(t *testing.T) {
	dir := t.TempDir()
	logFile := filepath.Join(dir, "gpu0.log")
	tempOut := filepath.Join(dir, "out.mkv")
	writeFile(t, tempOut, "fake")

	if salvageSignaledRun(signalErr(t), logFile, tempOut) {
		t.Fatal("expected no salvage when log file is missing")
	}
}
