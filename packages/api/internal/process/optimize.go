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

func RunOptimize(ctx context.Context, cfg config.Config, d *docker.Docker, fileList []string, onEvent func(logger.JobLog), onProgress func(docker.Progress)) error {
	if err := os.MkdirAll(cfg.OptimizedDir, 0755); err != nil {
		return fmt.Errorf("mkdir optimized: %w", err)
	}

	ffmpegProgress := func(p docker.Progress) {
		p.Source = "FFMPEG"
		onProgress(p)
	}

	for i, filename := range fileList {
		index := i + 1

		if err := ctx.Err(); err != nil {
			return err
		}

		outPath := filepath.Join(cfg.OptimizedDir, filename)
		if files.FileExists(outPath) {
			onEvent(logger.JobLog{Source: "FFMPEG", Level: "SKIP", Index: index, Message: "Pulando " + filename + " (já existe)", Time: time.Now()})
			continue
		}

		onEvent(logger.JobLog{Source: "FFMPEG", Level: "INFO", Index: index, Message: "Iniciando: " + filename, Time: time.Now()})

		err := d.FFmpegEncode(ctx,
			"input/"+filename,
			"optimized/"+filename,
			19,
			cfg.HalfCPUs,
			"ffmpeg-optimize",
			true,
			ffmpegProgress,
		)
		if err != nil {
			onEvent(logger.JobLog{Source: "FFMPEG", Level: "ERRO", Index: index, Message: fmt.Sprintf("Falha ao processar: %s (%v)", filename, err), Time: time.Now()})
			continue
		}

		if !files.FileExists(outPath) {
			onEvent(logger.JobLog{Source: "FFMPEG", Level: "ERRO", Index: index, Message: "ffmpeg retornou 0 mas output não existe: " + filename, Time: time.Now()})
			continue
		}

		d.Chown(ctx, cfg.OptimizedDir, filename)
		onEvent(logger.JobLog{Source: "FFMPEG", Level: "OK", Index: index, Message: "Concluído: " + filename, Time: time.Now()})
	}

	return nil
}
