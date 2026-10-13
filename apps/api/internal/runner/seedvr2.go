package runner

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
)

// SeedVR2 is a diffusion-transformer video restoration model (ByteDance,
// Apache-2.0), run through the headless CLI of numz/ComfyUI-SeedVR2_VideoUpscaler.
// It only exists in the image built with `--target seedvr2` (see the
// Dockerfile); SeedVR2Available reports whether this image has it.
//
// Unlike video2x it is a PyTorch/CUDA program: it picks the GPU through
// CUDA_VISIBLE_DEVICES, spawns an ffmpeg encoder of its own, drops audio and
// subtitles, and sizes its output by a target short side instead of a scale.

const (
	DefaultSeedVR2Model  = "seedvr2-7b"
	DefaultSeedVR2Height = 1080
)

// seedVR2Model maps a model name to its DiT weights file and the BlockSwap
// depth that fits a 16 GB GPU. The 7B files are the mixed fp8 builds (block 35
// kept in fp16), the 3B one is full fp16: all three leave room for activations.
type seedVR2Model struct {
	dit       string
	swap      int // default --blocks_to_swap
	maxBlocks int // the model's block count, used for the out-of-memory retry
}

var seedVR2Models = map[string]seedVR2Model{
	"seedvr2-3b":       {dit: "seedvr2_ema_3b_fp16.safetensors", swap: 0, maxBlocks: 32},
	"seedvr2-7b":       {dit: "seedvr2_ema_7b_fp8_e4m3fn_mixed_block35_fp16.safetensors", swap: 20, maxBlocks: 36},
	"seedvr2-7b-sharp": {dit: "seedvr2_ema_7b_sharp_fp8_e4m3fn_mixed_block35_fp16.safetensors", swap: 20, maxBlocks: 36},
}

const seedVR2VAE = "ema_vae_fp16.safetensors"

// SeedVR2Tuning is one set of memory settings for a run. The first attempt
// uses the configured defaults; after a CUDA out-of-memory error the caller
// retries with SafeSeedVR2Tuning.
type SeedVR2Tuning struct {
	BatchSize    int // frames per temporal batch, 4n+1
	BlocksToSwap int
	VAETileSize  int
}

// SeedVR2Tuning returns the first-attempt settings for model: SEEDVR2_BATCH_SIZE
// and SEEDVR2_BLOCKS_TO_SWAP override the built-in defaults.
func (r *Runner) SeedVR2Tuning(model string) SeedVR2Tuning {
	m := seedVR2Models[model]
	t := SeedVR2Tuning{BatchSize: 13, BlocksToSwap: m.swap, VAETileSize: 1024}
	if r.cfg.SeedVR2BatchSize > 0 {
		t.BatchSize = r.cfg.SeedVR2BatchSize
	}
	if r.cfg.SeedVR2BlocksToSwap >= 0 {
		t.BlocksToSwap = min(r.cfg.SeedVR2BlocksToSwap, m.maxBlocks)
	}
	return t
}

// SafeSeedVR2Tuning is the out-of-memory fallback: every block swapped to
// system RAM, the smallest useful batch and half-size VAE tiles. Slower and
// less temporally stable, but it fits where the defaults did not.
func SafeSeedVR2Tuning(model string) SeedVR2Tuning {
	return SeedVR2Tuning{BatchSize: 5, BlocksToSwap: seedVR2Models[model].maxBlocks, VAETileSize: 512}
}

// SeedVR2Available reports whether this image ships the SeedVR2 runtime.
func (r *Runner) SeedVR2Available() bool {
	return SeedVR2Installed(r.cfg.SeedVR2Dir)
}

// SeedVR2Installed reports whether dir holds the SeedVR2 venv and CLI.
func SeedVR2Installed(dir string) bool {
	for _, p := range []string{seedVR2Python(dir), seedVR2Script(dir)} {
		if _, err := os.Stat(p); err != nil {
			return false
		}
	}
	return true
}

func seedVR2Python(dir string) string { return filepath.Join(dir, "venv", "bin", "python") }
func seedVR2Script(dir string) string { return filepath.Join(dir, "app", "inference_cli.py") }

