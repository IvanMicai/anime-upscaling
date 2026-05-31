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

// RunUpscale processes all files using cfg.GPUCount*cfg.StreamsPerGPU workers (CLI convenience wrapper).
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
	gpuCount := cfg.GPUCount
	streams := cfg.StreamsPerGPU
	for gpuID := 0; gpuID < gpuCount; gpuID++ {
		for streamIdx := 0; streamIdx < streams; streamIdx++ {
			wg.Add(1)
			go func(gpuID, streamIdx int) {
				defer wg.Done()
				for w := range fileCh {
					if ctx.Err() != nil {
						return
					}
					UpscaleFile(ctx, cfg, r, gpuID, streamIdx, w.filename, w.index, scale, runner.UpscaleOptions{}, cfg.InputDir, cfg.OutputDir, onEvent, safeProgress(onProgress))
				}
			}(gpuID, streamIdx)
		}
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

// UpscaleFile processes a single file on the given GPU stream.
// Returns true if the file was successfully upscaled (or skipped).
func UpscaleFile(ctx context.Context, cfg config.Config, r *runner.Runner, gpuID, streamIdx int, filename string, index int, scale int, opts runner.UpscaleOptions, inputDir, outputDir string, onEvent func(logger.JobLog), onProgress func(runner.Progress)) bool {
	tempOutputDir := cfg.TempDir + "/output"
	subDir := filepath.Dir(filename)
	for _, dir := range []string{outputDir, tempOutputDir} {
		target := dir
		if subDir != "." && subDir != "" {
			target = filepath.Join(dir, subDir)
		}
		if err := os.MkdirAll(target, 0755); err != nil {
			source := runner.GPUSource(gpuID, streamIdx, cfg.StreamsPerGPU)
			onEvent(logger.JobLog{Source: source, Level: "ERRO", Index: index, Message: fmt.Sprintf("mkdir output: %v", err), Time: time.Now()})
			return false
		}
	}

	source := runner.GPUSource(gpuID, streamIdx, cfg.StreamsPerGPU)
	gpuProgress := func(p runner.Progress) {
		p.Source = source
		p.Filename = filename
		onProgress(p)
	}

	outPath := filepath.Join(outputDir, filename)
	if files.FileExists(outPath) {
		onEvent(logger.JobLog{Source: source, Level: "SKIP", Index: index, Message: "Pulando " + filename + " (já existe)", Time: time.Now()})
		return true
	}

	onEvent(logger.JobLog{Source: source, Level: "INFO", Index: index, Message: "Iniciando: " + filename, Time: time.Now()})

	logFile := gpuLogPath(cfg, gpuID, streamIdx)
	err := r.Video2x(ctx, gpuID, streamIdx, filename, logFile, scale, opts, inputDir, tempOutputDir, gpuProgress)
	tempOutPath := filepath.Join(tempOutputDir, filename)

	// glslang's PoolAlloc assertion (an upstream video2x/ncnn-vulkan bug) can
	// abort the process during Vulkan teardown after the output is already
	// written, or kill it mid-run during shader compilation. Salvage the
	// first case; retry once for the second.
	if err != nil && ctx.Err() == nil {
		if salvageSignaledRun(err, logFile, tempOutPath) {
			onEvent(logger.JobLog{Source: source, Level: "INFO", Index: index, Message: fmt.Sprintf("Recuperando %s: video2x morreu em signal mas output foi escrito por completo", filename), Time: time.Now()})
			err = nil
		} else if sig, signaled := runner.SignalFromError(err); signaled {
			onEvent(logger.JobLog{Source: source, Level: "INFO", Index: index, Message: fmt.Sprintf("Tentativa 1 morreu com signal %s; repetindo upscale: %s", sig, filename), Time: time.Now()})
			_ = os.Remove(tempOutPath)
			err = r.Video2x(ctx, gpuID, streamIdx, filename, logFile, scale, opts, inputDir, tempOutputDir, gpuProgress)
			if err != nil && salvageSignaledRun(err, logFile, tempOutPath) {
				onEvent(logger.JobLog{Source: source, Level: "INFO", Index: index, Message: fmt.Sprintf("Recuperando %s na 2ª tentativa: video2x morreu em signal mas output foi escrito por completo", filename), Time: time.Now()})
				err = nil
			}
		}
	}

	if err != nil {
		_ = os.Remove(tempOutPath)
		onEvent(logger.JobLog{Source: source, Level: "ERRO", Index: index, Message: fmt.Sprintf("Falha ao processar: %s (%v)", filename, err), Time: time.Now()})
		return false
	}

	if !files.FileExists(tempOutPath) {
		onEvent(logger.JobLog{Source: source, Level: "ERRO", Index: index, Message: "video2x retornou 0 mas output não existe: " + filename, Time: time.Now()})
		return false
	}

	if err := os.Rename(tempOutPath, outPath); err != nil {
		_ = os.Remove(tempOutPath)
		onEvent(logger.JobLog{Source: source, Level: "ERRO", Index: index, Message: fmt.Sprintf("Falha ao mover output: %s (%v)", filename, err), Time: time.Now()})
		return false
	}

	_ = r.Chown(ctx, outputDir, filename)
	onEvent(logger.JobLog{Source: source, Level: "OK", Index: index, Message: "Concluído: " + filename, Time: time.Now()})
	return true
}

// gpuLogPath returns the per-GPU log file path. Keeps the legacy "gpu%d.log"
// filename when streamsPerGPU<=1 so external log tailers keep working; adds a
// stream suffix only when multiple concurrent streams share a GPU.
func gpuLogPath(cfg config.Config, gpuID, streamIdx int) string {
	if cfg.StreamsPerGPU <= 1 {
		return fmt.Sprintf("%s/gpu%d.log", cfg.BaseDir, gpuID)
	}
	return fmt.Sprintf("%s/gpu%d-s%d.log", cfg.BaseDir, gpuID, streamIdx)
}
