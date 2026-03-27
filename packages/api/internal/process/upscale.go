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

// RunUpscale processes all files using 2 GPU workers (CLI convenience wrapper).
func RunUpscale(ctx context.Context, cfg config.Config, r *runner.Runner, fileList []string, scale int, onEvent func(logger.JobLog), onProgress func(runner.Progress)) error {
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
				UpscaleFile(ctx, cfg, r, gpuID, w.filename, w.index, scale, runner.UpscaleOptions{}, cfg.InputDir, cfg.OutputDir, cfg.InputDir, onEvent, safeProgress(onProgress))
			}
		}(gpuID)
	}
	wg.Wait()
	return nil
}

func safeProgress(fn func(runner.Progress)) func(runner.Progress) {
	if fn == nil {
		return func(runner.Progress) {}
	}
	return fn
}

// UpscaleFile processes a single file on the given GPU.
// Returns true if the file was successfully upscaled (or skipped).
func UpscaleFile(ctx context.Context, cfg config.Config, r *runner.Runner, gpuID int, filename string, index int, scale int, opts runner.UpscaleOptions, inputDir, outputDir string, originalInputDir string, onEvent func(logger.JobLog), onProgress func(runner.Progress)) bool {
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		source := fmt.Sprintf("GPU %d", gpuID)
		onEvent(logger.JobLog{Source: source, Level: "ERRO", Index: index, Message: fmt.Sprintf("mkdir output: %v", err), Time: time.Now()})
		return false
	}

	source := fmt.Sprintf("GPU %d", gpuID)
	gpuProgress := func(p runner.Progress) {
		p.Source = source
		onProgress(p)
	}

	outPath := filepath.Join(outputDir, filename)
	if files.FileExists(outPath) {
		onEvent(logger.JobLog{Source: source, Level: "SKIP", Index: index, Message: "Pulando " + filename + " (já existe)", Time: time.Now()})
		return true
	}

	onEvent(logger.JobLog{Source: source, Level: "INFO", Index: index, Message: "Iniciando: " + filename, Time: time.Now()})

	logFile := fmt.Sprintf("%s/gpu%d.log", cfg.BaseDir, gpuID)
	err := r.Video2x(ctx, gpuID, filename, logFile, scale, opts, inputDir, outputDir, gpuProgress)

	if err != nil {
		// Clean up partial output on failure
		os.Remove(outPath)
		onEvent(logger.JobLog{Source: source, Level: "ERRO", Index: index, Message: fmt.Sprintf("Falha ao processar: %s (%v)", filename, err), Time: time.Now()})
		return false
	}

	if !files.FileExists(outPath) {
		onEvent(logger.JobLog{Source: source, Level: "ERRO", Index: index, Message: "video2x retornou 0 mas output não existe: " + filename, Time: time.Now()})
		return false
	}

	// Remux audio+subtitles from the original input file onto the processed output
	originalPath := filepath.Join(originalInputDir, filename)
	if files.FileExists(originalPath) {
		if err := r.FFmpegRemuxAudio(ctx, outPath, originalPath); err != nil {
			onEvent(logger.JobLog{Source: source, Level: "WARN", Index: index, Message: fmt.Sprintf("Remux áudio falhou: %s (%v)", filename, err), Time: time.Now()})
		}
	}

	r.Chown(ctx, outputDir, filename)
	onEvent(logger.JobLog{Source: source, Level: "OK", Index: index, Message: "Concluído: " + filename, Time: time.Now()})
	return true
}
