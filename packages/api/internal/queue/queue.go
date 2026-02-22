package queue

import "context"

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

// GPUQueue is a worker pool where each slot is associated with a GPU ID.
// When a slot is acquired, the caller receives the GPU ID to use.
type GPUQueue struct {
	ch chan int
}

// NewGPUQueue creates a GPUQueue with GPU IDs from 0 to gpuCount-1.
func NewGPUQueue(gpuCount int) *GPUQueue {
	ch := make(chan int, gpuCount)
	for i := 0; i < gpuCount; i++ {
		ch <- i
	}
	return &GPUQueue{ch: ch}
}

// Acquire blocks until a GPU slot is available or ctx is cancelled.
// Returns the GPU ID to use.
func (q *GPUQueue) Acquire(ctx context.Context) (int, error) {
	select {
	case id := <-q.ch:
		return id, nil
	case <-ctx.Done():
		return 0, ctx.Err()
	}
}

// Release returns a GPU ID back to the pool.
func (q *GPUQueue) Release(gpuID int) {
	q.ch <- gpuID
}
