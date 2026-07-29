package process

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"anime-upscaling/internal/config"
	"anime-upscaling/internal/files"
	"anime-upscaling/internal/logger"
	"anime-upscaling/internal/pipeline"
	"anime-upscaling/internal/queue"
	"anime-upscaling/internal/runner"
)

// pipelineStepWeight ensures that step priority always dominates episode
// index in the composite priority. Assumes a single job has < 1M episodes,
// which is comfortably above any realistic batch.
const pipelineStepWeight = 1_000_000

// pipelinePriority composes step index and episode index into a single GPU
// queue priority so that:
//   - episodes further along in the pipeline always win over episodes still
//     on earlier steps (finish what's already started before opening new fronts);
//   - within the same step, the lower-indexed episode (earlier in the
//     natural-sorted file list) wins the tiebreak.
//
// Note: this is global across all custom-pipeline jobs sharing the GPU queue.
// A new job's step-0 acquires lose to any older job's later steps — intentional.
func pipelinePriority(stepIdx, index int) int {
	return stepIdx*pipelineStepWeight - index
}

// pipelineSlots tracks the worker-pool slot a file currently holds, so the step
// loop can carry it across a step boundary instead of dropping it and queueing
// up again from scratch.
//
// Dropping it is a priority inversion. Between the Release at the end of one
// step and the Acquire at the start of the next — with the cleanup steps in
// between running on no slot at all — the file holds nothing and is absent from
// the waiter list, so Release hands the slot to whoever is parked even when the
// priority comparator ranks that file below the one just leaving. That is how an
// episode that had finished its upscale and only owed an interpolate kept losing
// a GPU to a freshly started episode. Holding through the cleanups and swapping
// via Yield closes the window; a file only gives up its slot to a pool it is
// actually leaving, or when it is done.
type pipelineSlots struct {
	gpu        bool
	gpuID      int
	streamIdx  int
	ffmpeg     bool
	ffmpegSlot int
}

// acquireGPU puts the file on a GPU slot at the given priority, keeping the one
// it already holds unless a higher-priority waiter claims it. Any FFmpeg slot
// held is given up first — a file must never occupy a pool it is not using.
func (s *pipelineSlots) acquireGPU(ctx context.Context, gpuQ *queue.GPUQueue, ffmpegQ *queue.Queue, priority int) error {
	if s.ffmpeg {
		ffmpegQ.Release(s.ffmpegSlot)
		s.ffmpeg = false
	}
	if s.gpu {
		gpuID, streamIdx, err := gpuQ.Yield(ctx, s.gpuID, s.streamIdx, priority)
		if err != nil {
			s.gpu = false // Yield already released the slot.
			return err
		}
		s.gpuID, s.streamIdx = gpuID, streamIdx
		return nil
	}
	gpuID, streamIdx, err := gpuQ.Acquire(ctx, priority)
	if err != nil {
		return err
	}
	s.gpu, s.gpuID, s.streamIdx = true, gpuID, streamIdx
	return nil
}

// acquireFFmpeg is acquireGPU's counterpart for the FFmpeg pool.
func (s *pipelineSlots) acquireFFmpeg(ctx context.Context, gpuQ *queue.GPUQueue, ffmpegQ *queue.Queue, priority int) error {
	if s.gpu {
		gpuQ.Release(s.gpuID, s.streamIdx)
		s.gpu = false
	}
	if s.ffmpeg {
		slot, err := ffmpegQ.Yield(ctx, s.ffmpegSlot, priority)
		if err != nil {
			s.ffmpeg = false // Yield already released the slot.
			return err
		}
		s.ffmpegSlot = slot
		return nil
	}
	slot, err := ffmpegQ.Acquire(ctx, priority)
	if err != nil {
		return err
	}
	s.ffmpeg, s.ffmpegSlot = true, slot
	return nil
}

// releaseUnneeded gives back every pool other than next, which is the pool the
// file's remaining steps will contend for (QueueNone when none of them will).
func (s *pipelineSlots) releaseUnneeded(next QueueKind, gpuQ *queue.GPUQueue, ffmpegQ *queue.Queue) {
	if s.gpu && next != QueueGPU {
		gpuQ.Release(s.gpuID, s.streamIdx)
		s.gpu = false
	}
	if s.ffmpeg && next != QueueFFmpeg {
		ffmpegQ.Release(s.ffmpegSlot)
		s.ffmpeg = false
	}
}

