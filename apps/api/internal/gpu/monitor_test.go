package gpu

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"
)

func TestMonitorStartsHealthyAndWaitReturnsImmediately(t *testing.T) {
	m := NewMonitor(true, time.Second, time.Second, 2)
	if !m.IsHealthy() {
		t.Fatalf("expected fresh monitor to be healthy")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	if err := m.WaitHealthy(ctx); err != nil {
		t.Fatalf("WaitHealthy on healthy monitor returned %v", err)
	}
}

func TestMonitorTransitionsAfterThresholdAndRecovers(t *testing.T) {
	m := NewMonitor(true, time.Second, time.Second, 2)

	probeErr := errors.New("nvidia-smi hangs")
	m.applyResult(probeErr)
	if !m.IsHealthy() {
		t.Fatalf("after 1 failure (threshold 2) should still be healthy")
	}
	m.applyResult(probeErr)
	if m.IsHealthy() {
		t.Fatalf("after 2 failures should be unhealthy")
	}

	// WaitHealthy must block now
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	if err := m.WaitHealthy(ctx); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("WaitHealthy should have timed out, got %v", err)
	}

	// Recovery wakes blocked waiters
	done := make(chan error, 1)
	waitCtx, waitCancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer waitCancel()
	go func() { done <- m.WaitHealthy(waitCtx) }()

	time.Sleep(20 * time.Millisecond) // give goroutine time to enter the select
	m.applyResult(nil)

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("WaitHealthy after recovery returned %v", err)
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatalf("WaitHealthy did not return after recovery")
	}

	if !m.IsHealthy() {
		t.Fatalf("should be healthy after recovery")
	}
	if got := m.Status().ConsecutiveFailures; got != 0 {
		t.Fatalf("ConsecutiveFailures should reset to 0, got %d", got)
	}
}

func TestMonitorWatchInvokesProbeAndStopsOnContextDone(t *testing.T) {
	m := NewMonitor(true, 10*time.Millisecond, time.Second, 1)
	var calls int32
	m.SetProbe(func(ctx context.Context) error {
		atomic.AddInt32(&calls, 1)
		return nil
	})

	ctx, cancel := context.WithCancel(context.Background())
	stopped := make(chan struct{})
	go func() {
		m.Watch(ctx)
		close(stopped)
	}()

	time.Sleep(50 * time.Millisecond)
	cancel()
	select {
	case <-stopped:
	case <-time.After(500 * time.Millisecond):
		t.Fatalf("Watch did not stop after ctx cancel")
	}
	if atomic.LoadInt32(&calls) < 2 {
		t.Fatalf("expected probe to fire at least 2x, got %d", calls)
	}
}

func TestMonitorDisabledIsNoOp(t *testing.T) {
	m := NewMonitor(false, time.Millisecond, time.Second, 1)
	var calls int32
	m.SetProbe(func(ctx context.Context) error {
		atomic.AddInt32(&calls, 1)
		return errors.New("should not be called")
	})

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	m.Watch(ctx)

	if atomic.LoadInt32(&calls) != 0 {
		t.Fatalf("disabled monitor should not probe, got %d calls", calls)
	}
	if !m.IsHealthy() {
		t.Fatalf("disabled monitor should report healthy")
	}
}
