package process

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"anime-upscaling/internal/config"
	"anime-upscaling/internal/docker"
	"anime-upscaling/internal/files"
	"anime-upscaling/internal/logger"
)

// RunOptimize processes all files sequentially with FFmpeg (CLI convenience wrapper).
func RunOptimize(ctx context.Context, cfg config.Config, d *docker.Docker, fileList []string, onEvent func(logger.JobLog), onProgress func(docker.Progress)) error {
	for i, f := range fileList {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		OptimizeFile(ctx, cfg, d, f, i+1, "input", onEvent, safeProgress(onProgress))
	}
	return nil
}

// OptimizeFile compresses a single file from input/ to optimized/ using FFmpeg.
func OptimizeFile(ctx context.Context, cfg config.Config, d *docker.Docker, filename string, index int, source string, onEvent func(logger.JobLog), onProgress func(docker.Progress)) {
	if err := os.MkdirAll(cfg.OptimizedDir, 0755); err != nil {
		onEvent(logger.JobLog{Source: "FFMPEG", Level: "ERRO", Index: index, Message: fmt.Sprintf("mkdir optimized: %v", err), Time: time.Now()})
		return
	}

	ffmpegProgress := func(p docker.Progress) {
		p.Source = "FFMPEG"
		onProgress(p)
	}

	outPath := filepath.Join(cfg.OptimizedDir, filename)
	if files.FileExists(outPath) {
		onEvent(logger.JobLog{Source: "FFMPEG", Level: "SKIP", Index: index, Message: "Pulando " + filename + " (já existe)", Time: time.Now()})
		return
	}

	onEvent(logger.JobLog{Source: "FFMPEG", Level: "INFO", Index: index, Message: "Iniciando: " + filename, Time: time.Now()})

	err := d.FFmpegEncode(ctx,
		source+"/"+filename,
		"optimized/"+filename,
		19,
		cfg.HalfCPUs,
		"",
		true,
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

	d.Chown(ctx, cfg.OptimizedDir, filename)
	onEvent(logger.JobLog{Source: "FFMPEG", Level: "OK", Index: index, Message: "Concluído: " + filename, Time: time.Now()})
}
