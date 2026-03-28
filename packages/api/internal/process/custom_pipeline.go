package process

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"anime-upscaling/internal/config"
	"anime-upscaling/internal/logger"
	"anime-upscaling/internal/pipeline"
	"anime-upscaling/internal/queue"
	"anime-upscaling/internal/runner"
)

// RunCustomPipelineForFile executes all pipeline steps sequentially for a single file.
// It acquires/releases GPU and FFmpeg queue slots as needed per step.
func RunCustomPipelineForFile(
	ctx context.Context,
	cfg config.Config,
	r *runner.Runner,
	gpuQ *queue.GPUQueue,
	ffmpegQ *queue.Queue,
	steps []pipeline.PipelineStep,
	filename string,
	index int,
	onEvent func(logger.JobLog),
	onProgress func(runner.Progress),
) bool {
	currentInputDir := cfg.InputDir

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

			gpuID, err := gpuQ.Acquire(ctx, stepIdx)
			if err != nil {
				return false
			}

			onEvent(logger.JobLog{
				Source: fmt.Sprintf("GPU %d", gpuID), Level: "INFO", Index: index,
				Message: stepLabel + "Upscale " + fmt.Sprintf("%dx", scale) + ": " + filename,
				Time:    time.Now(),
			})

			ok := UpscaleFile(ctx, cfg, r, gpuID, filename, index, scale, upOpts, currentInputDir, cfg.OutputDir, onEvent, onProgress)
			gpuQ.Release(gpuID)

			if !ok {
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

			gpuID, err := gpuQ.Acquire(ctx, stepIdx)
			if err != nil {
				return false
			}

			onEvent(logger.JobLog{
				Source: fmt.Sprintf("GPU %d", gpuID), Level: "INFO", Index: index,
				Message: stepLabel + "Interpolate " + fmt.Sprintf("%dx", multiplier) + ": " + filename,
				Time:    time.Now(),
			})

			ok := InterpolateFile(ctx, cfg, r, gpuID, filename, index, multiplier, rifeOpts, currentInputDir, cfg.InterpolatedDir, onEvent, onProgress)
			gpuQ.Release(gpuID)

			if !ok {
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
			threads := step.Threads
			encOpts := runner.EncodeOptions{
				Codec:      step.Codec,
				Preset:     step.Preset,
				Tune:       step.Tune,
				PixFmt:     step.PixFmt,
				AudioCodec: step.AudioCodec,
			}

			// Convert currentInputDir to relative source name for optimize
			source := dirToSource(cfg, currentInputDir)

			done := make(chan struct{})
			if err := ffmpegQ.Submit(ctx, func() {
				defer close(done)
				onEvent(logger.JobLog{
					Source: "FFMPEG", Level: "INFO", Index: index,
					Message: stepLabel + "Optimize (" + quality + "): " + filename,
					Time:    time.Now(),
				})
				OptimizeFile(ctx, cfg, r, filename, index, source, resolution, crf, threads, encOpts, onEvent, onProgress)
			}); err != nil {
				return false
			}
			<-done

			currentInputDir = cfg.OptimizedDir
		}
	}

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
