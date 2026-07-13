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

// RunInterpolate processes all files using cfg.GPUCount*cfg.StreamsPerGPU workers.
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
					InterpolateFile(ctx, cfg, r, gpuID, streamIdx, w.filename, w.index, multiplier, rifeOpts, cfg.InputDir, cfg.InterpolatedDir, onEvent, safeProgress(onProgress))
				}
			}(gpuID, streamIdx)
		}
	}
	wg.Wait()
	return nil
}

// InterpolateFile processes a single file on the given GPU stream using RIFE frame interpolation.
func InterpolateFile(ctx context.Context, cfg config.Config, r *runner.Runner, gpuID, streamIdx int, filename string, index int, multiplier int, rifeOpts runner.RifeOptions, inputDir, outputDir string, onEvent func(logger.JobLog), onProgress func(runner.Progress)) bool {
	tempOutputDir := cfg.TempDir + "/interpolated"
	source := runner.GPUSource(gpuID, streamIdx, cfg.StreamsPerGPU)
	subDir := filepath.Dir(filename)
	for _, dir := range []string{outputDir, tempOutputDir} {
		target := dir
		if subDir != "." && subDir != "" {
			target = filepath.Join(dir, subDir)
		}
		if err := os.MkdirAll(target, 0755); err != nil {
			onEvent(logger.JobLog{Source: source, Level: "ERRO", Index: index, Message: fmt.Sprintf("mkdir interpolated: %v", err), Time: time.Now()})
			return false
		}
	}

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

	// Auto-enable UHD mode for high-resolution input (>= 1440p)
	probePath := filepath.Join(inputDir, filename)
	if res, err := r.ProbeResolution(ctx, probePath); err == nil && res.Height >= 1440 {
		rifeOpts.UHD = true
	}

	// Convert anamorphic/interlaced sources to square pixels before video2x so
	// the output keeps the true aspect (no side bars). No-op when interpolating
	// after upscale, since that input is already square. Cleanup is deferred so
	// it also fires across the salvage/retry path below.
	effInputDir, normCleanup, err := normalizeForVideo2x(ctx, cfg, r, filename, inputDir, source, index, onEvent)
	defer normCleanup()
	if err != nil {
		onEvent(logger.JobLog{Source: source, Level: "ERRO", Index: index, Message: fmt.Sprintf("Falha ao normalizar: %s (%v)", filename, err), Time: time.Now()})
		return false
	}

	onEvent(logger.JobLog{Source: source, Level: "INFO", Index: index, Message: "Interpolando: " + filename, Time: time.Now()})

	logFile := gpuLogPath(cfg, gpuID, streamIdx)
	err = r.Video2xRife(ctx, gpuID, streamIdx, filename, logFile, multiplier, rifeOpts, effInputDir, tempOutputDir, gpuProgress)
	tempOutPath := filepath.Join(tempOutputDir, filename)

	// See upscale.go: glslang PoolAlloc aborts can fire after the output is
	// fully written (salvage) or during shader compilation (retry once).
	if err != nil && ctx.Err() == nil {
		if salvageSignaledRun(err, logFile, tempOutPath) {
			onEvent(logger.JobLog{Source: source, Level: "INFO", Index: index, Message: fmt.Sprintf("Recuperando %s: video2x morreu em signal mas output foi escrito por completo", filename), Time: time.Now()})
			err = nil
		} else if sig, signaled := runner.SignalFromError(err); signaled {
			onEvent(logger.JobLog{Source: source, Level: "INFO", Index: index, Message: fmt.Sprintf("Tentativa 1 morreu com signal %s; repetindo interpolação: %s", sig, filename), Time: time.Now()})
			_ = os.Remove(tempOutPath)
			err = r.Video2xRife(ctx, gpuID, streamIdx, filename, logFile, multiplier, rifeOpts, effInputDir, tempOutputDir, gpuProgress)
			if err != nil && salvageSignaledRun(err, logFile, tempOutPath) {
				onEvent(logger.JobLog{Source: source, Level: "INFO", Index: index, Message: fmt.Sprintf("Recuperando %s na 2ª tentativa: video2x morreu em signal mas output foi escrito por completo", filename), Time: time.Now()})
				err = nil
			}
		}
	}

	if err != nil {
		_ = os.Remove(tempOutPath)
		onEvent(logger.JobLog{Source: source, Level: "ERRO", Index: index, Message: fmt.Sprintf("Falha ao interpolar: %s (%v)", filename, err), Time: time.Now()})
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
