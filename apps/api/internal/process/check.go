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
		p.Filename = filename
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
			if isFFmpegProgressLine(line) {
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

func isFFmpegProgressLine(line string) bool {
	if strings.HasPrefix(line, "video:") {
		return true
	}
	for _, prefix := range []string{
		"frame=", "fps=", "stream_", "bitrate=", "total_size=", "out_time_",
		"out_time=", "dup_frames=", "drop_frames=", "speed=", "progress=",
	} {
		if strings.HasPrefix(line, prefix) {
			return true
		}
	}
	return strings.Contains(line, "size=")
}
