package queue

import (
	"context"
	"sync"
)

// Queue is a priority-aware worker pool with N concurrent slots.
// Each slot has an index (0..slots-1) so callers can disambiguate concurrent
// invocations for logging/progress labeling. When slots are busy, waiters are
// served highest-priority-first (mirrors GPUQueue), so callers that launch all
// their work goroutines up front — e.g. custom pipelines — still dispatch items
// in a deterministic order via Acquire's priority argument.
type Queue struct {
	mu      sync.Mutex
	free    []int
	waiters []queueWaiter
}

type queueWaiter struct {
	priority int
	ch       chan int
}

// New creates a Queue with the given number of concurrent slots.
func New(slots int) *Queue {
	if slots < 1 {
		slots = 1
	}
	free := make([]int, slots)
	for i := range free {
		free[i] = i
	}
	return &Queue{free: free}
}

// Acquire blocks until a slot is available or ctx is cancelled, returning the
// slot index. Higher priority values are served first when multiple callers are
// waiting. The caller must call Release with the returned slot when done.
func (q *Queue) Acquire(ctx context.Context, priority int) (int, error) {
	q.mu.Lock()
	if len(q.free) > 0 {
		slot := q.free[len(q.free)-1]
		q.free = q.free[:len(q.free)-1]
		q.mu.Unlock()
		return slot, nil
	}
	w := queueWaiter{priority: priority, ch: make(chan int, 1)}
	q.waiters = append(q.waiters, w)
	q.mu.Unlock()

	select {
	case slot := <-w.ch:
		return slot, nil
	case <-ctx.Done():
		q.mu.Lock()
		for i, ww := range q.waiters {
			if ww.ch == w.ch {
				q.waiters = append(q.waiters[:i], q.waiters[i+1:]...)
				q.mu.Unlock()
				return 0, ctx.Err()
			}
		}
		// Waiter was already fulfilled between ctx cancel and lock — return the slot.
		q.mu.Unlock()
		slot := <-w.ch
		q.Release(slot)
		return 0, ctx.Err()
	}
}

// Release returns a slot back to the pool, waking the highest-priority waiter if any.
func (q *Queue) Release(slot int) {
	q.mu.Lock()
	if len(q.waiters) == 0 {
		q.free = append(q.free, slot)
		q.mu.Unlock()
		return
	}
	best := 0
	for i := 1; i < len(q.waiters); i++ {
		if q.waiters[i].priority > q.waiters[best].priority {
			best = i
		}
	}
	w := q.waiters[best]
	q.waiters = append(q.waiters[:best], q.waiters[best+1:]...)
	q.mu.Unlock()
	w.ch <- slot
}

// Submit enqueues fn for execution. Blocks until a slot is free or ctx is cancelled,
// then runs fn in a goroutine and releases the slot when it returns. fn receives the
// slot index it is running in. Equivalent to an Acquire(priority 0)/Release pair, kept
// for callers that dispatch sequentially (where slot order is already deterministic).
func (q *Queue) Submit(ctx context.Context, fn func(slot int)) error {
	slot, err := q.Acquire(ctx, 0)
	if err != nil {
		return err
	}
	go func() {
		defer q.Release(slot)
		fn(slot)
	}()
	return nil
}

type gpuSlot struct {
	gpuID     int
	streamIdx int
}

type gpuWaiter struct {
	priority int
	ch       chan gpuSlot
}

// GateFunc is an optional pre-acquisition hook. If set on a GPUQueue, every
// Acquire calls it before consuming a slot; a non-nil return aborts the
// acquisition and is propagated to the caller. Used to block dispatch while
// the GPU driver is wedged.
type GateFunc func(ctx context.Context) error

// GPUQueue is a priority-aware worker pool where each slot is associated with a
// (gpuID, streamIdx) pair. Multiple streams per GPU mean the same gpuID can be
// acquired multiple times concurrently, with distinct streamIdx values so callers
// can produce unique log files and tracker labels.
type GPUQueue struct {
	mu      sync.Mutex
	slots   []gpuSlot
	waiters []gpuWaiter
	gate    GateFunc
}

// SetGate installs a pre-acquisition gate. Pass nil to clear.
func (q *GPUQueue) SetGate(g GateFunc) {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.gate = g
}

func (q *GPUQueue) currentGate() GateFunc {
	q.mu.Lock()
	defer q.mu.Unlock()
	return q.gate
}

// NewGPUQueue creates a GPUQueue with gpuCount GPUs and streamsPerGPU slots each.
// Slot ordering has GPU ids interleaved so that the first streamsPerGPU acquires
// always cover distinct GPUs when possible.
func NewGPUQueue(gpuCount, streamsPerGPU int) *GPUQueue {
	if gpuCount < 1 {
		gpuCount = 1
	}
	if streamsPerGPU < 1 {
		streamsPerGPU = 1
	}
	slots := make([]gpuSlot, 0, gpuCount*streamsPerGPU)
	// Interleave: (gpu0,s0), (gpu1,s0), ..., (gpu0,s1), (gpu1,s1), ...
	for s := 0; s < streamsPerGPU; s++ {
		for g := 0; g < gpuCount; g++ {
			slots = append(slots, gpuSlot{gpuID: g, streamIdx: s})
		}
	}
	return &GPUQueue{slots: slots}
}

// Acquire blocks until a GPU slot is available or ctx is cancelled.
// Higher priority values are served first when multiple callers are waiting.
// If a gate is installed, it runs first; a non-nil return aborts acquisition.
func (q *GPUQueue) Acquire(ctx context.Context, priority int) (int, int, error) {
	if gate := q.currentGate(); gate != nil {
		if err := gate(ctx); err != nil {
			return 0, 0, err
		}
	}
	q.mu.Lock()
	if len(q.slots) > 0 {
		s := q.slots[len(q.slots)-1]
		q.slots = q.slots[:len(q.slots)-1]
		q.mu.Unlock()
		return s.gpuID, s.streamIdx, nil
	}
	w := gpuWaiter{priority: priority, ch: make(chan gpuSlot, 1)}
	q.waiters = append(q.waiters, w)
	q.mu.Unlock()

	select {
	case s := <-w.ch:
		return s.gpuID, s.streamIdx, nil
	case <-ctx.Done():
		q.mu.Lock()
		for i, ww := range q.waiters {
			if ww.ch == w.ch {
				q.waiters = append(q.waiters[:i], q.waiters[i+1:]...)
				q.mu.Unlock()
				return 0, 0, ctx.Err()
			}
		}
		// Waiter was already fulfilled between ctx cancel and lock — return the slot.
		q.mu.Unlock()
		s := <-w.ch
		q.Release(s.gpuID, s.streamIdx)
		return 0, 0, ctx.Err()
	}
}

// Release returns a GPU slot back to the pool, waking the highest-priority waiter if any.
func (q *GPUQueue) Release(gpuID, streamIdx int) {
	s := gpuSlot{gpuID: gpuID, streamIdx: streamIdx}
	q.mu.Lock()
	if len(q.waiters) == 0 {
		q.slots = append(q.slots, s)
		q.mu.Unlock()
		return
	}
	best := 0
	for i := 1; i < len(q.waiters); i++ {
		if q.waiters[i].priority > q.waiters[best].priority {
			best = i
		}
	}
	w := q.waiters[best]
	q.waiters = append(q.waiters[:best], q.waiters[best+1:]...)
	q.mu.Unlock()
	w.ch <- s
}
