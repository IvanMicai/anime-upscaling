package runner

import (
	"context"
	"os/exec"
	"sync"
	"syscall"
	"testing"
	"time"
)

func startSleeper(t *testing.T) *exec.Cmd {
	t.Helper()
	cmd := exec.Command("sleep", "30")
	if err := cmd.Start(); err != nil {
		t.Fatalf("start sleeper: %v", err)
	}
	t.Cleanup(func() {
		_ = cmd.Process.Signal(syscall.SIGKILL)
		_, _ = cmd.Process.Wait()
	})
	return cmd
}

func waitExited(t *testing.T, cmd *exec.Cmd, want string) {
	t.Helper()
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	select {
	case err := <-done:
		if err == nil {
			t.Fatalf("%s: expected non-nil err for terminated process", want)
		}
	case <-time.After(2 * time.Second):
		t.Fatalf("%s: process did not exit after signal", want)
	}
}

func aliveAfter(t *testing.T, cmd *exec.Cmd, d time.Duration) {
	t.Helper()
	done := make(chan struct{})
	var once sync.Once
	go func() {
		_, _ = cmd.Process.Wait()
		once.Do(func() { close(done) })
	}()
	select {
	case <-done:
		t.Fatalf("expected process %d to remain alive, but it exited", cmd.Process.Pid)
	case <-time.After(d):
		// still alive, good
	}
}

func TestStopByJobID_OnlyKillsMatchingJob(t *testing.T) {
	resetTracker(t)

	a1 := startSleeper(t)
	a2 := startSleeper(t)
	b1 := startSleeper(t)

	tracker.registerForJob("a1", a1, "job-A")
	tracker.registerForJob("a2", a2, "job-A")
	tracker.registerForJob("b1", b1, "job-B")

	if n := StopByJobID("job-A"); n != 2 {
		t.Fatalf("StopByJobID(job-A) signaled %d processes, want 2", n)
	}

	waitExited(t, a1, "a1")
	waitExited(t, a2, "a2")
	aliveAfter(t, b1, 200*time.Millisecond)
}

func TestStopByJobID_EmptyIDIsNoop(t *testing.T) {
	resetTracker(t)

	c := startSleeper(t)
	tracker.register("anon", c)

	if n := StopByJobID(""); n != 0 {
		t.Fatalf("StopByJobID(\"\") signaled %d, want 0", n)
	}
	aliveAfter(t, c, 200*time.Millisecond)
}

func TestStopByPrefix_StillSignalsAll(t *testing.T) {
	resetTracker(t)

	x := startSleeper(t)
	y := startSleeper(t)

	tracker.registerForJob(ProcessPrefix+"ffmpeg-encode-x", x, "job-A")
	tracker.register(ProcessPrefix+"ffmpeg-encode-y", y) // anonymous

	if n := StopByPrefix(ProcessPrefix + "ffmpeg-"); n != 2 {
		t.Fatalf("StopByPrefix signaled %d, want 2", n)
	}

	waitExited(t, x, "x")
	waitExited(t, y, "y")
}

func TestJobIDFromContext(t *testing.T) {
	if id := JobIDFromContext(context.Background()); id != "" {
		t.Fatalf("expected empty jobID, got %q", id)
	}
	ctx := WithJobID(context.Background(), "j_42")
	if id := JobIDFromContext(ctx); id != "j_42" {
		t.Fatalf("expected j_42, got %q", id)
	}
	// WithJobID("") is a no-op
	ctx2 := WithJobID(context.Background(), "")
	if id := JobIDFromContext(ctx2); id != "" {
		t.Fatalf("expected empty after WithJobID(\"\"), got %q", id)
	}
}

// resetTracker clears the global tracker between tests so they don't leak.
func resetTracker(t *testing.T) {
	t.Helper()
	tracker.mu.Lock()
	for k := range tracker.procs {
		delete(tracker.procs, k)
	}
	tracker.mu.Unlock()
	t.Cleanup(func() {
		tracker.mu.Lock()
		for k, tc := range tracker.procs {
			if tc.cmd != nil && tc.cmd.Process != nil {
				_ = tc.cmd.Process.Signal(syscall.SIGKILL)
			}
			delete(tracker.procs, k)
		}
		tracker.mu.Unlock()
	})
	// Sanity: prefix-stop on a fresh tracker is a no-op.
	if n := StopByPrefix("never-matches-"); n != 0 {
		t.Fatalf("expected 0 from empty tracker, got %d", n)
	}
}