// seedVR2FetchMu serializes weight downloads. The CLI downloads into
// "<file>.download" and renames at the end, so two runs starting together
// on a fresh install would append to the same partial file.
var seedVR2FetchMu sync.Mutex

// SeedVR2FetchModels makes sure the weights for model are in the model
// directory, downloading them (several GB on first use) if needed. The CLI
// skips files already present, so this is fast once they are cached.
func (r *Runner) SeedVR2FetchModels(ctx context.Context, model, logPath string, onProgress func(Progress)) error {
	m, ok := seedVR2Models[model]
	if !ok {
		return fmt.Errorf("unknown SeedVR2 model %q", model)
	}
	seedVR2FetchMu.Lock()
	defer seedVR2FetchMu.Unlock()

	if err := os.MkdirAll(r.cfg.SeedVR2ModelDir, 0755); err != nil {
		return fmt.Errorf("create model dir: %w", err)
	}
	f, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return fmt.Errorf("open log: %w", err)
	}
	defer f.Close()

	code := "import sys; from src.utils.downloads import download_weight; " +
		"sys.exit(0 if download_weight(dit_model=sys.argv[1], vae_model=sys.argv[2], model_dir=sys.argv[3]) else 1)"
	cmd := exec.CommandContext(ctx, seedVR2Python(r.cfg.SeedVR2Dir), "-c", code, m.dit, seedVR2VAE, r.cfg.SeedVR2ModelDir)
	cmd.Dir = filepath.Dir(seedVR2Script(r.cfg.SeedVR2Dir))
	setProcessGroup(cmd)

	if onProgress != nil {
		onProgress(Progress{Phase: "Download"})
	}
	tail := newTailWriter(20)
	cmd.Stdout = io.MultiWriter(f, tail)
	cmd.Stderr = io.MultiWriter(f, tail)

	label := ProcessPrefix + "seedvr2-fetch"
	registerForCtx(ctx, label, cmd)
	defer tracker.unregister(label)
	return runError(cmd.Run(), tail)
}

// SeedVR2 upscales inputPath to outputPath (an .mp4 holding only the video
// track, x265 10-bit at CRF 12) on the given GPU. totalFrames, when known,
// lets progress report frames and fps instead of a bare percentage.
func (r *Runner) SeedVR2(ctx context.Context, gpuID, streamIdx int, inputPath, outputPath, logPath string, opts UpscaleOptions, tuning SeedVR2Tuning, totalFrames int, onProgress func(Progress)) error {
	opts = opts.WithDefaults()
	m, ok := seedVR2Models[opts.Model]
	if !ok {
		return fmt.Errorf("unknown SeedVR2 model %q", opts.Model)
	}

	f, err := os.Create(logPath)
	if err != nil {
		return fmt.Errorf("create log: %w", err)
	}
	defer f.Close()

	cmd := exec.CommandContext(ctx, seedVR2Python(r.cfg.SeedVR2Dir), seedVR2Args(r.cfg.SeedVR2ModelDir, m.dit, inputPath, outputPath, opts.TargetHeight, tuning, r.cfg.SeedVR2ExtraArgs)...)
	cmd.Dir = filepath.Dir(seedVR2Script(r.cfg.SeedVR2Dir))
	// Vulkan (video2x) and CUDA may enumerate GPUs in different orders; PCI
	// bus order makes CUDA device N the same card as video2x's -d N on the
	// usual single-vendor host.
	cmd.Env = append(os.Environ(),
		"CUDA_DEVICE_ORDER=PCI_BUS_ID",
		"CUDA_VISIBLE_DEVICES="+strconv.Itoa(gpuID),
		"PYTHONUNBUFFERED=1",
	)
	setProcessGroup(cmd)

	tail := newTailWriter(20)
	out := io.MultiWriter(f, tail)
	if onProgress != nil {
		out = io.MultiWriter(newSeedVR2ProgressWriter(f, totalFrames, onProgress), tail)
	}
	cmd.Stdout = out
	cmd.Stderr = out

	label := fmt.Sprintf("%sseedvr2-gpu%d-s%d", ProcessPrefix, gpuID, streamIdx)
	registerForCtx(ctx, label, cmd)
	defer tracker.unregister(label)

	return runError(cmd.Run(), tail)
}

