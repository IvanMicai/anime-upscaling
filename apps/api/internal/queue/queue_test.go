package queue

import (
	"context"
	"sync"
	"sync/atomic"
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

// waitForGPUWaiters is waitForWaiters for the GPU pool.
func waitForGPUWaiters(t *testing.T, q *GPUQueue, n int) {
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
			t.Fatalf("timed out waiting for %d GPU waiters, have %d", n, got)
		}
		time.Sleep(time.Millisecond)
	}
}

// TestQueue_YieldKeepsSlotWhenNobodyOutranksIt is the step-boundary regression.
// A file that finished one step and is about to start another on the same pool
// used to Release and then Acquire, and in that window Release handed its slot
// to whatever was parked — including files the priority comparator ranks below
// it. Yield must keep the slot instead, leaving the lower-priority waiter parked.
func TestQueue_YieldKeepsSlotWhenNobodyOutranksIt(t *testing.T) {
	q := New(1)

	// The in-flight file holds the only slot, on step 0.
	held, err := q.Acquire(context.Background(), -1)
	if err != nil {
		t.Fatalf("initial Acquire failed: %v", err)
	}

	// A fresh file parks behind it, also on step 0 but later in the list.
	lost := make(chan int, 1)
	go func() {
		slot, err := q.Acquire(context.Background(), -2)
		if err != nil {
			return
		}
		lost <- slot
	}()
	waitForWaiters(t, q, 1)

	// The in-flight file advances to a later step: strictly higher priority.
	got, err := q.Yield(context.Background(), held, 1_000_000-1)
	if err != nil {
		t.Fatalf("Yield failed: %v", err)
	}
	if got != held {
		t.Fatalf("Yield returned slot %d, want the held slot %d", got, held)
	}
	select {
	case slot := <-lost:
		t.Fatalf("lower-priority waiter took slot %d across the step boundary", slot)
	case <-time.After(50 * time.Millisecond):
	}

	// The waiter is still parked and gets the slot once the file is really done.
	q.Release(got)
	select {
	case <-lost:
	case <-time.After(2 * time.Second):
		t.Fatal("waiter never got the slot after the pipeline released it")
	}
}

// TestQueue_YieldHandsOffToHigherPriority is the other half of the contract:
// keeping the slot must not become a way to jump the line. A waiter that
// outranks the new priority takes the slot, and the yielding caller rejoins the
// queue and is served in turn.
func TestQueue_YieldHandsOffToHigherPriority(t *testing.T) {
	q := New(1)

	held, err := q.Acquire(context.Background(), 0)
	if err != nil {
		t.Fatalf("initial Acquire failed: %v", err)
	}

	// A file already on a later step is parked behind us.
	took := make(chan int, 1)
	go func() {
		slot, err := q.Acquire(context.Background(), 1_000_000)
		if err != nil {
			return
		}
		took <- slot
		q.Release(slot)
	}()
	waitForWaiters(t, q, 1)

	yielded := make(chan int, 1)
	go func() {
		slot, err := q.Yield(context.Background(), held, -5)
		if err != nil {
			t.Errorf("Yield failed: %v", err)
			return
		}
		yielded <- slot
	}()

	select {
	case <-took:
	case <-time.After(2 * time.Second):
		t.Fatal("higher-priority waiter never received the yielded slot")
	}
	select {
	case <-yielded:
	case <-time.After(2 * time.Second):
		t.Fatal("yielding caller never got a slot back")
	}
}

// TestQueue_YieldContextCancel verifies a cancelled Yield leaves no waiter
// behind and no slot leaked: the pool must be fully available afterwards.
func TestQueue_YieldContextCancel(t *testing.T) {
	q := New(1)

	held, err := q.Acquire(context.Background(), 0)
	if err != nil {
		t.Fatalf("initial Acquire failed: %v", err)
	}

	// Park a waiter that outranks the yield, so Yield takes the handoff path.
	blocker := make(chan struct{})
	blocked := make(chan int, 1)
	go func() {
		slot, err := q.Acquire(context.Background(), 1_000_000)
		if err != nil {
			return
		}
		blocked <- slot
		<-blocker
		q.Release(slot)
	}()
	waitForWaiters(t, q, 1)

	ctx, cancel := context.WithCancel(context.Background())
	yieldErr := make(chan error, 1)
	go func() {
		_, err := q.Yield(ctx, held, 0)
		yieldErr <- err
	}()

	<-blocked // the handoff happened; the yielder is now parked
	waitForWaiters(t, q, 1)
	cancel()
	if err := <-yieldErr; err == nil {
		t.Fatal("cancelled Yield returned nil error")
	}

	q.mu.Lock()
	leftover := len(q.waiters)
	q.mu.Unlock()
	if leftover != 0 {
		t.Fatalf("waiter not cleaned up after cancelled Yield: %d remain", leftover)
	}

	close(blocker)
	got, err := q.Acquire(context.Background(), 0)
	if err != nil {
		t.Fatalf("Acquire after cancelled Yield failed: %v", err)
	}
	if got != held {
		t.Fatalf("expected the pool's only slot %d back, got %d", held, got)
	}
}

