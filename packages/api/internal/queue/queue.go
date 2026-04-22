package queue

import (
	"context"
	"sync"
)

// Queue is a simple worker pool with N concurrent slots.
// Work is processed FIFO as slots become available.
// The function receives its slot index (0..slots-1) so callers can disambiguate
// concurrent invocations for logging/progress labeling.
type Queue struct {
	sem chan int
}

// New creates a Queue with the given number of concurrent slots.
func New(slots int) *Queue {
	if slots < 1 {
		slots = 1
	}
	q := &Queue{sem: make(chan int, slots)}
	for i := 0; i < slots; i++ {
		q.sem <- i
	}
	return q
}

// Submit enqueues fn for execution. Blocks until a slot is free or ctx is cancelled.
// fn receives the slot index it is running in.
func (q *Queue) Submit(ctx context.Context, fn func(slot int)) error {
	var slot int
	select {
	case slot = <-q.sem:
	case <-ctx.Done():
		return ctx.Err()
	}
	go func() {
		defer func() { q.sem <- slot }()
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

// GPUQueue is a priority-aware worker pool where each slot is associated with a
// (gpuID, streamIdx) pair. Multiple streams per GPU mean the same gpuID can be
// acquired multiple times concurrently, with distinct streamIdx values so callers
// can produce unique log files and tracker labels.
type GPUQueue struct {
	mu      sync.Mutex
	slots   []gpuSlot
	waiters []gpuWaiter
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
func (q *GPUQueue) Acquire(ctx context.Context, priority int) (int, int, error) {
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