// seedVR2Args builds the inference_cli.py command line.
func seedVR2Args(modelDir, dit, inputPath, outputPath string, height int, t SeedVR2Tuning, extra []string) []string {
	args := []string{
		"inference_cli.py", inputPath,
		"--output", outputPath,
		"--output_format", "mp4",
		// The ffmpeg backend writes x265 10-bit at CRF 12: a near-lossless
		// intermediate that a later Optimize step re-encodes.
		"--video_backend", "ffmpeg", "--10bit",
		"--model_dir", modelDir,
		"--dit_model", dit,
		"--resolution", strconv.Itoa(height),
		"--batch_size", strconv.Itoa(t.BatchSize), "--uniform_batch_size",
		"--temporal_overlap", "3",
		// Streaming keeps system RAM bounded on long videos; --cache_* keeps
		// the models loaded between chunks instead of reloading per chunk.
		"--chunk_size", strconv.Itoa(t.BatchSize * 5),
		"--cache_dit", "--cache_vae",
		"--dit_offload_device", "cpu", "--vae_offload_device", "cpu",
		"--vae_encode_tiled", "--vae_decode_tiled",
		"--vae_encode_tile_size", strconv.Itoa(t.VAETileSize),
		"--vae_decode_tile_size", strconv.Itoa(t.VAETileSize),
		"--vae_encode_tile_overlap", strconv.Itoa(t.VAETileSize / 8),
		"--vae_decode_tile_overlap", strconv.Itoa(t.VAETileSize / 8),
		"--color_correction", "lab",
		"--seed", "42",
	}
	if t.BlocksToSwap > 0 {
		args = append(args, "--blocks_to_swap", strconv.Itoa(t.BlocksToSwap), "--swap_io_components")
	}
	return append(args, extra...)
}

// IsCUDAOutOfMemory reports whether a SeedVR2 failure was the GPU running out
// of memory, which a retry with SafeSeedVR2Tuning can fix.
func IsCUDAOutOfMemory(err error) bool {
	if err == nil {
		return false
	}
	s := strings.ToLower(err.Error())
	return strings.Contains(s, "out of memory") || strings.Contains(s, "outofmemoryerror")
}

// setProcessGroup puts cmd in a process group of its own and makes context
// cancellation kill the whole group: SeedVR2 runs an ffmpeg encoder as a
// child, which would otherwise outlive a cancelled job.
func setProcessGroup(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error {
		return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
	}
	cmd.WaitDelay = 10 * time.Second
}

var (
	reSeedVR2Chunk = regexp.MustCompile(`Chunk (\d+)/(\d+)`)
	reSeedVR2Batch = regexp.MustCompile(`(Encoding|Upscaling|Decoding|Post-processing) batch (\d+)/(\d+)`)
)

// seedVR2Phases are the per-chunk phases in the order the CLI runs them.
var seedVR2Phases = map[string]int{"Encoding": 0, "Upscaling": 1, "Decoding": 2, "Post-processing": 3}

// seedVR2ProgressWriter turns the CLI's log lines into Progress. The CLI
// prints no frame counter, only "Chunk i/N" and, inside each chunk,
// "<phase> batch j/n" for four phases, so progress is estimated as
// (finished chunks + phase fraction) / N.
type seedVR2ProgressWriter struct {
	underlying  io.Writer
	onProgress  func(Progress)
	totalFrames int
	startedAt   time.Time
	minInterval time.Duration

	mu       sync.Mutex
	buf      []byte
	chunk    int
	chunks   int
	lastEmit time.Time
}

func newSeedVR2ProgressWriter(w io.Writer, totalFrames int, onProgress func(Progress)) *seedVR2ProgressWriter {
	return &seedVR2ProgressWriter{underlying: w, onProgress: onProgress, totalFrames: totalFrames, startedAt: time.Now(), minInterval: time.Second}
}