// TestGPUQueue_YieldKeepsSlotAcrossStepBoundary replays the reported GPU
// ordering bug at production shape. Two GPUs are busy with episodes 1 and 2;
// episodes 3 and 4 are parked waiting to start their upscale. When episode 1
// finishes its upscale and moves on to its interpolate — a later step, so a
// strictly higher priority — it must keep its GPU. The old Release/Acquire pair
// gave that GPU to episode 3 instead, which is how "GPU 0 finished E64 and
// immediately started Kai E01" showed up in the logs while E64's own
// interpolate waited.
func TestGPUQueue_YieldKeepsSlotAcrossStepBoundary(t *testing.T) {
	q := NewGPUQueue(2, 1)

	ep1GPU, ep1Stream, err := q.Acquire(context.Background(), pipelinePrio(0, 1))
	if err != nil {
		t.Fatalf("episode 1 Acquire failed: %v", err)
	}
	ep2GPU, ep2Stream, err := q.Acquire(context.Background(), pipelinePrio(0, 2))
	if err != nil {
		t.Fatalf("episode 2 Acquire failed: %v", err)
	}

	started := make(chan int, 2)
	for _, idx := range []int{3, 4} {
		index := idx
		go func() {
			gpuID, streamIdx, err := q.Acquire(context.Background(), pipelinePrio(0, index))
			if err != nil {
				return
			}
			started <- index
			q.Release(gpuID, streamIdx)
		}()
	}
	waitForGPUWaiters(t, q, 2)

	// Episode 1 advances upscale -> interpolate.
	gotGPU, gotStream, err := q.Yield(context.Background(), ep1GPU, ep1Stream, pipelinePrio(2, 1))
	if err != nil {
		t.Fatalf("episode 1 Yield failed: %v", err)
	}
	if gotGPU != ep1GPU || gotStream != ep1Stream {
		t.Fatalf("episode 1 moved to GPU %d/%d, want to keep %d/%d", gotGPU, gotStream, ep1GPU, ep1Stream)
	}
	select {
	case idx := <-started:
		t.Fatalf("episode %d jumped onto the GPU across a step boundary", idx)
	case <-time.After(50 * time.Millisecond):
	}

	// The waiting episodes start only once a running one is truly finished, and
	// the earlier one goes first. Released one at a time so the assertion tests
	// the handoff order rather than goroutine scheduling.
	q.Release(gotGPU, gotStream)
	if first := <-started; first != 3 {
		t.Fatalf("first freed GPU went to episode %d, want episode 3", first)
	}
	q.Release(ep2GPU, ep2Stream)
	if second := <-started; second != 4 {
		t.Fatalf("second freed GPU went to episode %d, want episode 4", second)
	}
}

// pipelinePrio mirrors process.pipelinePriority (stepIdx*1_000_000 - index),
// duplicated here because the queue package cannot import process.
func pipelinePrio(stepIdx, index int) int { return stepIdx*1_000_000 - index }

