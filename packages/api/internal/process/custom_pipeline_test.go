package process

import (
	"context"
	"os"
	"path/filepath"
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
		name, 1, "input", func() {},
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
