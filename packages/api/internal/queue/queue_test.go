package queue

import (
	"context"
	"sync"
	"testing"
	"time"
)

// waitForWaiters blocks until the queue has at least n registered waiters or the
// deadline elapses, so tests can deterministically ensure all contenders are
// parked before the held slot is released.
func waitForWaiters(t *testing.T, q *Queue, n int) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for {
		q.mu.Lock()
		got := len(q.waiters)
		q.mu.Unlock()
		if got >= n {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %d waiters, have %d", n, got)
		}
		time.Sleep(time.Millisecond)
	}
}

// TestQueue_ReleaseWakesHighestPriority verifies that when several callers are
// blocked on a single busy slot, releasing it serves the highest-priority waiter.
func TestQueue_ReleaseWakesHighestPriority(t *testing.T) {
	q := New(1)

	// Hold the only slot.
	held, err := q.Acquire(context.Background(), 0)
	if err != nil {
		t.Fatalf("initial Acquire failed: %v", err)
	}

	// Park three waiters with distinct priorities.
	type result struct {
		priority int
		order    int
	}
	results := make(chan result, 3)
	var started sync.WaitGroup
	for _, p := range []int{5, 20, 10} {
		started.Add(1)
		priority := p
		go func() {
			started.Done()
			slot, err := q.Acquire(context.Background(), priority)
			if err != nil {
				t.Errorf("waiter %d Acquire failed: %v", priority, err)
				return
			}
			results <- result{priority: priority}
			// Hand the slot to the next waiter.
			q.Release(slot)
		}()
	}
	started.Wait()
	waitForWaiters(t, q, 3)

	// Release the held slot; waiters should now drain in priority order.
	q.Release(held)

	order := make([]int, 0, 3)
	for i := 0; i < 3; i++ {
		order = append(order, (<-results).priority)
	}
	want := []int{20, 10, 5}
	for i := range want {
		if order[i] != want[i] {
			t.Fatalf("wake order = %v, want %v", order, want)
		}
	}
}

// TestQueue_PipelineDispatchOrder reproduces how StartPipelineJob dispatches a
// single-step optimize-CPU pipeline: all per-file goroutines are launched at
// once (here in shuffled order) and contend for a constrained queue, each using
// priority -index (the value pipelinePriority(0, index) produces). Once every
// goroutine is parked as a waiter, the slots must drain in natural index order
// (1, 2, 3, ...), proving the ordering guarantee for the case that regressed.
func TestQueue_PipelineDispatchOrder(t *testing.T) {
	const n = 8
	q := New(1)

	// Hold the only slot so every contender parks as a waiter before any drains.
	held, err := q.Acquire(context.Background(), 0)
	if err != nil {
		t.Fatalf("initial Acquire failed: %v", err)
	}

	acquired := make(chan int, n)
	// Launch in a non-sorted order to prove the queue — not goroutine start
	// order — decides who runs next.
	launchOrder := []int{5, 1, 8, 3, 7, 2, 6, 4}
	var started sync.WaitGroup
	for _, idx := range launchOrder {
		started.Add(1)
		index := idx
		go func() {
			started.Done()
			// pipelinePriority(0, index) == -index
			slot, err := q.Acquire(context.Background(), -index)
			if err != nil {
				t.Errorf("waiter %d Acquire failed: %v", index, err)
				return
			}
			acquired <- index
			q.Release(slot)
		}()
	}
	started.Wait()
	waitForWaiters(t, q, n)

	// Release the held slot; the single slot now flows through waiters in order.
	q.Release(held)

	got := make([]int, 0, n)
	for i := 0; i < n; i++ {
		got = append(got, <-acquired)
	}
	for i, idx := range got {
		if idx != i+1 {
			t.Fatalf("dispatch order = %v, want 1..%d ascending", got, n)
		}
	}
}

// TestQueue_SubmitBlocksUntilSlotFree verifies Submit waits for a free slot and
// releases it once fn returns, so a backlogged second submission can proceed.
func TestQueue_SubmitBlocksUntilSlotFree(t *testing.T) {
	q := New(1)

	firstRunning := make(chan struct{})
	releaseFirst := make(chan struct{})
	if err := q.Submit(context.Background(), func(slot int) {
		close(firstRunning)
		<-releaseFirst
	}); err != nil {
		t.Fatalf("first Submit failed: %v", err)
	}
	<-firstRunning

	secondRan := make(chan int, 1)
	submitReturned := make(chan struct{})
	go func() {
		if err := q.Submit(context.Background(), func(slot int) {
			secondRan <- slot
		}); err != nil {
			t.Errorf("second Submit failed: %v", err)
		}
		close(submitReturned)
	}()

	// The second Submit must block while the only slot is held by the first fn.
	select {
	case <-submitReturned:
		t.Fatal("second Submit returned before the slot was free")
	case <-time.After(50 * time.Millisecond):
	}

	close(releaseFirst)

	select {
	case <-secondRan:
	case <-time.After(2 * time.Second):
		t.Fatal("second fn never ran after slot freed")
	}
}

// TestQueue_AcquireContextCancel verifies a cancelled Acquire removes its waiter
// and does not consume a slot, so a subsequent Acquire still succeeds.
func TestQueue_AcquireContextCancel(t *testing.T) {
	q := New(1)

	held, err := q.Acquire(context.Background(), 0)
	if err != nil {
		t.Fatalf("initial Acquire failed: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	acqErr := make(chan error, 1)
	go func() {
		_, err := q.Acquire(ctx, 0)
		acqErr <- err
	}()
	waitForWaiters(t, q, 1)

	cancel()
	if err := <-acqErr; err == nil {
		t.Fatal("cancelled Acquire returned nil error")
	}

	q.mu.Lock()
	leftover := len(q.waiters)
	q.mu.Unlock()
	if leftover != 0 {
		t.Fatalf("waiter not cleaned up after cancel: %d remain", leftover)
	}

	// The slot must not have leaked: releasing the held slot makes it available.
	q.Release(held)
	got, err := q.Acquire(context.Background(), 0)
	if err != nil {
		t.Fatalf("Acquire after cancel failed: %v", err)
	}
	if got != held {
		t.Fatalf("expected slot %d back, got %d", held, got)
	}
}
