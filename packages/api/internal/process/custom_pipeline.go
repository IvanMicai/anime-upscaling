package process

import (
	"context"
	"fmt"
	"io"
	"os"
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
// sourceDir is the directory the first step reads from. outputDir, if non-empty,
// is where the final file is moved after the last step (if different from the last
// step's canonical output).
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
	outputDir string,
	onEvent func(logger.JobLog),
	onProgress func(runner.Progress),
) bool {
	if sourceDir == "" {
		sourceDir = cfg.InputDir
	}
	currentInputDir := sourceDir

	// Wrap onEvent so that step-level OK/SKIP/ERRO don't increment job counters.
	// They become "STEP" level which cleans up containers but doesn't affect progress.
	stepOnEvent := func(e logger.JobLog) {
		switch e.Level {
		case "OK", "SKIP", "ERRO":
			e.Level = "STEP"
		}
		onEvent(e)
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

			gpuID, streamIdx, err := gpuQ.Acquire(ctx, stepIdx)
			if err != nil {
				return false
			}

			gpuSrc := runner.GPUSource(gpuID, streamIdx, cfg.StreamsPerGPU)
			onEvent(logger.JobLog{
				Source: gpuSrc, Level: "INFO", Index: index,
				Message: stepLabel + "Upscale " + fmt.Sprintf("%dx", scale) + ": " + filename,
				Time:    time.Now(),
			})

			ok := UpscaleFile(ctx, cfg, r, gpuID, streamIdx, filename, index, scale, upOpts, currentInputDir, cfg.OutputDir, stepOnEvent, onProgress)
			gpuQ.Release(gpuID, streamIdx)

			if !ok {
				onEvent(logger.JobLog{Source: "PIPELINE", Level: "ERRO", Index: index, Message: "Falha: " + filename, Time: time.Now()})
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

			gpuID, streamIdx, err := gpuQ.Acquire(ctx, stepIdx)
			if err != nil {
				return false
			}

			gpuSrc := runner.GPUSource(gpuID, streamIdx, cfg.StreamsPerGPU)
			onEvent(logger.JobLog{
				Source: gpuSrc, Level: "INFO", Index: index,
				Message: stepLabel + "Interpolate " + fmt.Sprintf("%dx", multiplier) + ": " + filename,
				Time:    time.Now(),
			})

			ok := InterpolateFile(ctx, cfg, r, gpuID, streamIdx, filename, index, multiplier, rifeOpts, currentInputDir, cfg.InterpolatedDir, stepOnEvent, onProgress)
			gpuQ.Release(gpuID, streamIdx)

			if !ok {
				onEvent(logger.JobLog{Source: "PIPELINE", Level: "ERRO", Index: index, Message: "Falha: " + filename, Time: time.Now()})
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
				GPUVendor:  cfg.GPUVendor,
			}

			// Convert currentInputDir to relative source name for optimize
			source := dirToSource(cfg, currentInputDir)

			useGPU := step.UseGPU && cfg.GPUVendor != "" && step.Codec != "copy" && step.Codec != "libvpx-vp9"
			var optimizeOk bool

			if useGPU {
				gpuID, streamIdx, err := gpuQ.Acquire(ctx, stepIdx)
				if err != nil {
					return false
				}
				stepOpts := encOpts
				stepOpts.UseGPU = true
				stepOpts.GPUDevice = gpuID
				src := runner.GPUSource(gpuID, streamIdx, cfg.StreamsPerGPU)
				onEvent(logger.JobLog{
					Source: src, Level: "INFO", Index: index,
					Message: stepLabel + "Optimize GPU (" + quality + "): " + filename,
					Time:    time.Now(),
				})
				optimizeOk = OptimizeFile(ctx, cfg, r, filename, index, source, src, resolution, crf, threads, stepOpts, stepOnEvent, onProgress)
				gpuQ.Release(gpuID, streamIdx)
			} else {
				done := make(chan struct{})
				if err := ffmpegQ.Submit(ctx, func(slot int) {
					defer close(done)
					ffSrc := runner.FFmpegSource(slot, cfg.FFmpegStreams)
					onEvent(logger.JobLog{
						Source: ffSrc, Level: "INFO", Index: index,
						Message: stepLabel + "Optimize (" + quality + "): " + filename,
						Time:    time.Now(),
					})
					optimizeOk = OptimizeFile(ctx, cfg, r, filename, index, source, ffSrc, resolution, crf, threads, encOpts, stepOnEvent, onProgress)
				}); err != nil {
					return false
				}
				<-done
			}

			if !optimizeOk {
				onEvent(logger.JobLog{Source: "PIPELINE", Level: "ERRO", Index: index, Message: "Falha: " + filename, Time: time.Now()})
				return false
			}
			currentInputDir = cfg.OptimizedDir
		}
	}

	// If the caller requested a custom final output folder, move the resulting
	// file from the last step's canonical directory to outputDir.
	if outputDir != "" && filepath.Clean(outputDir) != filepath.Clean(currentInputDir) {
		if err := moveFinalOutput(currentInputDir, outputDir, filename); err != nil {
			onEvent(logger.JobLog{
				Source: "PIPELINE", Level: "ERRO", Index: index,
				Message: fmt.Sprintf("Falha ao mover para %s/: %s (%v)", dirToSource(cfg, outputDir), filename, err),
				Time:    time.Now(),
			})
			return false
		}
		onEvent(logger.JobLog{
			Source: "PIPELINE", Level: "INFO", Index: index,
			Message: fmt.Sprintf("Movido para %s/: %s", dirToSource(cfg, outputDir), filename),
			Time:    time.Now(),
		})
	}

	onEvent(logger.JobLog{Source: "PIPELINE", Level: "OK", Index: index, Message: "Concluído: " + filename, Time: time.Now()})
	return true
}

// moveFinalOutput moves a file from srcDir/name to dstDir/name. Tries rename first
// (atomic when on the same filesystem), falls back to copy+delete across devices.
func moveFinalOutput(srcDir, dstDir, name string) error {
	srcPath := filepath.Join(srcDir, name)
	dstPath := filepath.Join(dstDir, name)

	if err := os.MkdirAll(dstDir, 0755); err != nil {
		return fmt.Errorf("mkdir %s: %w", dstDir, err)
	}

	if err := os.Rename(srcPath, dstPath); err == nil {
		return nil
	}

	// Cross-device fallback: copy then delete.
	src, err := os.Open(srcPath)
	if err != nil {
		return fmt.Errorf("open src: %w", err)
	}
	defer src.Close()

	dst, err := os.OpenFile(dstPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0644)
	if err != nil {
		return fmt.Errorf("create dst: %w", err)
	}
	if _, err := io.Copy(dst, src); err != nil {
		dst.Close()
		os.Remove(dstPath)
		return fmt.Errorf("copy: %w", err)
	}
	if err := dst.Close(); err != nil {
		os.Remove(dstPath)
		return fmt.Errorf("close dst: %w", err)
	}
	if err := os.Remove(srcPath); err != nil {
		return fmt.Errorf("remove src: %w", err)
	}
	return nil
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
