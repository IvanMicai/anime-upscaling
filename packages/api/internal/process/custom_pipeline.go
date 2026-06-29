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

// RunCustomPipelineForFile executes all pipeline steps sequentially for a single file.
// It acquires/releases GPU and FFmpeg queue slots as needed per step.
// sourceDir is the directory the first step reads from; each step writes to its
// canonical output folder (output/, interpolated/, optimized/).
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

	for stepIdx, step := range steps {
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

			gpuID, streamIdx, err := gpuQ.Acquire(ctx, pipelinePriority(stepIdx, index))
			if err != nil {
				return false
			}
			admit()

			gpuSrc := runner.GPUSource(gpuID, streamIdx, cfg.StreamsPerGPU)
			onEvent(logger.JobLog{
				Source: gpuSrc, Level: "INFO", Index: index,
				Message: stepLabel + "Upscale " + fmt.Sprintf("%dx", scale) + ": " + filename,
				Time:    time.Now(),
			})

			ok := UpscaleFile(ctx, cfg, r, gpuID, streamIdx, filename, index, scale, upOpts, currentInputDir, cfg.OutputDir, stepOnEvent, onProgress)
			gpuQ.Release(gpuID, streamIdx)

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

			gpuID, streamIdx, err := gpuQ.Acquire(ctx, pipelinePriority(stepIdx, index))
			if err != nil {
				return false
			}
			admit()

			gpuSrc := runner.GPUSource(gpuID, streamIdx, cfg.StreamsPerGPU)
			onEvent(logger.JobLog{
				Source: gpuSrc, Level: "INFO", Index: index,
				Message: stepLabel + "Interpolate " + fmt.Sprintf("%dx", multiplier) + ": " + filename,
				Time:    time.Now(),
			})

			ok := InterpolateFile(ctx, cfg, r, gpuID, streamIdx, filename, index, multiplier, rifeOpts, currentInputDir, cfg.InterpolatedDir, stepOnEvent, onProgress)
			gpuQ.Release(gpuID, streamIdx)

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

			useGPU := step.UseGPU && cfg.GPUVendor != "" && step.Codec != "copy" && step.Codec != "libvpx-vp9"
			var optimizeOk bool

			if useGPU {
				gpuID, streamIdx, err := gpuQ.Acquire(ctx, pipelinePriority(stepIdx, index))
				if err != nil {
					return false
				}
				admit()
				stepOpts := encOpts
				stepOpts.UseGPU = true
				stepOpts.GPUDevice = gpuID
				src := runner.GPUSource(gpuID, streamIdx, cfg.StreamsPerGPU)
				onEvent(logger.JobLog{
					Source: src, Level: "INFO", Index: index,
					Message: stepLabel + "Optimize GPU (" + quality + "): " + filename,
					Time:    time.Now(),
				})
				optimizeOk = OptimizeFile(ctx, cfg, r, filename, index, source, src, resolution, frameRate, frameRateAbsolute, crf, threads, stepOpts, stepOnEvent, onProgress)
				gpuQ.Release(gpuID, streamIdx)
			} else {
				slot, err := ffmpegQ.Acquire(ctx, pipelinePriority(stepIdx, index))
				if err != nil {
					return false
				}
				admit()
				ffSrc := runner.FFmpegSource(slot, cfg.FFmpegStreams)
				onEvent(logger.JobLog{
					Source: ffSrc, Level: "INFO", Index: index,
					Message: stepLabel + "Optimize (" + quality + "): " + filename,
					Time:    time.Now(),
				})
				optimizeOk = OptimizeFile(ctx, cfg, r, filename, index, source, ffSrc, resolution, frameRate, frameRateAbsolute, crf, threads, encOpts, stepOnEvent, onProgress)
				ffmpegQ.Release(slot)
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
