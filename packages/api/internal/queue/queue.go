package queue

import (
	"context"
	"sync"
)

// Queue is a simple worker pool with N concurrent slots.
// Work is processed FIFO as slots become available.
type Queue struct {
	sem chan struct{}
}

// New creates a Queue with the given number of concurrent slots.
func New(slots int) *Queue {
	return &Queue{sem: make(chan struct{}, slots)}
}

// Submit enqueues fn for execution. Blocks until a slot is free or ctx is cancelled.
func (q *Queue) Submit(ctx context.Context, fn func()) error {
	select {
	case q.sem <- struct{}{}:
	case <-ctx.Done():
		return ctx.Err()
	}
	go func() {
		defer func() { <-q.sem }()
		fn()
	}()
	return nil
}

type gpuWaiter struct {
	priority int
	ch       chan int
}

// GPUQueue is a priority-aware worker pool where each slot is associated with a GPU ID.
// When multiple callers are waiting, the one with the highest priority is served first.
// Within the same priority, waiters are served FIFO.
type GPUQueue struct {
	mu      sync.Mutex
	gpus    []int
	waiters []gpuWaiter
}

// NewGPUQueue creates a GPUQueue with GPU IDs from 0 to gpuCount-1.
func NewGPUQueue(gpuCount int) *GPUQueue {
	gpus := make([]int, gpuCount)
	for i := range gpus {
		gpus[i] = i
	}
	return &GPUQueue{gpus: gpus}
}

// Acquire blocks until a GPU slot is available or ctx is cancelled.
// Higher priority values are served first when multiple callers are waiting.
func (q *GPUQueue) Acquire(ctx context.Context, priority int) (int, error) {
	q.mu.Lock()
	if len(q.gpus) > 0 {
		id := q.gpus[len(q.gpus)-1]
		q.gpus = q.gpus[:len(q.gpus)-1]
		q.mu.Unlock()
		return id, nil
	}
	w := gpuWaiter{priority: priority, ch: make(chan int, 1)}
	q.waiters = append(q.waiters, w)
	q.mu.Unlock()

	select {
	case id := <-w.ch:
		return id, nil
	case <-ctx.Done():
		q.mu.Lock()
		for i, ww := range q.waiters {
			if ww.ch == w.ch {
				q.waiters = append(q.waiters[:i], q.waiters[i+1:]...)
				q.mu.Unlock()
				return 0, ctx.Err()
			}
		}
		// Waiter was already fulfilled between ctx cancel and lock — return the GPU.
		q.mu.Unlock()
		id := <-w.ch
		q.Release(id)
		return 0, ctx.Err()
	}
}

// Release returns a GPU ID back to the pool, waking the highest-priority waiter if any.
func (q *GPUQueue) Release(gpuID int) {
	q.mu.Lock()
	if len(q.waiters) == 0 {
		q.gpus = append(q.gpus, gpuID)
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
	w.ch <- gpuID
}
