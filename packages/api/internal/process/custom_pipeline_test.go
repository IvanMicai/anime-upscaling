package process

import (
	"context"
	"sync/atomic"
	"testing"

	"anime-upscaling/internal/config"
	"anime-upscaling/internal/logger"
	"anime-upscaling/internal/pipeline"
	"anime-upscaling/internal/queue"
	"anime-upscaling/internal/runner"
)

// TestPipelinePriority_LowerIndexWinsWithinStep verifies that, within the same
// step, the earlier file (lower index in the natural-sorted list) gets a higher
// priority value — so the queue serves episode 1 before episode 2, etc.
func TestPipelinePriority_LowerIndexWinsWithinStep(t *testing.T) {
	const step = 0
	prev := pipelinePriority(step, 1)
	for idx := 2; idx <= 100; idx++ {
		cur := pipelinePriority(step, idx)
		if cur >= prev {
			t.Fatalf("priority not strictly decreasing with index: idx %d got %d, previous %d", idx, cur, prev)
		}
		prev = cur
	}
}

// TestPipelinePriority_LaterStepDominates verifies that any file on a later step
// outranks any file on an earlier step, regardless of index — so episodes
// already advanced in the pipeline finish before new fronts open. The guard
// holds for batches up to pipelineStepWeight files.
func TestPipelinePriority_LaterStepDominates(t *testing.T) {
	const batch = 10_000 // well under pipelineStepWeight (1_000_000)
	for step := 0; step < 4; step++ {
		// Lowest priority on the later step (highest index) must still beat the
		// highest priority on the earlier step (index 1).
		laterWorst := pipelinePriority(step+1, batch)
		earlierBest := pipelinePriority(step, 1)
		if laterWorst <= earlierBest {
			t.Fatalf("step %d index %d (prio %d) did not dominate step %d index 1 (prio %d)",
				step+1, batch, laterWorst, step, earlierBest)
		}
	}
}

// TestRunCustomPipelineForFile_AdmitsNextOnEarlyReturn verifies the deadlock
// backstop: when the function returns before any queue Acquire happens (here,
// the context is already cancelled), the deferred admit() still fires exactly
// once. Without this, the ordered-admission relay in StartPipelineJob would
// stall — the next file's gate would never receive its token.
func TestRunCustomPipelineForFile_AdmitsNextOnEarlyReturn(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancelled before the step loop reaches any Acquire

	var admits int32
	admitNext := func() { atomic.AddInt32(&admits, 1) }

	steps := []pipeline.PipelineStep{{Operation: "upscale", Scale: 2}}
	gpuQ := queue.NewGPUQueue(2, 1)
	ffmpegQ := queue.New(1)

	ok := RunCustomPipelineForFile(
		ctx, config.Config{}, nil, gpuQ, ffmpegQ, steps,
		"ep.mkv", 1, "input", admitNext,
		func(logger.JobLog) {}, func(runner.Progress) {},
	)
	if ok {
		t.Fatal("expected RunCustomPipelineForFile to fail on cancelled context")
	}
	if got := atomic.LoadInt32(&admits); got != 1 {
		t.Fatalf("admitNext called %d times, want exactly 1", got)
	}
}