// TestQueue_ResumedRunServesFFmpegInAlphabeticalOrder replays the reported
// FFmpeg starvation at the queue level, with the priorities a correctly ordered
// plan produces. Optimize is the only step of that pipeline that touches the
// FFmpeg pool, so every waiter carries the same step term and the tiebreak is
// purely the file's alphabetical rank: the three episodes stranded by the
// cancelled run must be served before the fresh ones that reach the same step
// later, not after all of them.
func TestQueue_ResumedRunServesFFmpegInAlphabeticalOrder(t *testing.T) {
	const optimizeStep = 4
	// Alphabetical ranks from the plan: Digimon E39..E41 (1..3) are already
	// running on the three slots; these six are queued behind them.
	queued := []struct {
		name string
		rank int
	}{
		{"Digimon Tamers S01E42", 4},
		{"Dragon Ball GT S01E55", 5},
		{"Dragon Ball GT S01E57", 6},
		{"Dragon Ball GT S01E58", 7},
		{"Dragon Ball GT S01E59", 8},
		{"Dragon Ball GT S01E60", 9},
	}

	q := New(3)
	held := make([]int, 0, 3)
	for range [3]struct{}{} {
		slot, err := q.Acquire(context.Background(), pipelinePrio(optimizeStep, 1))
		if err != nil {
			t.Fatalf("holding a slot failed: %v", err)
		}
		held = append(held, slot)
	}

	started := make(chan string, len(queued))
	finish := make(chan struct{})
	var wg sync.WaitGroup
	// Launch scrambled: the queue, not goroutine start order, decides.
	for _, i := range []int{3, 0, 5, 1, 4, 2} {
		f := queued[i]
		wg.Add(1)
		go func() {
			defer wg.Done()
			slot, err := q.Acquire(context.Background(), pipelinePrio(optimizeStep, f.rank))
			if err != nil {
				return
			}
			started <- f.name
			// Occupy the slot like a real encode would, so exactly one file
			// starts per freed slot and the assertion tests the handoff rather
			// than goroutine scheduling.
			<-finish
			q.Release(slot)
		}()
	}
	waitForWaiters(t, q, len(queued))

	// The three running encodes finish one at a time. Each freed slot must go to
	// a stranded episode, in alphabetical order — this is the exact moment the
	// production run handed all three to Dragon Ball GT S01E58/E59/E60 instead.
	for i, want := range queued[:len(held)] {
		q.Release(held[i])
		select {
		case got := <-started:
			if got != want.name {
				t.Fatalf("free slot %d went to %q, want %q", i+1, got, want.name)
			}
		case <-time.After(2 * time.Second):
			t.Fatalf("no file picked up free slot %d", i+1)
		}
	}

	// The rest are freed at once, so only their completeness is deterministic:
	// nothing may be left behind.
	close(finish)
	wg.Wait()
	ran := map[string]bool{}
	for len(started) > 0 {
		ran[<-started] = true
	}
	for _, f := range queued[len(held):] {
		if !ran[f.name] {
			t.Errorf("%s never ran", f.name)
		}
	}
}

// TestQueue_ReleaseTiebreakIsParkOrder pins the FIFO fallback the ordering
// scheme leans on: files that compare equal — two jobs' files at the same step
// and rank, say — are served in the order they started waiting, never reordered
// arbitrarily.
func TestQueue_ReleaseTiebreakIsParkOrder(t *testing.T) {
	q := New(1)

	held, err := q.Acquire(context.Background(), 0)
	if err != nil {
		t.Fatalf("initial Acquire failed: %v", err)
	}

	started := make(chan int, 3)
	for i := 1; i <= 3; i++ {
		index := i
		go func() {
			slot, err := q.Acquire(context.Background(), 7) // identical priority
			if err != nil {
				return
			}
			started <- index
			q.Release(slot)
		}()
		// Park them one at a time so "park order" is well defined.
		waitForWaiters(t, q, index)
	}

	q.Release(held)
	for want := 1; want <= 3; want++ {
		select {
		case got := <-started:
			if got != want {
				t.Fatalf("equal-priority waiters served in the wrong order: got %d, want %d", got, want)
			}
		case <-time.After(2 * time.Second):
			t.Fatalf("waiter %d never ran", want)
		}
	}
}

// TestQueue_YieldStormRespectsCapacity stresses the property Yield must not
// break: however slots are swapped at step boundaries, no more callers may be
// running on the pool at once than it has slots, and every slot must find its
// way home afterwards.
//
// Ownership transfers the instant Yield hands the slot to a higher-priority
// waiter, which is before Yield returns — so a caller counts as running only
// between a successful acquire and its next Yield/Release, which is exactly how
// the pipeline uses it.
func TestQueue_YieldStormRespectsCapacity(t *testing.T) {
	const slots, workers, rounds = 3, 12, 200
	q := New(slots)

	var running, peak int64
	enter := func() {
		if n := atomic.AddInt64(&running, 1); n > atomic.LoadInt64(&peak) {
			atomic.StoreInt64(&peak, n)
		}
		if n := atomic.LoadInt64(&running); n > slots {
			t.Errorf("%d callers running on a %d-slot pool", n, slots)
		}
	}
	leave := func() { atomic.AddInt64(&running, -1) }

	var wg sync.WaitGroup
	for w := 0; w < workers; w++ {
		id := w
		wg.Add(1)
		go func() {
			defer wg.Done()
			for r := 0; r < rounds; r++ {
				ctx, cancel := context.WithCancel(context.Background())
				// Cancel a quarter of the rounds mid-flight to exercise both
				// cancellation paths.
				if (id+r)%4 == 0 {
					go func() {
						time.Sleep(time.Duration((id*7+r)%200) * time.Microsecond)
						cancel()
					}()
				}
				slot, err := q.Acquire(ctx, (id*31+r*17)%1000)
				if err != nil {
					cancel()
					continue
				}
				enter()
				lost := false
				for s := 0; s < (id+r)%4; s++ {
					leave()
					next, err := q.Yield(ctx, slot, (id*13+s*29)%1000)
					if err != nil {
						lost = true // Yield released it on our behalf.
						break
					}
					slot = next
					enter()
				}
				if !lost {
					leave()
					q.Release(slot)
				}
				cancel()
			}
		}()
	}
	wg.Wait()

	if got := atomic.LoadInt64(&running); got != 0 {
		t.Errorf("%d callers still running after the storm", got)
	}
	if atomic.LoadInt64(&peak) == 0 {
		t.Fatal("the storm never acquired anything — the test proves nothing")
	}
	q.mu.Lock()
	free, waiters := len(q.free), len(q.waiters)
	q.mu.Unlock()
	if free != slots {
		t.Errorf("pool came back with %d slots, want %d", free, slots)
	}
	if waiters != 0 {
		t.Errorf("%d waiters leaked", waiters)
	}
}

