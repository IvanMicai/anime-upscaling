package process

import (
	"context"
	"sync"

	"anime-upscaling/internal/config"
	"anime-upscaling/internal/logger"
	"anime-upscaling/internal/runner"
)

// RunPipeline runs the full upscale+encode pipeline with 2 GPU workers feeding 1 FFmpeg worker (CLI convenience wrapper).
func RunPipeline(ctx context.Context, cfg config.Config, r *runner.Runner, fileList []string, scale int, onEvent func(logger.JobLog), onProgress func(runner.Progress)) error {
	type work struct {
		filename string
		index    int
	}
	fileCh := make(chan work, len(fileList))
	for i, f := range fileList {
		fileCh <- work{filename: f, index: i + 1}
	}
	close(fileCh)

	readyCh := make(chan work, len(fileList))

	var gpuWg sync.WaitGroup
	gpuCount := cfg.GPUCount
	streams := cfg.StreamsPerGPU
	for gpuID := 0; gpuID < gpuCount; gpuID++ {
		for streamIdx := 0; streamIdx < streams; streamIdx++ {
			gpuWg.Add(1)
			go func(gpuID, streamIdx int) {
				defer gpuWg.Done()
				for w := range fileCh {
					if ctx.Err() != nil {
						return
					}
					ok := UpscaleFile(ctx, cfg, r, gpuID, streamIdx, w.filename, w.index, scale, runner.UpscaleOptions{}, cfg.InputDir, cfg.OutputDir, onEvent, safeProgress(onProgress))
					if ok {
						readyCh <- w
					}
				}
			}(gpuID, streamIdx)
		}
	}

	go func() {
		gpuWg.Wait()
		close(readyCh)
	}()

	for w := range readyCh {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		OptimizeFile(ctx, cfg, r, w.filename, w.index, "output", "FFMPEG", 1, 22, 0, runner.EncodeOptions{}, onEvent, safeProgress(onProgress))
	}
	return nil
}

// EncodeFile compresses a single file from output/ to optimized/ using FFmpeg (pipeline phase 2).
func EncodeFile(ctx context.Context, cfg config.Config, r *runner.Runner, filename string, threads int, onEvent func(logger.JobLog), onProgress func(runner.Progress)) bool {
	return OptimizeFile(ctx, cfg, r, filename, 0, "output", "FFMPEG", 1, 22, threads, runner.EncodeOptions{}, onEvent, onProgress)
}
