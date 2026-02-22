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

func RunPipeline(ctx context.Context, cfg config.Config, d *docker.Docker, fileList []string, onEvent func(logger.JobLog), onProgress func(docker.Progress)) error {
	for _, dir := range []string{cfg.OutputDir, cfg.OptimizedDir} {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return fmt.Errorf("mkdir: %w", err)
		}
	}

	type work struct {
		filename string
		index    int
	}

	// Channel for GPU work-stealing
	fileCh := make(chan work, len(fileList))
	for i, f := range fileList {
		fileCh <- work{filename: f, index: i + 1}
	}
	close(fileCh)

	// Channel for GPU→ffmpeg signaling
	readyCh := make(chan string, len(fileList))

	// GPU goroutines
	var gpuWg sync.WaitGroup
	gpuCount := 2

	for gpuID := 0; gpuID < gpuCount; gpuID++ {
		gpuWg.Add(1)
		go func(gpuID int) {
			defer gpuWg.Done()
			source := fmt.Sprintf("GPU %d", gpuID)

			for w := range fileCh {
				if ctx.Err() != nil {
					return
				}

				outPath := filepath.Join(cfg.OutputDir, w.filename)
				if files.FileExists(outPath) {
					onEvent(logger.JobLog{Source: source, Level: "SKIP", Index: w.index, Message: "Pulando " + w.filename + " (já existe)", Time: time.Now()})
					readyCh <- w.filename
					continue
				}

				onEvent(logger.JobLog{Source: source, Level: "INFO", Index: w.index, Message: "Iniciando: " + w.filename, Time: time.Now()})

				dockerLog := fmt.Sprintf("%s/docker_gpu%d.log", cfg.BaseDir, gpuID)
				err := d.Video2x(ctx, gpuID, w.filename, dockerLog)

				if err != nil {
					onEvent(logger.JobLog{Source: source, Level: "ERRO", Index: w.index, Message: fmt.Sprintf("Falha ao processar: %s (%v)", w.filename, err), Time: time.Now()})
					continue
				}

				if !files.FileExists(outPath) {
					onEvent(logger.JobLog{Source: source, Level: "ERRO", Index: w.index, Message: "video2x retornou 0 mas output não existe: " + w.filename, Time: time.Now()})
					continue
				}

				d.Chown(ctx, cfg.OutputDir, w.filename)
				onEvent(logger.JobLog{Source: source, Level: "OK", Index: w.index, Message: "Concluído: " + w.filename, Time: time.Now()})

				readyCh <- w.filename
			}
		}(gpuID)
	}

	// Close readyCh when all GPUs are done
	go func() {
		gpuWg.Wait()
		close(readyCh)
	}()

	// FFmpeg consumer goroutine
	var ffmpegWg sync.WaitGroup
	ffmpegWg.Add(1)
	go func() {
		defer ffmpegWg.Done()

		for filename := range readyCh {
			if ctx.Err() != nil {
				return
			}

			optPath := filepath.Join(cfg.OptimizedDir, filename)
			if files.FileExists(optPath) {
				onEvent(logger.JobLog{Source: "FFMPEG", Level: "SKIP", Index: 0, Message: "Pulando " + filename + " (já existe)", Time: time.Now()})
				continue
			}

			onEvent(logger.JobLog{Source: "FFMPEG", Level: "INFO", Index: 0, Message: "Comprimindo: " + filename, Time: time.Now()})

			err := d.FFmpegEncode(ctx,
				"output/"+filename,
				"optimized/"+filename,
				22,
				cfg.HalfCPUs,
				"ffmpeg-pipeline",
				false,
				onProgress,
			)
			if err != nil {
				onEvent(logger.JobLog{Source: "FFMPEG", Level: "ERRO", Index: 0, Message: fmt.Sprintf("Falha: %s (%v)", filename, err), Time: time.Now()})
				continue
			}

			onEvent(logger.JobLog{Source: "FFMPEG", Level: "OK", Index: 0, Message: "Concluído: " + filename, Time: time.Now()})
		}

		onEvent(logger.JobLog{Source: "FFMPEG", Level: "INFO", Index: 0, Message: "Worker ffmpeg finalizado.", Time: time.Now()})
	}()

	ffmpegWg.Wait()
	return nil
}