// release gives back whatever the file still holds. Safe to call repeatedly.
func (s *pipelineSlots) release(gpuQ *queue.GPUQueue, ffmpegQ *queue.Queue) {
	if s.gpu {
		gpuQ.Release(s.gpuID, s.streamIdx)
		s.gpu = false
	}
	if s.ffmpeg {
		ffmpegQ.Release(s.ffmpegSlot)
		s.ffmpeg = false
	}
}

// RunCustomPipelineForFile executes pipeline steps sequentially for a single
// file, starting at startStep. It acquires/releases GPU and FFmpeg queue slots
// as needed per step. sourceDir is the directory startStep reads from; each
// step writes to its canonical output folder (output/, interpolated/,
// optimized/).
//
// startStep is non-zero for files resumed mid-pipeline — see PlanPipelineFiles.
func RunCustomPipelineForFile(
	ctx context.Context,
	cfg config.Config,
	r *runner.Runner,
	gpuQ *queue.GPUQueue,
	ffmpegQ *queue.Queue,
	steps []pipeline.PipelineStep,
	filename string,
	index int,
	sourceDir string,
	startStep int,
	admitNext func(),
	onEvent func(logger.JobLog),
	onProgress func(runner.Progress),
) bool {
	// admitNext releases the next file's admission gate. It must fire exactly
	// once — right after this file's first queue Acquire returns (so files enter
	// the GPU/FFmpeg queues in natural-sorted order) — and the deferred backstop
	// guarantees it also fires on early returns (ctx cancelled, Acquire error,
	// empty steps) so the relay never deadlocks.
	var admitOnce sync.Once
	admit := func() {
		if admitNext != nil {
			admitOnce.Do(admitNext)
		}
	}
	defer admit()

	// The slot is carried across steps, so exactly one place gives it back: this
	// backstop, on every exit path (success, step failure, cancellation).
	var slots pipelineSlots
	defer slots.release(gpuQ, ffmpegQ)

	if sourceDir == "" {
		sourceDir = cfg.InputDir
	}
	currentInputDir := sourceDir

	// Wrap onEvent so step-level ERRO is suppressed (handled by failRemaining
	// below for accurate accounting). OK and SKIP pass through so each step
	// increments Completed/Skipped exactly once.
	stepOnEvent := func(e logger.JobLog) {
		if e.Level == "ERRO" {
			e.Level = "STEP"
		}
		onEvent(e)
	}

	// failRemaining emits one ERRO with the real failure message and N-1
	// ERROs for the still-pending steps, so Failed accounts for every step
	// that won't run. Keeps Completed + Failed + Skipped == Total.
	failRemaining := func(stepIdx int, source, filename string) {
		remaining := len(steps) - stepIdx
		onEvent(logger.JobLog{Source: source, Level: "ERRO", Index: index, Message: "Falha: " + filename, Time: time.Now()})
		for i := 1; i < remaining; i++ {
			onEvent(logger.JobLog{Source: "PIPELINE", Level: "ERRO", Index: index, Message: "Step ignorado (pipeline falhou): " + filename, Time: time.Now()})
		}
	}

	if startStep < 0 {
		startStep = 0
	}
	for stepIdx := startStep; stepIdx < len(steps); stepIdx++ {
		step := steps[stepIdx]
		if ctx.Err() != nil {
			return false
		}

		stepNum := stepIdx + 1
		stepLabel := fmt.Sprintf("[%d/%d] ", stepNum, len(steps))

		switch step.Operation {
		case "upscale":
			scale := step.Scale
			if scale == 0 {
				scale = 2
			}
			upOpts := runner.UpscaleOptions{
				Processor:  step.Processor,
				Model:      step.Model,
				NoiseLevel: step.NoiseLevel,
			}

			if err := slots.acquireGPU(ctx, gpuQ, ffmpegQ, pipelinePriority(stepIdx, index)); err != nil {
				return false
			}
			admit()

			gpuSrc := runner.GPUSource(slots.gpuID, slots.streamIdx, cfg.StreamsPerGPU)
			onEvent(logger.JobLog{
				Source: gpuSrc, Level: "INFO", Index: index,
				Message: stepLabel + "Upscale " + fmt.Sprintf("%dx", scale) + ": " + filename,
				Time:    time.Now(),
			})

			ok := UpscaleFile(ctx, cfg, r, slots.gpuID, slots.streamIdx, filename, index, scale, upOpts, currentInputDir, cfg.OutputDir, stepOnEvent, onProgress)

			if !ok {
				failRemaining(stepIdx, "PIPELINE", filename)
				return false
			}
			currentInputDir = cfg.OutputDir

		case "interpolate":
			multiplier := step.Multiplier
			if multiplier == 0 {
				multiplier = 2
			}
			rifeModel := step.RifeModel
			if rifeModel == "" {
				rifeModel = "rife-v4.6"
			}
			sceneThresh := step.SceneThresh
			if sceneThresh == 0 {
				sceneThresh = 10.0
			}

			rifeOpts := runner.RifeOptions{
				Model:       rifeModel,
				SceneThresh: sceneThresh,
			}

			if err := slots.acquireGPU(ctx, gpuQ, ffmpegQ, pipelinePriority(stepIdx, index)); err != nil {
				return false
			}
			admit()

			gpuSrc := runner.GPUSource(slots.gpuID, slots.streamIdx, cfg.StreamsPerGPU)
			onEvent(logger.JobLog{
				Source: gpuSrc, Level: "INFO", Index: index,
				Message: stepLabel + "Interpolate " + fmt.Sprintf("%dx", multiplier) + ": " + filename,
				Time:    time.Now(),
			})

			ok := InterpolateFile(ctx, cfg, r, slots.gpuID, slots.streamIdx, filename, index, multiplier, rifeOpts, currentInputDir, cfg.InterpolatedDir, stepOnEvent, onProgress)

			if !ok {
				failRemaining(stepIdx, "PIPELINE", filename)
				return false
			}
			currentInputDir = cfg.InterpolatedDir

		case "optimize":
			quality := step.Quality
			if quality == "" {
				quality = "alta"
			}
			crf := pipeline.QualityToCRF[quality]
			if crf == 0 {
				crf = 19
			}
			resolution := step.Resolution
			if resolution == 0 {
				resolution = 1
			}
			frameRate := step.FrameRate
			if frameRate == 0 {
				frameRate = 1
			}
			frameRateAbsolute := 0.0
			if step.FrameRateMode == "absolute" && step.FrameRateAbsolute > 0 {
				frameRateAbsolute = step.FrameRateAbsolute
			}
			threads := step.Threads
			encOpts := runner.EncodeOptions{
				Codec:      step.Codec,
				Preset:     step.Preset,
				Tune:       step.Tune,
				PixFmt:     step.PixFmt,
				AudioCodec: step.AudioCodec,
				GPUVendor:  cfg.GPUVendor,
			}

			// Convert currentInputDir to relative source name for optimize
			source := dirToSource(cfg, currentInputDir)

			useGPU := optimizeUsesGPU(cfg, step)
			var optimizeOk bool

			if useGPU {
				if err := slots.acquireGPU(ctx, gpuQ, ffmpegQ, pipelinePriority(stepIdx, index)); err != nil {
					return false
				}
				admit()
				stepOpts := encOpts
				stepOpts.UseGPU = true
				stepOpts.GPUDevice = slots.gpuID
				src := runner.GPUSource(slots.gpuID, slots.streamIdx, cfg.StreamsPerGPU)
				onEvent(logger.JobLog{
					Source: src, Level: "INFO", Index: index,
					Message: stepLabel + "Optimize GPU (" + quality + "): " + filename,
					Time:    time.Now(),
				})
				optimizeOk = OptimizeFile(ctx, cfg, r, filename, index, source, src, resolution, frameRate, frameRateAbsolute, crf, threads, stepOpts, stepOnEvent, onProgress)
			} else {
				if err := slots.acquireFFmpeg(ctx, gpuQ, ffmpegQ, pipelinePriority(stepIdx, index)); err != nil {
					return false
				}
				admit()
				ffSrc := runner.FFmpegSource(slots.ffmpegSlot, cfg.FFmpegStreams)
				onEvent(logger.JobLog{
					Source: ffSrc, Level: "INFO", Index: index,
					Message: stepLabel + "Optimize (" + quality + "): " + filename,
					Time:    time.Now(),
				})
				optimizeOk = OptimizeFile(ctx, cfg, r, filename, index, source, ffSrc, resolution, frameRate, frameRateAbsolute, crf, threads, encOpts, stepOnEvent, onProgress)
			}

			if !optimizeOk {
				failRemaining(stepIdx, "PIPELINE", filename)
				return false
			}
			currentInputDir = cfg.OptimizedDir

		case "cleanup":
			// Delete the file currently being processed from the selected stage
			// folders. Best-effort: missing files are skipped, real errors are
			// logged but never fail the pipeline (the processing already succeeded).
			onEvent(logger.JobLog{
				Source: "PIPELINE", Level: "INFO", Index: index,
				Message: stepLabel + "Limpeza (" + strings.Join(step.CleanupFolders, ", ") + "): " + filename,
				Time:    time.Now(),
			})

			dir := filepath.Dir(filename)
			if dir == "." {
				dir = ""
			}
			name := filepath.Base(filename)

			folderDirs := map[string]string{
				"input":        cfg.InputDir,
				"output":       cfg.OutputDir,
				"interpolated": cfg.InterpolatedDir,
				"optimized":    cfg.OptimizedDir,
			}
			// Only attempt folders where the file actually exists, so stages this
			// file never passed through don't produce noisy "no such file" errors.
			var present []string
			for _, folder := range step.CleanupFolders {
				d, ok := folderDirs[folder]
				if ok && files.FileExists(filepath.Join(d, dir, name)) {
					present = append(present, folder)
				}
			}

			if len(present) > 0 {
				_, errs := files.DeleteFiles(
					[]files.DeleteItem{{Name: name, Path: dir, Folders: present}},
					cfg.InputDir, cfg.OutputDir, cfg.OptimizedDir, cfg.InterpolatedDir, cfg.VideoExts,
				)
				for _, e := range errs {
					onEvent(logger.JobLog{Source: "PIPELINE", Level: "STEP", Index: index, Message: "Limpeza: " + e, Time: time.Now()})
				}
			}

			// Emit one OK so progress accounting stays correct
			// (Completed+Failed+Skipped == Total). currentInputDir is left
			// unchanged — cleanup produces no new file for the next step.
			onEvent(logger.JobLog{Source: "PIPELINE", Level: "OK", Index: index, Message: stepLabel + "Limpeza concluída: " + filename, Time: time.Now()})
		}

		// Hand back a pool the remaining steps will not use, the moment that
		// becomes known. Carrying a slot is only worth it while the file is still
		// going to run on that pool; a trailing cleanup must not sit on a GPU that
		// another episode could be using.
		slots.releaseUnneeded(FirstQueue(cfg, steps, stepIdx+1), gpuQ, ffmpegQ)
	}

	// Each step's OK already incremented Completed; emit a STEP-level event
	// here just so the log shows pipeline completion without double-counting.
	onEvent(logger.JobLog{Source: "PIPELINE", Level: "STEP", Index: index, Message: "Concluído: " + filename, Time: time.Now()})
	return true
}

// dirToSource converts an absolute directory path back to the source name used by OptimizeFile.
func dirToSource(cfg config.Config, dir string) string {
	// OptimizeFile uses relative source like "input", "output", "interpolated", "optimized"
	absDir := filepath.Clean(dir)
	for _, pair := range []struct {
		dir  string
		name string
	}{
		{cfg.InputDir, "input"},
		{cfg.OutputDir, "output"},
		{cfg.InterpolatedDir, "interpolated"},
		{cfg.OptimizedDir, "optimized"},
	} {
		if filepath.Clean(pair.dir) == absDir || strings.HasSuffix(absDir, "/"+pair.name) {
			return pair.name
		}
	}
	return "input"
}
