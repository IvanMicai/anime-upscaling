package process

import (
	"context"
	"fmt"
	"strings"
	"time"

	"anime-upscaling/internal/config"
	"anime-upscaling/internal/docker"
	"anime-upscaling/internal/logger"
)

// CheckFile runs a full decode pass on a single file to verify integrity.
func CheckFile(ctx context.Context, cfg config.Config, d *docker.Docker, filename string, index int, source string, onEvent func(logger.JobLog), onProgress func(docker.Progress)) {
	ffmpegProgress := func(p docker.Progress) {
		p.Source = "FFMPEG"
		onProgress(p)
	}

	onEvent(logger.JobLog{Source: "FFMPEG", Level: "INFO", Index: index, Message: "Verificando: " + filename, Time: time.Now()})

	output, err := d.FFmpegDecode(ctx, source+"/"+filename, "check", ffmpegProgress)
	if err != nil {
		// Filter out stats lines, keep only actual error lines
		var errors []string
		for _, line := range strings.Split(output, "\n") {
			line = strings.TrimSpace(line)
			if line == "" {
				continue
			}
			// Skip ffmpeg stats/progress lines
			if strings.Contains(line, "frame=") || strings.Contains(line, "size=") || strings.HasPrefix(line, "video:") {
				continue
			}
			errors = append(errors, line)
		}
		detail := strings.Join(errors, "; ")
		if detail == "" {
			detail = err.Error()
		}
		onEvent(logger.JobLog{Source: "FFMPEG", Level: "ERRO", Index: index, Message: fmt.Sprintf("Erro: %s (%s)", filename, detail), Time: time.Now()})
		return
	}

	onEvent(logger.JobLog{Source: "FFMPEG", Level: "OK", Index: index, Message: "Íntegro: " + filename, Time: time.Now()})
}