// TestGPUQueue_YieldStormRespectsCapacity is the GPU pool's version of the
// capacity/conservation storm, and additionally checks no (gpu, stream) pair is
// duplicated or lost.
func TestGPUQueue_YieldStormRespectsCapacity(t *testing.T) {
	const gpus, streams, workers, rounds = 2, 2, 10, 200
	const total = gpus * streams
	q := NewGPUQueue(gpus, streams)

	var running int64
	var wg sync.WaitGroup
	for w := 0; w < workers; w++ {
		id := w
		wg.Add(1)
		go func() {
			defer wg.Done()
			for r := 0; r < rounds; r++ {
				ctx, cancel := context.WithCancel(context.Background())
				if (id+r)%4 == 0 {
					go func() {
						time.Sleep(time.Duration((id*11+r)%200) * time.Microsecond)
						cancel()
					}()
				}
				gpuID, streamIdx, err := q.Acquire(ctx, (id*31+r*17)%1000)
				if err != nil {
					cancel()
					continue
				}
				if n := atomic.AddInt64(&running, 1); n > total {
					t.Errorf("%d callers running on a %d-slot GPU pool", n, total)
				}
				lost := false
				for s := 0; s < (id+r)%4; s++ {
					atomic.AddInt64(&running, -1)
					gpuID, streamIdx, err = q.Yield(ctx, gpuID, streamIdx, (id*13+s*29)%1000)
					if err != nil {
						lost = true
						break
					}
					if n := atomic.AddInt64(&running, 1); n > total {
						t.Errorf("%d callers running on a %d-slot GPU pool", n, total)
					}
				}
				if !lost {
					atomic.AddInt64(&running, -1)
					q.Release(gpuID, streamIdx)
				}
				cancel()
			}
		}()
	}
	wg.Wait()

	if got := atomic.LoadInt64(&running); got != 0 {
		t.Errorf("%d callers still running after the storm", got)
	}
	q.mu.Lock()
	got := append([]gpuSlot(nil), q.slots...)
	waiters := len(q.waiters)
	q.mu.Unlock()
	if waiters != 0 {
		t.Errorf("%d waiters leaked", waiters)
	}
	seen := map[gpuSlot]int{}
	for _, s := range got {
		seen[s]++
	}
	for g := 0; g < gpus; g++ {
		for s := 0; s < streams; s++ {
			if n := seen[gpuSlot{gpuID: g, streamIdx: s}]; n != 1 {
				t.Errorf("slot gpu%d/stream%d came back %d times, want exactly 1", g, s, n)
			}
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

// TestGPUQueue_OrderedAdmissionRelay reproduces StartPipelineJob's admission
// relay against a GPUQueue with FREE slots — the scenario the priority argument
// alone does NOT cover, because Acquire's free-slot fast path serves whoever
// grabs the mutex first. The relay gates each file so that file i's Acquire
// returns before file i+1's runs, guaranteeing index-ordered entry even when
// the goroutines are launched out of order.
func TestGPUQueue_OrderedAdmissionRelay(t *testing.T) {
	const n = 12
	q := NewGPUQueue(2, 1) // 2 GPU slots free at start — exercises the fast path

	gates := make([]chan struct{}, n+1)
	for i := range gates {
		gates[i] = make(chan struct{}, 1)
	}
	gates[0] <- struct{}{} // admit the first file

	acquired := make(chan int, n)
	// Launch in scrambled order to prove the relay — not goroutine start order —
	// decides entry order.
	launchOrder := []int{9, 2, 11, 4, 7, 1, 12, 5, 10, 3, 8, 6}
	var wg sync.WaitGroup
	for _, idx := range launchOrder {
		wg.Add(1)
		index := idx
		gateIn := gates[index-1]
		gateOut := gates[index]
		go func() {
			defer wg.Done()
			<-gateIn
			// pipelinePriority(0, index) == -index
			gpuID, streamIdx, err := q.Acquire(context.Background(), -index)
			if err != nil {
				t.Errorf("file %d Acquire failed: %v", index, err)
				return
			}
			// Record entry order at the point of acquisition, BEFORE admitting the
			// next file — this is the guarantee the fix provides.
			acquired <- index
			gateOut <- struct{}{}
			// Hold the slot briefly so later files contend as waiters too.
			time.Sleep(time.Millisecond)
			q.Release(gpuID, streamIdx)
		}()
	}

	got := make([]int, 0, n)
	for i := 0; i < n; i++ {
		got = append(got, <-acquired)
	}
	wg.Wait()

	for i, idx := range got {
		if idx != i+1 {
			t.Fatalf("admission order = %v, want 1..%d ascending", got, n)
		}
	}
}

// TestQueues_PerPoolRelayKeepsBothPoolsBusy reproduces the starvation that
// StartPipelineJob's per-pool admission relay exists to prevent, using the shape
// of a resumed pipeline run: many files whose first step needs a GPU, plus a few
// stranded files whose first step needs FFmpeg.
//
// The GPU workload is larger than the GPU pool, so a SINGLE global relay stalls
// forever at the first file that cannot get a GPU slot — every later file,
// including the FFmpeg-bound ones, never receives its gate token and the FFmpeg
// pool sits idle with work ready for it. Giving each pool its own relay lets
// them advance independently while preserving order within each pool.
func TestQueues_PerPoolRelayKeepsBothPoolsBusy(t *testing.T) {
	const gpuFiles, ffmpegFiles = 8, 3
	gpuQ := NewGPUQueue(2, 1) // deliberately smaller than the GPU workload
	ffmpegQ := New(ffmpegFiles)

	hold := make(chan struct{}) // keeps every acquired GPU slot occupied
	admitted := make(chan int, ffmpegFiles)
	var wg sync.WaitGroup

	// GPU pool relay.
	gpuGates := make([]chan struct{}, gpuFiles+1)
	for i := range gpuGates {
		gpuGates[i] = make(chan struct{}, 1)
	}
	gpuGates[0] <- struct{}{}
	for i := 0; i < gpuFiles; i++ {
		wg.Add(1)
		gateIn, gateOut := gpuGates[i], gpuGates[i+1]
		go func() {
			defer wg.Done()
			<-gateIn
			gpuID, streamIdx, err := gpuQ.Acquire(context.Background(), 0)
			if err != nil {
				return
			}
			gateOut <- struct{}{} // admit next only after our Acquire returned
			<-hold
			gpuQ.Release(gpuID, streamIdx)
		}()
	}

	// FFmpeg pool relay — independent of the GPU cascade above.
	ffGates := make([]chan struct{}, ffmpegFiles+1)
	for i := range ffGates {
		ffGates[i] = make(chan struct{}, 1)
	}
	ffGates[0] <- struct{}{}
	for i := 0; i < ffmpegFiles; i++ {
		wg.Add(1)
		index := i
		gateIn, gateOut := ffGates[i], ffGates[i+1]
		go func() {
			defer wg.Done()
			<-gateIn
			slot, err := ffmpegQ.Acquire(context.Background(), 0)
			if err != nil {
				return
			}
			admitted <- index
			gateOut <- struct{}{}
			ffmpegQ.Release(slot)
		}()
	}

	// Every FFmpeg-bound file must reach its pool while the GPU pool is fully
	// saturated and its own relay is stalled.
	got := make([]int, 0, ffmpegFiles)
	for i := 0; i < ffmpegFiles; i++ {
		select {
		case idx := <-admitted:
			got = append(got, idx)
		case <-time.After(5 * time.Second):
			t.Fatalf("FFmpeg pool starved: only %d of %d files admitted (%v) while GPUs were busy",
				len(got), ffmpegFiles, got)
		}
	}
	for i, idx := range got {
		if idx != i {
			t.Fatalf("FFmpeg admission order = %v, want 0..%d ascending", got, ffmpegFiles-1)
		}
	}

	close(hold) // let the GPU cascade drain
	wg.Wait()
}
