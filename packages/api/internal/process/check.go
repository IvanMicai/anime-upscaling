package process

import (
	"context"
	"fmt"
	"strings"
	"time"

	"anime-upscaling/internal/config"
	"anime-upscaling/internal/logger"
	"anime-upscaling/internal/runner"
)

// CheckFile runs a full decode pass on a single file to verify integrity.
// logSource is the source label emitted on logs/progress (e.g. "FFMPEG" or "FFMPEG 2");
// empty defaults to "FFMPEG".
func CheckFile(ctx context.Context, cfg config.Config, r *runner.Runner, filename string, index int, source, logSource string, onEvent func(logger.JobLog), onProgress func(runner.Progress)) {
	if logSource == "" {
		logSource = "FFMPEG"
	}
	ffmpegProgress := func(p runner.Progress) {
		p.Source = logSource
		onProgress(p)
	}

	onEvent(logger.JobLog{Source: logSource, Level: "INFO", Index: index, Message: "Verificando: " + filename, Time: time.Now()})

	output, err := r.FFmpegDecode(ctx, source+"/"+filename, "check", ffmpegProgress)
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
		onEvent(logger.JobLog{Source: logSource, Level: "ERRO", Index: index, Message: fmt.Sprintf("Erro: %s (%s)", filename, detail), Time: time.Now()})
		return
	}

	onEvent(logger.JobLog{Source: logSource, Level: "OK", Index: index, Message: "Íntegro: " + filename, Time: time.Now()})
}
