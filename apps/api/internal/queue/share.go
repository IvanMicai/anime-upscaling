package queue

import (
	"context"
	"sync"
)

// GPUShares arbitrates what runs on a GPU at the same time, on top of the
// GPUQueue slots. A slot says a caller may use a GPU; a share says how much of
// it. video2x and RIFE take one share each, so STREAMS_PER_GPU of them fit side
// by side as before; SeedVR2 fills a 16 GB card by itself and takes every
// share, so it waits for the other streams on its GPU to finish and holds them
// off until it is done.
//
// A caller acquires its share only while it already holds a GPUQueue slot on
// that GPU and releases it before giving the slot up, so the shares in use
// never exceed the slots handed out and a waiter never blocks a holder: holders
// only run their process and release. Waiters are served in arrival order, so
// a whole-GPU request is not starved by a stream of single shares.
type GPUShares struct {
	mu   sync.Mutex
	gpus map[int]*gpuShare
}

type gpuShare struct {
	used    int
	waiters []*shareWaiter
}

type shareWaiter struct {
	weight int
	ready  chan struct{}
}

// NewGPUShares returns an empty arbiter.
func NewGPUShares() *GPUShares {
	return &GPUShares{gpus: make(map[int]*gpuShare)}
}

// Acquire takes weight shares of gpuID out of capacity (the GPU's stream
// count), blocking until they are free or ctx is done. weight is clamped to
// [1, capacity]. The returned release must be called exactly once.
func (g *GPUShares) Acquire(ctx context.Context, gpuID, weight, capacity int) (release func(), err error) {
	if capacity < 1 {
		capacity = 1
	}
	weight = min(max(weight, 1), capacity)

	g.mu.Lock()
	s := g.gpus[gpuID]
	if s == nil {
		s = &gpuShare{}
		g.gpus[gpuID] = s
	}
	if len(s.waiters) == 0 && s.used+weight <= capacity {
		s.used += weight
		g.mu.Unlock()
		return g.releaser(s, weight, capacity), nil
	}
	w := &shareWaiter{weight: weight, ready: make(chan struct{})}
	s.waiters = append(s.waiters, w)
	g.mu.Unlock()

	select {
	case <-w.ready:
		return g.releaser(s, weight, capacity), nil
	case <-ctx.Done():
		g.mu.Lock()
		defer g.mu.Unlock()
		for i, ww := range s.waiters {
			if ww == w {
				s.waiters = append(s.waiters[:i], s.waiters[i+1:]...)
				// Leaving the head may let the next waiters in.
				g.grant(s, capacity)
				return nil, ctx.Err()
			}
		}
		// Granted between ctx cancel and the lock: give the shares back.
		s.used -= weight
		g.grant(s, capacity)
		return nil, ctx.Err()
	}
}

func (g *GPUShares) releaser(s *gpuShare, weight, capacity int) func() {
	var once sync.Once
	return func() {
		once.Do(func() {
			g.mu.Lock()
			s.used -= weight
			g.grant(s, capacity)
			g.mu.Unlock()
		})
	}
}

// grant admits waiters from the head of the line while they fit. Callers hold g.mu.
func (g *GPUShares) grant(s *gpuShare, capacity int) {
	for len(s.waiters) > 0 && s.used+s.waiters[0].weight <= capacity {
		w := s.waiters[0]
		s.waiters = s.waiters[1:]
		s.used += w.weight
		close(w.ready)
	}
}
