package process

import (
	"context"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

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

// TestPipelineSlots_CarriesSlotAcrossStepBoundary verifies the file keeps the
// pool slot it already holds when it moves to another step on the same pool, and
// that a lower-priority file parked behind it does not get to slip in. This is
// the inversion the plain Release/Acquire pair allowed: the cleanup step between
// two GPU steps left the file holding nothing, so its GPU went to whichever
// freshly started episode happened to be waiting.
func TestPipelineSlots_CarriesSlotAcrossStepBoundary(t *testing.T) {
	gpuQ := queue.NewGPUQueue(1, 1)
	ffmpegQ := queue.New(1)
	ctx := context.Background()

	var slots pipelineSlots
	if err := slots.acquireGPU(ctx, gpuQ, ffmpegQ, pipelinePriority(0, 1)); err != nil {
		t.Fatalf("upscale acquire failed: %v", err)
	}
	gotGPU, gotStream := slots.gpuID, slots.streamIdx

	// A later episode still on step 0 parks behind us.
	jumped := make(chan struct{})
	go func() {
		gpuID, streamIdx, err := gpuQ.Acquire(ctx, pipelinePriority(0, 2))
		if err != nil {
			return
		}
		close(jumped)
		gpuQ.Release(gpuID, streamIdx)
	}()
	// Give the contender time to park before the step boundary.
	time.Sleep(20 * time.Millisecond)

	if err := slots.acquireGPU(ctx, gpuQ, ffmpegQ, pipelinePriority(2, 1)); err != nil {
		t.Fatalf("interpolate acquire failed: %v", err)
	}
	if slots.gpuID != gotGPU || slots.streamIdx != gotStream {
		t.Fatalf("moved to GPU %d/%d across the step boundary, want to keep %d/%d",
			slots.gpuID, slots.streamIdx, gotGPU, gotStream)
	}
	select {
	case <-jumped:
		t.Fatal("a lower-priority episode took the GPU across the step boundary")
	case <-time.After(50 * time.Millisecond):
	}

	slots.release(gpuQ, ffmpegQ)
	select {
	case <-jumped:
	case <-time.After(2 * time.Second):
		t.Fatal("waiting episode never got the GPU after the pipeline released it")
	}
}

// TestPipelineSlots_NeverHoldsTwoPools verifies a file hands its GPU back the
// moment it moves to an FFmpeg step, and vice versa. Holding both would idle a
// worker that another episode is waiting for — the pools are sized to run
// concurrently, not to be reserved by one file.
func TestPipelineSlots_NeverHoldsTwoPools(t *testing.T) {
	gpuQ := queue.NewGPUQueue(1, 1)
	ffmpegQ := queue.New(1)
	ctx := context.Background()

	var slots pipelineSlots
	if err := slots.acquireGPU(ctx, gpuQ, ffmpegQ, pipelinePriority(2, 1)); err != nil {
		t.Fatalf("interpolate acquire failed: %v", err)
	}
	if err := slots.acquireFFmpeg(ctx, gpuQ, ffmpegQ, pipelinePriority(4, 1)); err != nil {
		t.Fatalf("optimize acquire failed: %v", err)
	}
	if slots.gpu {
		t.Error("still holding a GPU slot while running on the FFmpeg pool")
	}
	if !slots.ffmpeg {
		t.Error("not holding the FFmpeg slot it just acquired")
	}
	assertFree(t, gpuQ, "GPU pool after moving to an FFmpeg step")

	// And back again, for pipelines that encode before a further GPU step.
	if err := slots.acquireGPU(ctx, gpuQ, ffmpegQ, pipelinePriority(5, 1)); err != nil {
		t.Fatalf("second GPU acquire failed: %v", err)
	}
	if slots.ffmpeg {
		t.Error("still holding an FFmpeg slot while running on the GPU pool")
	}

	slots.release(gpuQ, ffmpegQ)
	assertFree(t, gpuQ, "GPU pool after release")
}

// TestPipelineSlots_ReleaseUnneededFreesPoolsTheFileIsDoneWith verifies the
// trailing cleanup steps run holding nothing, so the last encode does not keep a
// worker reserved while it deletes a file.
func TestPipelineSlots_ReleaseUnneededFreesPoolsTheFileIsDoneWith(t *testing.T) {
	gpuQ := queue.NewGPUQueue(1, 1)
	ffmpegQ := queue.New(1)
	ctx := context.Background()

	var slots pipelineSlots
	if err := slots.acquireFFmpeg(ctx, gpuQ, ffmpegQ, pipelinePriority(4, 1)); err != nil {
		t.Fatalf("optimize acquire failed: %v", err)
	}

	// Only a cleanup left after the optimize step.
	slots.releaseUnneeded(QueueNone, gpuQ, ffmpegQ)
	if slots.ffmpeg || slots.gpu {
		t.Fatalf("still holding a slot with no pool steps left: %+v", slots)
	}

	acquired := make(chan struct{})
	go func() {
		slot, err := ffmpegQ.Acquire(ctx, 0)
		if err != nil {
			return
		}
		close(acquired)
		ffmpegQ.Release(slot)
	}()
	select {
	case <-acquired:
	case <-time.After(2 * time.Second):
		t.Fatal("FFmpeg slot was not returned to the pool")
	}
}

// TestFirstQueue_DrivesSlotHandoverOnTheRealPipeline pins the look-ahead the
// step loop uses to decide whether to carry its slot into the next step. On the
// production pipeline it must keep the GPU across the cleanup that separates
// upscale from interpolate, and give it up right after the interpolate — before
// the next cleanup — because the file's remaining work is an FFmpeg encode.
func TestFirstQueue_DrivesSlotHandoverOnTheRealPipeline(t *testing.T) {
	cfg := testCfg(t)
	steps := realPipeline()

	for _, tc := range []struct {
		afterStep int
		want      QueueKind
		why       string
	}{
		{0, QueueGPU, "upscale done: interpolate is still ahead, keep the GPU across the cleanup"},
		{2, QueueFFmpeg, "interpolate done: only the encode is left, hand the GPU back now"},
		{4, QueueNone, "encode done: the trailing cleanup needs no worker at all"},
	} {
		if got := FirstQueue(cfg, steps, tc.afterStep+1); got != tc.want {
			t.Errorf("after step %d: FirstQueue=%v, want %v (%s)", tc.afterStep, got, tc.want, tc.why)
		}
	}
}

// assertFree fails unless the GPU pool has a slot available right now.
func assertFree(t *testing.T, q *queue.GPUQueue, what string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	gpuID, streamIdx, err := q.Acquire(ctx, 0)
	if err != nil {
		t.Fatalf("%s: no slot available (%v)", what, err)
	}
	q.Release(gpuID, streamIdx)
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
		"ep.mkv", 1, "input", 0, admitNext,
		func(logger.JobLog) {}, func(runner.Progress) {},
	)
	if ok {
		t.Fatal("expected RunCustomPipelineForFile to fail on cancelled context")
	}
	if got := atomic.LoadInt32(&admits); got != 1 {
		t.Fatalf("admitNext called %d times, want exactly 1", got)
	}
}

