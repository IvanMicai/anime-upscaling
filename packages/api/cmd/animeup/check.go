package main

import (
	"context"
	"fmt"
	"strings"

	"anime-upscaling/internal/config"
	"anime-upscaling/internal/docker"
	"anime-upscaling/internal/files"
	"anime-upscaling/internal/logger"
)

func cmdCheck(ctx context.Context, args []string) error {
	cfg := config.NewConfig()
	d := docker.NewDocker(cfg)

	dirs := args
	if len(dirs) == 0 {
		dirs = []string{"input", "output", "optimized"}
	}

	fmt.Printf("%s%sVerificação de integridade de vídeos%s\n", logger.ColorBold, logger.ColorCyan, logger.ColorReset)
	fmt.Printf("Base: %s%s%s\n", logger.ColorCyan, cfg.BaseDir, logger.ColorReset)
	fmt.Printf("Pastas: %s%s%s\n", logger.ColorCyan, strings.Join(dirs, " "), logger.ColorReset)

	globalOK, globalErro, globalWarn, globalTotal := 0, 0, 0, 0

	for _, dirName := range dirs {
		if ctx.Err() != nil {
			return ctx.Err()
		}

		dirPath := cfg.BaseDir + "/" + dirName

		if !files.FileExists(dirPath) {
			fmt.Printf("\n%s%s=== %s/ ===%s\n\n", logger.ColorBold, logger.ColorCyan, dirName, logger.ColorReset)
			fmt.Printf("  %sPasta não encontrada: %s%s\n", logger.ColorYellow, dirPath, logger.ColorReset)
			continue
		}

		videoFiles, err := files.ListVideos(dirPath, cfg.VideoExts)
		if err != nil {
			return fmt.Errorf("list %s: %w", dirName, err)
		}

		fmt.Printf("\n%s%s=== %s/ (%d arquivos) ===%s\n\n", logger.ColorBold, logger.ColorCyan, dirName, len(videoFiles), logger.ColorReset)

		if len(videoFiles) == 0 {
			fmt.Printf("  %sNenhum vídeo encontrado%s\n", logger.ColorYellow, logger.ColorReset)
			continue
		}

		dirOK, dirErro, dirWarn := 0, 0, 0

		for _, filename := range videoFiles {
			if ctx.Err() != nil {
				return ctx.Err()
			}

			globalTotal++
			relPath := dirName + "/" + filename

			// Stage 1: ffprobe
			probeOut, probeErr := d.FFprobe(ctx, relPath)
			if probeErr != nil {
				fmt.Printf("  %s[ERRO]%s  %s (ffprobe falhou)\n", logger.ColorRed, logger.ColorReset, relPath)
				if probeOut != "" {
					for _, line := range firstLines(probeOut, 5) {
						fmt.Printf("          %s↳ %s%s\n", logger.ColorRed, line, logger.ColorReset)
					}
				}
				dirErro++
				globalErro++
				continue
			}

			if !strings.Contains(probeOut, "video") {
				fmt.Printf("  %s[ERRO]%s  %s (nenhuma stream de vídeo encontrada)\n", logger.ColorRed, logger.ColorReset, relPath)
				dirErro++
				globalErro++
				continue
			}

			// Stage 2: ffmpeg decode
			decodeOut, decodeErr := d.FFmpegDecode(ctx, relPath)
			if decodeErr != nil || decodeOut != "" {
				if decodeErr != nil {
					fmt.Printf("  %s[ERRO]%s  %s (decode falhou, %v)\n", logger.ColorRed, logger.ColorReset, relPath, decodeErr)
					dirErro++
					globalErro++
				} else {
					fmt.Printf("  %s[WARN]%s  %s (decode com avisos)\n", logger.ColorYellow, logger.ColorReset, relPath)
					dirWarn++
					globalWarn++
				}
				for _, line := range firstLines(decodeOut, 5) {
					fmt.Printf("          %s↳ %s%s\n", logger.ColorRed, line, logger.ColorReset)
				}
				continue
			}

			fmt.Printf("  %s[OK]%s    %s\n", logger.ColorGreen, logger.ColorReset, relPath)
			dirOK++
			globalOK++
		}

		fmt.Println()
		fmt.Printf("  Resumo: %s%d ok%s, %s%d erro%s, %s%d warn%s / %d total\n",
			logger.ColorGreen, dirOK, logger.ColorReset,
			logger.ColorRed, dirErro, logger.ColorReset,
			logger.ColorYellow, dirWarn, logger.ColorReset,
			len(videoFiles))
	}

	// Final summary
	fmt.Printf("\n%s%s=== Resumo Final ===%s\n\n", logger.ColorBold, logger.ColorCyan, logger.ColorReset)
	fmt.Printf("  Total verificado: %s%d%s\n", logger.ColorBold, globalTotal, logger.ColorReset)
	fmt.Printf("  %sOK:   %d%s\n", logger.ColorGreen, globalOK, logger.ColorReset)
	fmt.Printf("  %sERRO: %d%s\n", logger.ColorRed, globalErro, logger.ColorReset)
	fmt.Printf("  %sWARN: %d%s\n", logger.ColorYellow, globalWarn, logger.ColorReset)
	fmt.Println()

	if globalErro > 0 {
		fmt.Printf("  %s%sArquivos com erro detectados!%s\n", logger.ColorRed, logger.ColorBold, logger.ColorReset)
		return fmt.Errorf("%d files with errors", globalErro)
	}
	if globalWarn > 0 {
		fmt.Printf("  %sAlguns arquivos tiveram avisos durante decode.%s\n", logger.ColorYellow, logger.ColorReset)
	} else {
		fmt.Printf("  %sTodos os arquivos estão íntegros.%s\n", logger.ColorGreen, logger.ColorReset)
	}
	return nil
}

func firstLines(s string, n int) []string {
	lines := strings.Split(strings.TrimSpace(s), "\n")
	if len(lines) > n {
		lines = lines[:n]
	}
	var result []string
	for _, l := range lines {
		if l != "" {
			result = append(result, l)
		}
	}
	return result
}
