package main

import (
	"context"
	"fmt"

	"anime-upscaling/internal/config"
	"anime-upscaling/internal/docker"
	"anime-upscaling/internal/files"
	"anime-upscaling/internal/logger"
	"anime-upscaling/internal/process"
)

func cmdUpscale(ctx context.Context) error {
	cfg := config.NewConfig()
	d := docker.NewDocker(cfg)

	log, err := logger.NewLogger(cfg.LogFile)
	if err != nil {
		return err
	}
	defer log.Close()

	fileList, err := files.ListVideos(cfg.InputDir, cfg.VideoExts)
	if err != nil {
		return fmt.Errorf("list videos: %w", err)
	}
	if len(fileList) == 0 {
		fmt.Println("Nenhum vídeo encontrado em " + cfg.InputDir)
		return nil
	}

	log.SetTotal(len(fileList))
	log.Banner(fmt.Sprintf("Iniciando processamento de %d arquivos com fila dinâmica...", len(fileList)))

	err = process.RunUpscale(ctx, cfg, d, fileList, func(e logger.JobLog) {
		log.Log(e.Source, e.Level, e.Index, e.Message)
	}, nil)

	log.Banner("Tudo pronto!")
	return err
}