// TestRunCustomPipelineForFile_CleanupDeletesSelectedFolders verifies the
// cleanup step: it deletes the in-flight file from the selected stage folders,
// silently ignores folders where the file is absent, never touches unselected
// folders, and emits exactly one OK so progress accounting stays balanced
// (Completed+Failed+Skipped == Total).
func TestRunCustomPipelineForFile_CleanupDeletesSelectedFolders(t *testing.T) {
	base := t.TempDir()
	cfg := config.Config{
		BaseDir:         base,
		InputDir:        filepath.Join(base, "input"),
		OutputDir:       filepath.Join(base, "output"),
		OptimizedDir:    filepath.Join(base, "optimized"),
		InterpolatedDir: filepath.Join(base, "interpolated"),
		VideoExts:       []string{".mkv", ".mp4", ".avi"},
	}
	for _, d := range []string{cfg.InputDir, cfg.OutputDir, cfg.OptimizedDir, cfg.InterpolatedDir} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}

	const name = "ep.mkv"
	// File exists in input and output, but NOT in interpolated.
	inputFile := filepath.Join(cfg.InputDir, name)
	outputFile := filepath.Join(cfg.OutputDir, name)
	optimizedFile := filepath.Join(cfg.OptimizedDir, name)
	for _, f := range []string{inputFile, outputFile, optimizedFile} {
		if err := os.WriteFile(f, []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	steps := []pipeline.PipelineStep{
		// Select input + output (present) and interpolated (absent → ignored).
		{Operation: "cleanup", CleanupFolders: []string{"input", "output", "interpolated"}},
	}

	var oks int32
	onEvent := func(e logger.JobLog) {
		if e.Level == "OK" {
			atomic.AddInt32(&oks, 1)
		}
	}

	gpuQ := queue.NewGPUQueue(2, 1)
	ffmpegQ := queue.New(1)

	ok := RunCustomPipelineForFile(
		context.Background(), cfg, nil, gpuQ, ffmpegQ, steps,
		name, 1, "input", 0, func() {},
		onEvent, func(runner.Progress) {},
	)
	if !ok {
		t.Fatal("expected cleanup pipeline to succeed")
	}
	if got := atomic.LoadInt32(&oks); got != 1 {
		t.Fatalf("OK events = %d, want exactly 1 (progress accounting)", got)
	}

	// Selected + present folders are deleted.
	if _, err := os.Stat(inputFile); !os.IsNotExist(err) {
		t.Errorf("input file should have been deleted, stat err = %v", err)
	}
	if _, err := os.Stat(outputFile); !os.IsNotExist(err) {
		t.Errorf("output file should have been deleted, stat err = %v", err)
	}
	// Unselected folder is untouched.
	if _, err := os.Stat(optimizedFile); err != nil {
		t.Errorf("optimized file should remain, stat err = %v", err)
	}
}
