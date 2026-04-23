package process

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"anime-upscaling/internal/config"
	"anime-upscaling/internal/files"
	"anime-upscaling/internal/logger"
	"anime-upscaling/internal/runner"
)

// RunOptimize processes all files sequentially with FFmpeg (CLI convenience wrapper).
func RunOptimize(ctx context.Context, cfg config.Config, r *runner.Runner, fileList []string, onEvent func(logger.JobLog), onProgress func(runner.Progress)) error {
	for i, f := range fileList {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		OptimizeFile(ctx, cfg, r, f, i+1, "input", "FFMPEG", 1, 19, 0, runner.EncodeOptions{}, onEvent, safeProgress(onProgress))
	}
	return nil
}

// OptimizeFile compresses a single file from input/ to optimized/ using FFmpeg.
// logSource is the source label emitted on logs/progress (e.g. "FFMPEG" or "FFMPEG 2");
// empty defaults to "FFMPEG".
func OptimizeFile(ctx context.Context, cfg config.Config, r *runner.Runner, filename string, index int, source, logSource string, resolution int, crf int, threads int, opts runner.EncodeOptions, onEvent func(logger.JobLog), onProgress func(runner.Progress)) bool {
	if logSource == "" {
		logSource = "FFMPEG"
	}
	tempOptDir := cfg.TempDir + "/optimized"
	for _, dir := range []string{cfg.OptimizedDir, tempOptDir} {
		if err := os.MkdirAll(dir, 0755); err != nil {
			onEvent(logger.JobLog{Source: logSource, Level: "ERRO", Index: index, Message: fmt.Sprintf("mkdir optimized: %v", err), Time: time.Now()})
			return false
		}
	}

	outPath := filepath.Join(cfg.OptimizedDir, filename)
	if files.FileExists(outPath) {
		onEvent(logger.JobLog{Source: logSource, Level: "SKIP", Index: index, Message: "Pulando " + filename + " (já existe)", Time: time.Now()})
		return true
	}

	inputPath := cfg.BaseDir + "/" + source + "/" + filename

	// Pre-check: verify the input decodes end-to-end before we commit to a long encode.
	// Catches corrupted outputs from upstream steps (e.g. truncated interpolation writes)
	// that would otherwise surface as a mid-encode SIGSEGV in the ffmpeg encoder.
	decodeOut, decodeErr := r.FFmpegDecode(ctx, source+"/"+filename, "precheck-optimize", nil)
	if decodeErr != nil {
		if ctx.Err() != nil {
			return false
		}
		onEvent(logger.JobLog{
			Source: logSource, Level: "ERRO", Index: index,
			Message: fmt.Sprintf("PRE-CHECK FALHOU: %s (%v) — input %s %s (ver ffmpeg.log)",
				filename, decodeErr, inputPath, inputFileMeta(inputPath)),
			Time: time.Now(),
		})
		return false
	}

	onEvent(logger.JobLog{Source: logSource, Level: "INFO", Index: index, Message: "Iniciando: " + filename, Time: time.Now()})

	// Probe total frame count for ETA calculation. ProbeFrameCount tries cheap
	// metadata-based strategies; when they all fail (some MKVs lack usable
	// duration/frame tags) fall back to the exact count from the precheck
	// decode we already ran above.
	totalFrames := 0
	if count, err := r.ProbeFrameCount(ctx, inputPath); err == nil {
		totalFrames = count
	}
	if totalFrames == 0 {
		totalFrames = runner.ExtractFinalFrameCount(decodeOut)
	}

	ffmpegProgress := func(p runner.Progress) {
		p.Source = logSource
		p.Filename = filename
		if totalFrames > 0 && p.TotalFrames == 0 {
			p.TotalFrames = totalFrames
			if p.Frame > 0 {
				p.Percent = float64(p.Frame) / float64(totalFrames) * 100
			}
		}
		onProgress(p)
	}

	t := threads
	if t == 0 {
		t = cfg.HalfCPUs
	}

	tempOutPath := filepath.Join(tempOptDir, filename)

	attempt := func(attemptOpts runner.EncodeOptions) error {
		os.Remove(tempOutPath)
		return r.FFmpegEncode(ctx,
			source+"/"+filename,
			"temp/optimized/"+filename,
			crf,
			t,
			attemptOpts,
			"",
			true,
			resolution,
			ffmpegProgress,
		)
	}

	err := attempt(opts)
	if err != nil && ctx.Err() == nil {
		if sig, signaled := runner.SignalFromError(err); signaled {
			fallback := fallbackEncodeOptions(opts)
			onEvent(logger.JobLog{
				Source: logSource, Level: "INFO", Index: index,
				Message: fmt.Sprintf("Tentativa 1 morreu com signal %s; repetindo %s com codec=%s preset=%s pixfmt=%s",
					sig, filename, fallback.Codec, fallback.Preset, fallback.PixFmt),
				Time: time.Now(),
			})
			err = attempt(fallback)
		}
	}

	if err != nil {
		os.Remove(tempOutPath)
		onEvent(logger.JobLog{
			Source: logSource, Level: "ERRO", Index: index,
			Message: fmt.Sprintf("Falha ao processar: %s (%s) — input %s %s (ver ffmpeg.log)",
				filename, describeRunError(err), inputPath, inputFileMeta(inputPath)),
			Time: time.Now(),
		})
		return false
	}

	if !files.FileExists(tempOutPath) {
		onEvent(logger.JobLog{Source: logSource, Level: "ERRO", Index: index, Message: "ffmpeg retornou 0 mas output não existe: " + filename, Time: time.Now()})
		return false
	}

	if err := os.Rename(tempOutPath, outPath); err != nil {
		os.Remove(tempOutPath)
		onEvent(logger.JobLog{Source: logSource, Level: "ERRO", Index: index, Message: fmt.Sprintf("Falha ao mover output: %s (%v)", filename, err), Time: time.Now()})
		return false
	}

	r.Chown(ctx, cfg.OptimizedDir, filename)
	onEvent(logger.JobLog{Source: logSource, Level: "OK", Index: index, Message: "Concluído: " + filename, Time: time.Now()})
	return true
}

// fallbackEncodeOptions returns a conservative set of encode options for the
// retry attempt when the primary encoder died by signal. Switches to libx264,
// 8-bit pixel format and a medium preset to sidestep libx265/10-bit crashes.
// Preserves the original AudioCodec and any caller-provided ExtraArgs.
func fallbackEncodeOptions(orig runner.EncodeOptions) runner.EncodeOptions {
	fb := runner.EncodeOptions{
		Codec:      "libx264",
		Preset:     "medium",
		Tune:       "animation",
		PixFmt:     "yuv420p",
		AudioCodec: orig.AudioCodec,
		ExtraArgs:  orig.ExtraArgs,
	}
	return fb.WithDefaults()
}

// inputFileMeta returns a "size=N mtime=..." string for error logs, or an
// empty string if the file cannot be stat'd.
func inputFileMeta(path string) string {
	info, err := os.Stat(path)
	if err != nil {
		return "(stat falhou: " + err.Error() + ")"
	}
	return fmt.Sprintf("size=%d mtime=%s", info.Size(), info.ModTime().Format(time.RFC3339))
}

// describeRunError renders err including signal info when present.
func describeRunError(err error) string {
	if sig, signaled := runner.SignalFromError(err); signaled {
		return fmt.Sprintf("signal=%s: %v", sig, err)
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		return fmt.Sprintf("exit=%d: %v", exitErr.ExitCode(), err)
	}
	return err.Error()
}
