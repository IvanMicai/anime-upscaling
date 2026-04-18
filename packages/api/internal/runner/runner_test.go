package runner

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"syscall"
	"testing"
)

func TestSignalFromError_NilError(t *testing.T) {
	if _, ok := SignalFromError(nil); ok {
		t.Fatal("expected ok=false for nil error")
	}
}

func TestSignalFromError_NonExecError(t *testing.T) {
	if _, ok := SignalFromError(errors.New("boom")); ok {
		t.Fatal("expected ok=false for non-exec error")
	}
}

func TestSignalFromError_NormalExitCode(t *testing.T) {
	cmd := exec.Command("sh", "-c", "exit 3")
	err := cmd.Run()
	if err == nil {
		t.Fatal("expected error from exit 3")
	}
	if _, ok := SignalFromError(err); ok {
		t.Fatalf("expected ok=false for normal exit, got signal")
	}
}

func TestSignalFromError_KilledBySignal(t *testing.T) {
	cmd := exec.Command("sh", "-c", "kill -SEGV $$")
	err := cmd.Run()
	if err == nil {
		t.Fatal("expected error from SIGSEGV")
	}
	sig, ok := SignalFromError(err)
	if !ok {
		t.Fatalf("expected ok=true for signaled exit, err=%v", err)
	}
	if sig != syscall.SIGSEGV {
		t.Fatalf("expected SIGSEGV, got %v", sig)
	}
}

func TestSignalFromError_Wrapped(t *testing.T) {
	cmd := exec.Command("sh", "-c", "kill -SEGV $$")
	inner := cmd.Run()
	wrapped := fmt.Errorf("encode failed: %w", inner)
	sig, ok := SignalFromError(wrapped)
	if !ok || sig != syscall.SIGSEGV {
		t.Fatalf("expected SIGSEGV through wrap, got sig=%v ok=%v", sig, ok)
	}
}

// Sanity: ensure context cancellation still yields an exec error we can inspect.
func TestSignalFromError_CanceledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	cmd := exec.CommandContext(ctx, "sleep", "5")
	err := cmd.Run()
	if err == nil {
		t.Skip("sleep finished before cancel could take effect")
	}
	// Either signaled (SIGKILL) or non-exec "context canceled" — both acceptable,
	// just verify SignalFromError doesn't panic or misreport nil errors.
	_, _ = SignalFromError(err)
}
