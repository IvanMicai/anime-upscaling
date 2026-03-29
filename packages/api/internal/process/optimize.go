package process

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"anime-upscaling/internal/config"
	"anime-upscaling/internal/files"
	"anime-upscaling/internal/logger"
	"anime-upscaling/internal/runner"
)

// RunOptimize processes all files sequentially with FFmpeg (CLI convenience wrapper).
func RunOptimize(ctx context.Context, cfg config.Config, r *runner.Runner, fileList []string, onEvent func(logger.JobLog), onProgress func(runner.Progress)) error {
	for i, f := range fileList {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		OptimizeFile(ctx, cfg, r, f, i+1, "input", 1, 19, 0, runner.EncodeOptions{}, onEvent, safeProgress(onProgress))
	}
	return nil
}

// OptimizeFile compresses a single file from input/ to optimized/ using FFmpeg.
func OptimizeFile(ctx context.Context, cfg config.Config, r *runner.Runner, filename string, index int, source string, resolution int, crf int, threads int, opts runner.EncodeOptions, onEvent func(logger.JobLog), onProgress func(runner.Progress)) {
	if err := os.MkdirAll(cfg.OptimizedDir, 0755); err != nil {
		onEvent(logger.JobLog{Source: "FFMPEG", Level: "ERRO", Index: index, Message: fmt.Sprintf("mkdir optimized: %v", err), Time: time.Now()})
		return
	}

	outPath := filepath.Join(cfg.OptimizedDir, filename)
	if files.FileExists(outPath) {
		onEvent(logger.JobLog{Source: "FFMPEG", Level: "SKIP", Index: index, Message: "Pulando " + filename + " (já existe)", Time: time.Now()})
		return
	}

	onEvent(logger.JobLog{Source: "FFMPEG", Level: "INFO", Index: index, Message: "Iniciando: " + filename, Time: time.Now()})

	// Probe total frame count for ETA calculation
	totalFrames := 0
	inputPath := cfg.BaseDir + "/" + source + "/" + filename
	if count, err := r.ProbeFrameCount(ctx, inputPath); err == nil {
		totalFrames = count
	}

	ffmpegProgress := func(p runner.Progress) {
		p.Source = "FFMPEG"
		p.Filename = filename
		if totalFrames > 0 && p.TotalFrames == 0 {
			p.TotalFrames = totalFrames
			if p.Frame > 0 {
				p.Percent = float64(p.Frame) / float64(totalFrames) * 100
			}
		}
		onProgress(p)
	}

	t := threads
	if t == 0 {
		t = cfg.HalfCPUs
	}

	err := r.FFmpegEncode(ctx,
		source+"/"+filename,
		"optimized/"+filename,
		crf,
		t,
		opts,
		"",
		true,
		resolution,
		ffmpegProgress,
	)
	if err != nil {
		onEvent(logger.JobLog{Source: "FFMPEG", Level: "ERRO", Index: index, Message: fmt.Sprintf("Falha ao processar: %s (%v)", filename, err), Time: time.Now()})
		return
	}

	if !files.FileExists(outPath) {
		onEvent(logger.JobLog{Source: "FFMPEG", Level: "ERRO", Index: index, Message: "ffmpeg retornou 0 mas output não existe: " + filename, Time: time.Now()})
		return
	}

	r.Chown(ctx, cfg.OptimizedDir, filename)
	onEvent(logger.JobLog{Source: "FFMPEG", Level: "OK", Index: index, Message: "Concluído: " + filename, Time: time.Now()})
}
