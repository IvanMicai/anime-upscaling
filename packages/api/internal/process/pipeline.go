package process

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"anime-upscaling/internal/config"
	"anime-upscaling/internal/docker"
	"anime-upscaling/internal/files"
	"anime-upscaling/internal/logger"
)

// RunPipeline runs the full upscale+encode pipeline with 2 GPU workers feeding 1 FFmpeg worker (CLI convenience wrapper).
func RunPipeline(ctx context.Context, cfg config.Config, d *docker.Docker, fileList []string, scale int, onEvent func(logger.JobLog), onProgress func(docker.Progress)) error {
	type work struct {
		filename string
		index    int
	}
	fileCh := make(chan work, len(fileList))
	for i, f := range fileList {
		fileCh <- work{filename: f, index: i + 1}
	}
	close(fileCh)

	readyCh := make(chan string, len(fileList))

	var gpuWg sync.WaitGroup
	gpuCount := 2
	for gpuID := 0; gpuID < gpuCount; gpuID++ {
		gpuWg.Add(1)
		go func(gpuID int) {
			defer gpuWg.Done()
			for w := range fileCh {
				if ctx.Err() != nil {
					return
				}
				ok := UpscaleFile(ctx, cfg, d, gpuID, w.filename, w.index, scale, onEvent, safeProgress(onProgress))
				if ok {
					readyCh <- w.filename
				}
			}
		}(gpuID)
	}

	go func() {
		gpuWg.Wait()
		close(readyCh)
	}()

	for filename := range readyCh {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		EncodeFile(ctx, cfg, d, filename, onEvent, safeProgress(onProgress))
	}
	return nil
}

// EncodeFile compresses a single file from output/ to optimized/ using FFmpeg (pipeline phase 2).
func EncodeFile(ctx context.Context, cfg config.Config, d *docker.Docker, filename string, onEvent func(logger.JobLog), onProgress func(docker.Progress)) {
	for _, dir := range []string{cfg.OutputDir, cfg.OptimizedDir} {
		if err := os.MkdirAll(dir, 0755); err != nil {
			onEvent(logger.JobLog{Source: "FFMPEG", Level: "ERRO", Index: 0, Message: fmt.Sprintf("mkdir: %v", err), Time: time.Now()})
			return
		}
	}

	ffmpegProgress := func(p docker.Progress) {
		p.Source = "FFMPEG"
		onProgress(p)
	}

	optPath := filepath.Join(cfg.OptimizedDir, filename)
	if files.FileExists(optPath) {
		onEvent(logger.JobLog{Source: "FFMPEG", Level: "SKIP", Index: 0, Message: "Pulando " + filename + " (já existe)", Time: time.Now()})
		return
	}

	onEvent(logger.JobLog{Source: "FFMPEG", Level: "INFO", Index: 0, Message: "Comprimindo: " + filename, Time: time.Now()})

	err := d.FFmpegEncode(ctx,
		"output/"+filename,
		"optimized/"+filename,
		22,
		cfg.HalfCPUs,
		"",
		false,
		1,
		ffmpegProgress,
	)
	if err != nil {
		onEvent(logger.JobLog{Source: "FFMPEG", Level: "ERRO", Index: 0, Message: fmt.Sprintf("Falha: %s (%v)", filename, err), Time: time.Now()})
		return
	}

	onEvent(logger.JobLog{Source: "FFMPEG", Level: "OK", Index: 0, Message: "Concluído: " + filename, Time: time.Now()})
}
