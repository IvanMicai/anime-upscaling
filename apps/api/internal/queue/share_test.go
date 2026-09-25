package queue

import (
	"context"
	"testing"
	"time"
)

func acquireAsync(g *GPUShares, ctx context.Context, gpuID, weight, capacity int) <-chan func() {
	ch := make(chan func(), 1)
	go func() {
		release, err := g.Acquire(ctx, gpuID, weight, capacity)
		if err != nil {
			close(ch)
			return
		}
		ch <- release
	}()
	return ch
}

func mustBlock(t *testing.T, ch <-chan func(), what string) {
	t.Helper()
	select {
	case <-ch:
		t.Fatalf("%s should still be waiting", what)
	case <-time.After(50 * time.Millisecond):
	}
}

func mustGet(t *testing.T, ch <-chan func(), what string) func() {
	t.Helper()
	select {
	case release, ok := <-ch:
		if !ok {
			t.Fatalf("%s failed", what)
		}
		return release
	case <-time.After(time.Second):
		t.Fatalf("%s did not get its shares", what)
	}
	return nil
}

// TestGPUSharesWholeGPUWaitsForStreams: with two streams per GPU, a whole-GPU
// request waits for both single shares, and a single share arriving after it
// waits behind it instead of slipping in.
func TestGPUSharesWholeGPUWaitsForStreams(t *testing.T) {
	g := NewGPUShares()
	ctx := context.Background()

	r1, _ := g.Acquire(ctx, 0, 1, 2)
	r2, _ := g.Acquire(ctx, 0, 1, 2)

	whole := acquireAsync(g, ctx, 0, 2, 2)
	mustBlock(t, whole, "whole-GPU request")

	later := acquireAsync(g, ctx, 0, 1, 2)
	mustBlock(t, later, "single share behind the whole-GPU request")

	r1()
	mustBlock(t, whole, "whole-GPU request with one stream still busy")
	r2()
	rw := mustGet(t, whole, "whole-GPU request")
	mustBlock(t, later, "single share while the GPU is taken whole")

	rw()
	mustGet(t, later, "single share")()
}

// TestGPUSharesPerGPU: shares of one GPU never block another GPU.
func TestGPUSharesPerGPU(t *testing.T) {
	g := NewGPUShares()
	ctx := context.Background()
	r0, _ := g.Acquire(ctx, 0, 1, 1)
	defer r0()
	mustGet(t, acquireAsync(g, ctx, 1, 1, 1), "GPU 1 share")()
}

// TestGPUSharesCancelLetsNextIn: a cancelled head waiter is removed and the
// waiters behind it are admitted if they fit.
func TestGPUSharesCancelLetsNextIn(t *testing.T) {
	g := NewGPUShares()
	r1, _ := g.Acquire(context.Background(), 0, 1, 2)

	cctx, cancel := context.WithCancel(context.Background())
	whole := acquireAsync(g, cctx, 0, 2, 2)
	mustBlock(t, whole, "whole-GPU request")
	single := acquireAsync(g, context.Background(), 0, 1, 2)
	mustBlock(t, single, "single share behind it")

	cancel()
	if _, ok := <-whole; ok {
		t.Fatal("cancelled request should fail")
	}
	mustGet(t, single, "single share after the cancel")()
	r1()
}

// TestGPUSharesWeightClamped: asking for more than the capacity takes the
// whole GPU instead of waiting forever.
func TestGPUSharesWeightClamped(t *testing.T) {
	g := NewGPUShares()
	release, err := g.Acquire(context.Background(), 0, 5, 1)
	if err != nil {
		t.Fatal(err)
	}
	release()
	release() // idempotent
	mustGet(t, acquireAsync(g, context.Background(), 0, 1, 1), "share after release")()
}