func (pw *seedVR2ProgressWriter) Write(p []byte) (int, error) {
	n, err := pw.underlying.Write(p)
	pw.mu.Lock()
	defer pw.mu.Unlock()
	pw.buf = append(pw.buf, p[:n]...)
	for {
		i := strings.IndexAny(string(pw.buf), "\r\n")
		if i < 0 {
			break
		}
		line := string(pw.buf[:i])
		pw.buf = pw.buf[i+1:]
		pw.parseLine(line)
	}
	return n, err
}

func (pw *seedVR2ProgressWriter) parseLine(line string) {
	if m := reSeedVR2Chunk.FindStringSubmatch(line); m != nil {
		pw.chunk, _ = strconv.Atoi(m[1])
		pw.chunks, _ = strconv.Atoi(m[2])
		pw.emit(0, fmt.Sprintf("Chunk %d/%d", pw.chunk, pw.chunks), true)
		return
	}
	m := reSeedVR2Batch.FindStringSubmatch(line)
	if m == nil || pw.chunks == 0 {
		return
	}
	j, _ := strconv.Atoi(m[2])
	n, _ := strconv.Atoi(m[3])
	if n <= 0 {
		return
	}
	within := (float64(seedVR2Phases[m[1]]) + float64(j)/float64(n)) / float64(len(seedVR2Phases))
	pw.emit(within, fmt.Sprintf("Chunk %d/%d · %s", pw.chunk, pw.chunks, m[1]), false)
}

// emit reports progress at fraction `within` of the current chunk. Chunk
// boundaries always emit; batch lines are rate-limited to one per second.
func (pw *seedVR2ProgressWriter) emit(within float64, phase string, force bool) {
	now := time.Now()
	if !force && now.Sub(pw.lastEmit) < pw.minInterval {
		return
	}
	pw.lastEmit = now
	pct := (float64(pw.chunk-1) + within) / float64(pw.chunks) * 100
	elapsed := now.Sub(pw.startedAt)
	p := Progress{
		Percent: pct,
		Elapsed: FormatDuration(elapsed),
		Phase:   phase,
	}
	if pw.totalFrames > 0 {
		p.TotalFrames = pw.totalFrames
		p.Frame = int(pct / 100 * float64(pw.totalFrames))
		if secs := elapsed.Seconds(); secs > 0 {
			p.FPS = float64(p.Frame) / secs
		}
	}
	pw.onProgress(p)
}

// MuxSeedVR2Output joins the video SeedVR2 produced with every other track of
// the source (audio, subtitles, and on Matroska the attachments such as
// fonts), plus its chapters and metadata, into dst without re-encoding.
func (r *Runner) MuxSeedVR2Output(ctx context.Context, videoPath, sourcePath, dst string, onProgress func(Progress)) error {
	f, err := os.OpenFile(r.cfg.BaseDir+"/ffmpeg.log", os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return fmt.Errorf("open ffmpeg log: %w", err)
	}
	defer f.Close()

	cmd := exec.CommandContext(ctx, r.cfg.FFmpegBin, seedVR2MuxArgs(videoPath, sourcePath, dst)...)
	tail := newTailWriter(20)
	out := io.MultiWriter(f, tail)
	if onProgress != nil {
		out = io.MultiWriter(newFFmpegProgressWriter(f, onProgress, "Mux"), tail)
	}
	cmd.Stdout = out
	cmd.Stderr = out

	label := ProcessPrefix + "ffmpeg-mux-" + ephemeralSuffix()
	registerForCtx(ctx, label, cmd)
	defer tracker.unregister(label)
	return runError(cmd.Run(), tail)
}

func seedVR2MuxArgs(videoPath, sourcePath, dst string) []string {
	args := []string{
		"-y", "-progress", "pipe:2", "-nostats",
		"-i", videoPath,
		"-i", sourcePath,
		"-map", "0:v:0", "-map", "1:a?", "-map", "1:s?",
	}
	if strings.EqualFold(filepath.Ext(dst), ".mkv") {
		args = append(args, "-map", "1:t?")
	}
	return append(args,
		"-map_metadata", "1", "-map_chapters", "1",
		"-c", "copy",
		dst,
	)
}
