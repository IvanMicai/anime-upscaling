package main

import (
	"context"
	"fmt"

	"anime-upscaling/internal/config"
	"anime-upscaling/internal/docker"
)

func cmdStop(ctx context.Context) error {
	cfg := config.NewConfig()
	d := docker.NewDocker(cfg)

	images := []struct {
		label string
		image string
	}{
		{"video2x", cfg.Video2xImage},
		{"ffmpeg", cfg.FFmpegImage},
	}

	for _, img := range images {
		fmt.Printf("Parando containers %s...\n", img.label)
		n, err := d.StopByImage(ctx, img.image)
		if err != nil {
			fmt.Printf("Erro ao parar %s: %v\n", img.label, err)
			continue
		}
		if n == 0 {
			fmt.Printf("Nenhum container %s rodando.\n", img.label)
		} else {
			fmt.Printf("Containers %s parados (%d).\n", img.label, n)
		}
	}
	return nil
}
