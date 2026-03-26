package process

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"anime-upscaling/internal/config"
	"anime-upscaling/internal/files"
	"anime-upscaling/internal/logger"
	"anime-upscaling/internal/runner"
)

// RunInterpolate processes all files using 2 GPU workers for frame interpolation.
func RunInterpolate(ctx context.Context, cfg config.Config, r *runner.Runner, fileList []string, multiplier int, rifeOpts runner.RifeOptions, onEvent func(logger.JobLog), onProgress func(runner.Progress)) error {
	type work struct {
		filename string
		index    int
	}
	fileCh := make(chan work, len(fileList))
	for i, f := range fileList {
		fileCh <- work{filename: f, index: i + 1}
	}
	close(fileCh)

	var wg sync.WaitGroup
	gpuCount := 2
	for gpuID := 0; gpuID < gpuCount; gpuID++ {
		wg.Add(1)
		go func(gpuID int) {
			defer wg.Done()
			for w := range fileCh {
				if ctx.Err() != nil {
					return
				}
				InterpolateFile(ctx, cfg, r, gpuID, w.filename, w.index, multiplier, rifeOpts, onEvent, safeProgress(onProgress))
			}
		}(gpuID)
	}
	wg.Wait()
	return nil
}

// InterpolateFile processes a single file on the given GPU using RIFE frame interpolation.
func InterpolateFile(ctx context.Context, cfg config.Config, r *runner.Runner, gpuID int, filename string, index int, multiplier int, rifeOpts runner.RifeOptions, onEvent func(logger.JobLog), onProgress func(runner.Progress)) bool {
	if err := os.MkdirAll(cfg.InterpolatedDir, 0755); err != nil {
		source := fmt.Sprintf("GPU %d", gpuID)
		onEvent(logger.JobLog{Source: source, Level: "ERRO", Index: index, Message: fmt.Sprintf("mkdir interpolated: %v", err), Time: time.Now()})
		return false
	}

	source := fmt.Sprintf("GPU %d", gpuID)
	gpuProgress := func(p runner.Progress) {
		p.Source = source
		onProgress(p)
	}

	outPath := filepath.Join(cfg.InterpolatedDir, filename)
	if files.FileExists(outPath) {
		onEvent(logger.JobLog{Source: source, Level: "SKIP", Index: index, Message: "Pulando " + filename + " (já existe)", Time: time.Now()})
		return true
	}

	// Auto-enable UHD mode for high-resolution input (>= 1440p)
	inputPath := filepath.Join(cfg.InputDir, filename)
	if res, err := r.ProbeResolution(ctx, inputPath); err == nil && res.Height >= 1440 {
		rifeOpts.UHD = true
	}

	onEvent(logger.JobLog{Source: source, Level: "INFO", Index: index, Message: "Interpolando: " + filename, Time: time.Now()})

	logFile := fmt.Sprintf("%s/gpu%d.log", cfg.BaseDir, gpuID)
	err := r.Video2xRife(ctx, gpuID, filename, logFile, multiplier, rifeOpts, gpuProgress)

	if err != nil {
		// Clean up partial output on failure
		os.Remove(outPath)
		onEvent(logger.JobLog{Source: source, Level: "ERRO", Index: index, Message: fmt.Sprintf("Falha ao interpolar: %s (%v)", filename, err), Time: time.Now()})
		return false
	}

	if !files.FileExists(outPath) {
		onEvent(logger.JobLog{Source: source, Level: "ERRO", Index: index, Message: "video2x retornou 0 mas output não existe: " + filename, Time: time.Now()})
		return false
	}

	r.Chown(ctx, cfg.InterpolatedDir, filename)
	onEvent(logger.JobLog{Source: source, Level: "OK", Index: index, Message: "Concluído: " + filename, Time: time.Now()})
	return true
}
