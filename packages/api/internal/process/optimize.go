package process

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
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
		OptimizeFile(ctx, cfg, r, f, i+1, "input", "FFMPEG", 1, 1, 0, 19, 0, runner.EncodeOptions{}, onEvent, safeProgress(onProgress))
	}
	return nil
}

// OptimizeFile compresses a single file from input/ to optimized/ using FFmpeg.
// logSource is the source label emitted on logs/progress (e.g. "FFMPEG" or "FFMPEG 2");
// empty defaults to "FFMPEG".
// frameRateAbsolute > 0 takes precedence over frameRate (the divisor) and
// sets a fixed target FPS, capped at the source FPS by the encode filter.
func OptimizeFile(ctx context.Context, cfg config.Config, r *runner.Runner, filename string, index int, source, logSource string, resolution int, frameRate int, frameRateAbsolute float64, crf int, threads int, opts runner.EncodeOptions, onEvent func(logger.JobLog), onProgress func(runner.Progress)) bool {
	if logSource == "" {
		logSource = "FFMPEG"
	}
	tempOptDir := cfg.TempDir + "/optimized"
	subDir := filepath.Dir(filename)
	for _, dir := range []string{cfg.OptimizedDir, tempOptDir} {
		target := dir
		if subDir != "." && subDir != "" {
			target = filepath.Join(dir, subDir)
		}
		if err := os.MkdirAll(target, 0755); err != nil {
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
	precheckStarted := time.Now()
	precheckProgress := func(p runner.Progress) {
		p.Source = logSource
		p.Filename = filename
		if p.Phase == "" {
			p.Phase = "Pre-check"
		}
		onProgress(p)
	}
	onEvent(logger.JobLog{Source: logSource, Level: "INFO", Index: index, Message: "Pre-checking input: " + filename, Time: time.Now()})
	decodeOut, decodeErr := r.FFmpegDecode(ctx, source+"/"+filename, "precheck-optimize", precheckProgress)
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
	onEvent(logger.JobLog{Source: logSource, Level: "INFO", Index: index, Message: fmt.Sprintf("Pre-check completed in %s: %s", runner.FormatDuration(time.Since(precheckStarted)), filename), Time: time.Now()})

	// Orçamento de threads por encode: explícito do job, ou a fatia justa da
	// máquina (CPUs / streams paralelos) para que N encodes simultâneos não
	// disputem os mesmos cores.
	t := threads
	if t == 0 {
		t = cfg.NumCPUs / max(cfg.FFmpegStreams, 1)
		if t < 1 {
			t = 1
		}
	}

	// O -threads do ffmpeg só controla os frame threads do libx265; o worker
	// pool interno (pools) é dimensionado por padrão para TODOS os cores da
	// máquina, em cada instância. Com vários encodes paralelos isso gera
	// oversubscription massiva de CPU e multiplica o uso de RAM (pool +
	// lookahead), a ponto de o OOM killer derrubar os processos (SIGKILL).
	// pools=<t> confina cada encode ao seu orçamento de threads.
	opts = opts.WithDefaults()
	startMsg := "Iniciando: " + filename
	if opts.Codec == "libx265" && !opts.UseGPU {
		pools := strconv.Itoa(t)
		if t == 1 {
			pools = "none"
		}
		opts.ExtraArgs = setX265Pools(opts.ExtraArgs, pools)
		startMsg = fmt.Sprintf("Iniciando: %s (threads=%d pools=%s)", filename, t, pools)
	}

	onEvent(logger.JobLog{Source: logSource, Level: "INFO", Index: index, Message: startMsg, Time: time.Now()})

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
	if frameRateAbsolute > 0 && totalFrames > 0 {
		// fps filter caps at source_fps, so the output frame count is
		// totalFrames * min(absolute, sourceFps) / sourceFps.
		if sourceFps, _ := r.ProbeFrameRate(ctx, inputPath); sourceFps > 0 {
			target := frameRateAbsolute
			if target > sourceFps {
				target = sourceFps
			}
			totalFrames = int(float64(totalFrames)*target/sourceFps + 0.5)
		}
	} else if frameRate > 1 && totalFrames > 0 {
		totalFrames = (totalFrames + frameRate - 1) / frameRate
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

	tempOutPath := filepath.Join(tempOptDir, filename)

	// Encode em duas fases: (1) vídeo+áudio num intermediário sem legendas —
	// faixas de legenda esparsas quebram o interleave do muxer durante encodes
	// lentos (áudio adiantado ou buffer ilimitado → OOM, ver FFmpegEncode);
	// (2) remux -c copy rápido devolvendo as legendas do arquivo original.
	avRelPath := "temp/optimized/" + filename + ".av.mkv"
	tempAVPath := tempOutPath + ".av.mkv"

	attempt := func(attemptOpts runner.EncodeOptions) error {
		_ = os.Remove(tempOutPath)
		_ = os.Remove(tempAVPath)
		defer os.Remove(tempAVPath)
		if err := r.FFmpegEncode(ctx,
			source+"/"+filename,
			avRelPath,
			crf,
			t,
			attemptOpts,
			"",
			true,
			resolution,
			frameRate,
			frameRateAbsolute,
			ffmpegProgress,
		); err != nil {
			return err
		}
		return r.FFmpegRemuxSubtitles(ctx, avRelPath, source+"/"+filename, "temp/optimized/"+filename, ffmpegProgress)
	}

	// Tiered retry on encoder signal death (e.g. SIGSEGV in libx265): keep the
	// exact same codec profile so the output stays consistent with the rest of
	// the batch (HEVC 10-bit). First retry repeats the identical command — these
	// crashes are intermittent and fail early, so an identical re-run often
	// succeeds. If it dies again we add an x265 thread-pool mitigation
	// (pools=none) that sidesteps the crash without changing codec/preset/pixfmt.
	attempts := []struct {
		opts  runner.EncodeOptions
		label string
	}{
		{opts, "primária"},
		{opts, "retry idêntico"},
		{stableEncodeOptions(opts), "x265 estável (pools=none)"},
	}

	var err error
	for i, a := range attempts {
		err = attempt(a.opts)
		if err == nil || ctx.Err() != nil {
			break
		}
		sig, signaled := runner.SignalFromError(err)
		if !signaled || i == len(attempts)-1 {
			break
		}
		next := attempts[i+1]
		onEvent(logger.JobLog{
			Source: logSource, Level: "INFO", Index: index,
			Message: fmt.Sprintf("Tentativa %d morreu com signal %s; repetindo %s (%s) com codec=%s preset=%s pixfmt=%s",
				i+1, sig, filename, next.label, next.opts.Codec, next.opts.Preset, next.opts.PixFmt),
			Time: time.Now(),
		})
	}

	if err != nil {
		_ = os.Remove(tempOutPath)
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
		_ = os.Remove(tempOutPath)
		onEvent(logger.JobLog{Source: logSource, Level: "ERRO", Index: index, Message: fmt.Sprintf("Falha ao mover output: %s (%v)", filename, err), Time: time.Now()})
		return false
	}

	_ = r.Chown(ctx, cfg.OptimizedDir, filename)
	onEvent(logger.JobLog{Source: logSource, Level: "OK", Index: index, Message: "Concluído: " + filename, Time: time.Now()})
	return true
}

// stableEncodeOptions returns a copy of orig with an x265 thread-pool
// mitigation applied. Disabling the x265 worker pool (pools=none) sidesteps
// the intermittent SIGSEGV some inputs trigger in libx265's threaded encoder,
// while keeping the exact same codec, preset, pixel format and CRF so the
// output stays consistent with the rest of the batch (HEVC 10-bit). Any
// pools=N already present (the per-stream budget set by OptimizeFile) is
// overridden — the whole point of this tier is removing the pool.
func stableEncodeOptions(orig runner.EncodeOptions) runner.EncodeOptions {
	out := orig.WithDefaults()
	out.ExtraArgs = setX265Pools(out.ExtraArgs, "none")
	return out
}

// setX265Pools returns a copy of extra with the x265 pools option forced to
// the given value. If a -x265-params arg already exists, its pools= token is
// replaced (or appended when absent); otherwise a new -x265-params arg is
// added. The input slice is never mutated.
func setX265Pools(extra []string, pools string) []string {
	out := append([]string(nil), extra...)
	for i := 0; i+1 < len(out); i++ {
		if out[i] != "-x265-params" {
			continue
		}
		params := strings.Split(out[i+1], ":")
		replaced := false
		for j, p := range params {
			if strings.HasPrefix(p, "pools=") {
				params[j] = "pools=" + pools
				replaced = true
			}
		}
		if !replaced {
			params = append(params, "pools="+pools)
		}
		out[i+1] = strings.Join(params, ":")
		return out
	}
	return append(out, "-x265-params", "pools="+pools)
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
