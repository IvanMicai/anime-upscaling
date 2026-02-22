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

	fmt.Printf("Parando containers %s*...\n", docker.ContainerPrefix)
	n, err := d.StopByPrefix(ctx, docker.ContainerPrefix)
	if err != nil {
		return fmt.Errorf("erro ao parar containers: %w", err)
	}
	if n == 0 {
		fmt.Println("Nenhum container rodando.")
	} else {
		fmt.Printf("Containers parados (%d).\n", n)
	}
	return nil
}
