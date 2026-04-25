package runner

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"anime-upscaling/internal/config"
)

// needsSanitize returns true if the filename contains characters that
// video2x cannot handle (e.g. spaces).
func needsSanitize(filename string) bool {
	return strings.ContainsAny(filename, " ")
}

// sanitizeFilename replaces problematic characters with underscores and
// appends an ephemeral suffix before the extension to avoid collisions.
func sanitizeFilename(filename string) string {
	ext := filepath.Ext(filename)
	base := strings.TrimSuffix(filename, ext)
	safe := strings.ReplaceAll(base, " ", "_")
	return safe + "_" + ephemeralSuffix() + ext
}

// setupSanitizedPaths creates a symlink with a sanitized name for the input
// file if the filename needs sanitization. Returns the paths to use for
// the command and a cleanup function that must be deferred.
func setupSanitizedPaths(filename, inputDir, outputDir string) (cmdInputPath, cmdOutputPath, originalOutputPath string, sanitized bool, cleanup func()) {
	originalInputPath := inputDir + "/" + filename
	originalOutputPath = outputDir + "/" + filename

	if !needsSanitize(filename) {
		return originalInputPath, originalOutputPath, originalOutputPath, false, func() {}
	}

	safeName := sanitizeFilename(filename)
	linkPath := inputDir + "/" + safeName
	cmdInputPath = linkPath
	cmdOutputPath = outputDir + "/" + safeName

	// Create hard link so video2x sees a clean name with no reference to the original path.
	// Hard links are preferred over symlinks because some tools resolve symlinks
	// and may still fail on the original path containing spaces.
	os.Link(originalInputPath, linkPath)

	sanitized = true
	cleanup = func() {
		os.Remove(linkPath)
	}
	return
}

// finalizeSanitizedOutput renames the output from the sanitized name to the
// original name after successful processing. On failure, cleans up partial output.
func finalizeSanitizedOutput(cmdOutputPath, originalOutputPath string, err error) {
	if err != nil {
		os.Remove(cmdOutputPath)
		return
	}
	os.Rename(cmdOutputPath, originalOutputPath)
}

// runError wraps an exec error with the last lines of process output for context.
func runError(err error, tail *tailWriter) error {
	if err == nil {
		return nil
	}
	if detail := tail.LastLines(); detail != "" {
		return fmt.Errorf("%w: %s", err, detail)
	}
	return err
}

// SignalFromError extracts the termination signal from an exec error, if any.
// Returns (signal, true) when the process was killed by a signal (e.g. SIGSEGV);
// returns (0, false) for normal non-zero exits or non-exec errors.
func SignalFromError(err error) (syscall.Signal, bool) {
	if err == nil {
		return 0, false
	}
	var exitErr *exec.ExitError
	if !errors.As(err, &exitErr) {
		return 0, false
	}
	ws, ok := exitErr.Sys().(syscall.WaitStatus)
	if !ok {
		return 0, false
	}
	if !ws.Signaled() {
		return 0, false
	}
	return ws.Signal(), true
}

const ProcessPrefix = "anime-upscaling-"

func ephemeralSuffix() string {
	return strconv.FormatInt(time.Now().UnixNano(), 36)
}

type jobIDCtxKey struct{}

// WithJobID returns ctx tagged with jobID so that downstream tracker
// registrations record which job owns the spawned process. The API server
// sets this on every job context; callers without a job (CLI subcommands)
// pass plain contexts and registrations stay anonymous.
func WithJobID(ctx context.Context, jobID string) context.Context {
	if jobID == "" {
		return ctx
	}
	return context.WithValue(ctx, jobIDCtxKey{}, jobID)
}

// JobIDFromContext returns the jobID set via WithJobID, or "" if none.
func JobIDFromContext(ctx context.Context) string {
	v, _ := ctx.Value(jobIDCtxKey{}).(string)
	return v
}

func registerForCtx(ctx context.Context, label string, cmd *exec.Cmd) {
	if id := JobIDFromContext(ctx); id != "" {
		tracker.registerForJob(label, cmd, id)
	} else {
		tracker.register(label, cmd)
	}
}

type Runner struct {
	cfg config.Config
}

func NewRunner(cfg config.Config) *Runner {
	return &Runner{cfg: cfg}
}

// UpscaleOptions holds processor and model parameters for video2x upscaling.
type UpscaleOptions struct {
	Processor  string // "realesrgan" (default), "libplacebo", "realcugan"
	Model      string // model/shader name (depends on processor)
	NoiseLevel int    // 0=off, 1-3=noise reduction level
}

// WithDefaults returns a copy with zero-value fields replaced by defaults.
func (o UpscaleOptions) WithDefaults() UpscaleOptions {
	if o.Processor == "" {
		o.Processor = "realesrgan"
	}
	if o.Model == "" {
		switch o.Processor {
		case "realesrgan":
			o.Model = "realesr-animevideov3"
		case "libplacebo":
			o.Model = "anime4k-v4-a"
		case "realcugan":
			o.Model = "models-se"
		}
	}
	return o
}

// Video2x runs video2x upscale on a specific GPU, writing stdout/stderr to logPath.
// If onProgress is non-nil, the log output is also parsed for progress data.
// streamIdx disambiguates concurrent streams running on the same GPU for tracker
// labeling; callers should pass a unique streamIdx per in-flight invocation on gpuID.
func (r *Runner) Video2x(ctx context.Context, gpuID, streamIdx int, filename, logPath string, scale int, opts UpscaleOptions, inputDir, outputDir string, onProgress func(Progress)) error {
	f, err := os.Create(logPath)
	if err != nil {
		return fmt.Errorf("create log: %w", err)
	}
	defer f.Close()

	opts = opts.WithDefaults()

	cmdInputPath, cmdOutputPath, originalOutputPath, sanitized, cleanupSymlink := setupSanitizedPaths(filename, inputDir, outputDir)
	defer cleanupSymlink()

	args := []string{
		"-i", cmdInputPath,
		"-o", cmdOutputPath,
		"-p", opts.Processor,
		"-s", strconv.Itoa(scale),
		"-d", strconv.Itoa(gpuID),
	}
	switch opts.Processor {
	case "realesrgan":
		args = append(args, "--realesrgan-model", opts.Model)
	case "libplacebo":
		args = append(args, "--libplacebo-shader", opts.Model)
	case "realcugan":
		args = append(args, "--realcugan-model", opts.Model)
	}
	if opts.NoiseLevel > 0 {
		args = append(args, "-n", strconv.Itoa(opts.NoiseLevel))
	}

	cmd := exec.CommandContext(ctx, r.cfg.Video2xBin, args...)

	tail := newTailWriter(20)
	var out io.Writer = io.MultiWriter(f, tail)
	if onProgress != nil {
		out = io.MultiWriter(newProgressWriter(f, onProgress), tail)
	}
	cmd.Stdout = out
	cmd.Stderr = out

	label := fmt.Sprintf("%svideo2x-gpu%d-s%d", ProcessPrefix, gpuID, streamIdx)
	registerForCtx(ctx, label, cmd)
	defer tracker.unregister(label)

	err = cmd.Run()
	if sanitized {
		finalizeSanitizedOutput(cmdOutputPath, originalOutputPath, err)
	}
	return runError(err, tail)
}

// RifeOptions holds quality-related options for RIFE frame interpolation.
type RifeOptions struct {
	Model       string  // RIFE model name (e.g. "rife-v4.6")
	SceneThresh float64 // Scene detection threshold 0-100 (0=very sensitive, 100=off)
	UHD         bool    // Enable Ultra HD mode
}

// Video2xRife runs RIFE frame interpolation on a specific GPU.
// streamIdx disambiguates concurrent streams running on the same GPU.
func (r *Runner) Video2xRife(ctx context.Context, gpuID, streamIdx int, filename, logPath string, multiplier int, opts RifeOptions, inputDir, outputDir string, onProgress func(Progress)) error {
	f, err := os.Create(logPath)
	if err != nil {
		return fmt.Errorf("create log: %w", err)
	}
	defer f.Close()

	cmdInputPath, cmdOutputPath, originalOutputPath, sanitized, cleanupSymlink := setupSanitizedPaths(filename, inputDir, outputDir)
	defer cleanupSymlink()

	args := []string{
		"-i", cmdInputPath,
		"-o", cmdOutputPath,
		"-p", "rife",
		"-m", strconv.Itoa(multiplier),
		"-d", strconv.Itoa(gpuID),
	}
	if opts.Model != "" {
		args = append(args, "--rife-model", opts.Model)
	}
	if opts.UHD {
		args = append(args, "--rife-uhd")
	}
	args = append(args, "--scene-thresh", fmt.Sprintf("%.1f", opts.SceneThresh))

	cmd := exec.CommandContext(ctx, r.cfg.Video2xBin, args...)

	tail := newTailWriter(20)
	var out io.Writer = io.MultiWriter(f, tail)
	if onProgress != nil {
		out = io.MultiWriter(newProgressWriter(f, onProgress), tail)
	}
	cmd.Stdout = out
	cmd.Stderr = out

	label := fmt.Sprintf("%svideo2x-rife-gpu%d-s%d", ProcessPrefix, gpuID, streamIdx)
	registerForCtx(ctx, label, cmd)
	defer tracker.unregister(label)

	err = cmd.Run()
	if sanitized {
		finalizeSanitizedOutput(cmdOutputPath, originalOutputPath, err)
	}
	return runError(err, tail)
}

// EncodeOptions holds codec and encoding parameters for FFmpeg.
type EncodeOptions struct {
	Codec      string   // "libx265" (default), "libx264", "libvpx-vp9", "copy"
	Preset     string   // "fast" (default), "ultrafast"..."veryslow"
	Tune       string   // "animation" (default), "film", "grain", "zerolatency", "none" (no tune)
	PixFmt     string   // "yuv420p10le" (default), "yuv420p", "yuv444p"
	AudioCodec string   // "copy" (default), "aac", "libopus", "libmp3lame"
	ExtraArgs  []string // extra ffmpeg args appended before the output path (e.g. -x265-params pools=none)

	// GPU acceleration. When UseGPU=true and GPUVendor is set, the runner maps
	// the logical Codec ("libx265"/"libx264") to the vendor-specific encoder
	// (hevc_nvenc, hevc_amf, hevc_qsv, ...) and builds -hwaccel args.
	UseGPU    bool
	GPUVendor string // "nvidia" | "amd" | "intel"
	GPUDevice int    // device index for -hwaccel_device
}

// WithDefaults returns a copy with zero-value fields replaced by defaults.
func (o EncodeOptions) WithDefaults() EncodeOptions {
	if o.Codec == "" {
		o.Codec = "libx265"
	}
	if o.Preset == "" {
		o.Preset = "fast"
	}
	if o.Tune == "" {
		o.Tune = "animation"
	}
	if o.PixFmt == "" {
		o.PixFmt = "yuv420p10le"
	}
	if o.AudioCodec == "" {
		o.AudioCodec = "copy"
	}
	return o
}

// FFmpegEncode encodes a video with configurable codec and settings.
// If onProgress is non-nil, stderr/stdout are intercepted to parse progress data.
func (r *Runner) FFmpegEncode(ctx context.Context, inputRelPath, outputRelPath string, crf int, threads int, opts EncodeOptions, processName string, copySubtitles bool, scaleDivisor int, onProgress func(Progress)) error {
	f, err := os.OpenFile(r.cfg.BaseDir+"/ffmpeg.log", os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return fmt.Errorf("open ffmpeg log: %w", err)
	}
	defer f.Close()

	inputPath := r.cfg.BaseDir + "/" + inputRelPath
	outputPath := r.cfg.BaseDir + "/" + outputRelPath

	opts = opts.WithDefaults()
	useGPU := opts.UseGPU && opts.Codec != "copy" && gpuEncoderFor(opts.Codec, opts.GPUVendor) != ""

	// -progress pipe:2 emits machine-readable key=value lines (frame=N, fps=N,
	// out_time_us=N, speed=Nx, progress=continue|end) to stderr roughly every
	// 500ms. -nostats suppresses the legacy \r-delimited status line so the
	// progress parser only sees one format.
	args := []string{"-progress", "pipe:2", "-nostats"}

	if useGPU && opts.GPUVendor == "nvidia" {
		// Intentionally NOT using -hwaccel_output_format cuda: forcing frames to
		// stay in CUDA memory requires every downstream filter/format conversion
		// to have a CUDA implementation, which triggers ENOSYS (-38) when the
		// input bit depth differs from the encoder's -pix_fmt (e.g. 8-bit yuv420p
		// input → p010le NVENC output). Letting decoded frames land in CPU memory
		// lets NVENC upload + convert them internally. Encode still runs on GPU.
		args = append(args,
			"-hwaccel", "cuda",
			"-hwaccel_device", strconv.Itoa(opts.GPUDevice),
		)
	}

	args = append(args, "-i", inputPath)

	if copySubtitles {
		args = append(args, "-map", "0")
	}
	if scaleDivisor > 1 && opts.Codec != "copy" {
		args = append(args, "-vf", fmt.Sprintf("scale=iw/%d:ih/%d", scaleDivisor, scaleDivisor))
	}

	if opts.Codec == "copy" {
		args = append(args, "-c:v", "copy")
	} else if useGPU {
		args = append(args, buildGPUEncodeArgs(opts, crf)...)
	} else {
		args = append(args, "-c:v", opts.Codec)
		if opts.Codec != "libvpx-vp9" {
			args = append(args, "-preset", opts.Preset)
		}
		args = append(args, "-crf", strconv.Itoa(crf))
		if opts.Tune != "none" && opts.Codec != "libvpx-vp9" {
			args = append(args, "-tune", opts.Tune)
		}
		args = append(args, "-pix_fmt", opts.PixFmt)
		args = append(args, "-threads", strconv.Itoa(threads))
	}
	args = append(args, "-c:a", opts.AudioCodec)

	if copySubtitles {
		args = append(args, "-c:s", "copy")
	}
	if len(opts.ExtraArgs) > 0 {
		args = append(args, opts.ExtraArgs...)
	}
	args = append(args, outputPath)

	cmd := exec.CommandContext(ctx, r.cfg.FFmpegBin, args...)

	tail := newTailWriter(20)
	var out io.Writer = io.MultiWriter(f, tail)
	if onProgress != nil {
		out = io.MultiWriter(newFFmpegProgressWriter(f, onProgress, "Encode"), tail)
	}
	cmd.Stdout = out
	cmd.Stderr = out

	label := ProcessPrefix + "ffmpeg-encode-" + ephemeralSuffix()
	if processName != "" {
		label = ProcessPrefix + processName + "-" + ephemeralSuffix()
	}
	registerForCtx(ctx, label, cmd)
	defer tracker.unregister(label)

	return runError(cmd.Run(), tail)
}

// gpuEncoderFor maps a logical CPU codec + vendor to the corresponding hardware
// encoder name. Returns "" when the combination is not supported (caller falls
// back to the CPU branch).
func gpuEncoderFor(codec, vendor string) string {
	switch vendor {
	case "nvidia":
		switch codec {
		case "libx265":
			return "hevc_nvenc"
		case "libx264":
			return "h264_nvenc"
		}
	case "amd":
		switch codec {
		case "libx265":
			return "hevc_amf"
		case "libx264":
			return "h264_amf"
		}
	case "intel":
		switch codec {
		case "libx265":
			return "hevc_qsv"
		case "libx264":
			return "h264_qsv"
		}
	}
	return ""
}

// gpuPixFmt maps software pixel formats to the GPU-side equivalent used by the
// NVENC/AMF/QSV encoders. NVENC only accepts p010le (10-bit) or nv12 (8-bit);
// yuv444p is not supported so it downgrades to p010le with a warning in logs.
func gpuPixFmt(pixfmt, vendor string) string {
	if vendor != "nvidia" {
		return pixfmt
	}
	switch pixfmt {
	case "yuv420p10le":
		return "p010le"
	case "yuv420p":
		return "nv12"
	case "yuv444p":
		// NVENC doesn't support yuv444 — closest 10-bit layout.
		return "p010le"
	}
	return pixfmt
}

// buildGPUEncodeArgs builds the -c:v and vendor-specific rate-control args for
// hardware-accelerated encoding. Mirrors the validated commands from the plan:
//
//	NVIDIA: -c:v hevc_nvenc -preset p6 -tune hq -rc vbr -cq <CRF> -b:v 0
//	        -pix_fmt p010le -bf 4 -spatial-aq 1
//	AMD:    -c:v hevc_amf -quality quality -rc vbr_latency
//	INTEL:  -c:v hevc_qsv -preset veryslow -global_quality <CRF>
func buildGPUEncodeArgs(opts EncodeOptions, crf int) []string {
	encoder := gpuEncoderFor(opts.Codec, opts.GPUVendor)
	args := []string{"-c:v", encoder}

	switch opts.GPUVendor {
	case "nvidia":
		args = append(args,
			"-preset", "p6",
			"-tune", "hq",
			"-rc", "vbr",
			"-cq", strconv.Itoa(crf),
			"-b:v", "0",
			"-pix_fmt", gpuPixFmt(opts.PixFmt, opts.GPUVendor),
			"-bf", "4",
			"-spatial-aq", "1",
		)
	case "amd":
		args = append(args,
			"-quality", "quality",
			"-rc", "vbr_latency",
		)
	case "intel":
		args = append(args,
			"-preset", "veryslow",
			"-global_quality", strconv.Itoa(crf),
		)
	}
	return args
}

// ProbeFrameCount returns the total number of video frames in a file. Tries
// five strategies cheapest first: (1) stream.nb_frames (MP4/MOV carry this;
// MKV doesn't), (2) the MKV stream tag NUMBER_OF_FRAMES, (3) the MKV stream
// tag DURATION × r_frame_rate, (4) format.duration × r_frame_rate (works for
// any container), (5) stream.duration × r_frame_rate. Returns 0 with no error
// when all strategies yield nothing usable.
func (r *Runner) ProbeFrameCount(ctx context.Context, absPath string) (int, error) {
	probeStream := func(entries string) string {
		return probeFFprobe(ctx, r.cfg.FFprobeBin, absPath, []string{"-select_streams", "v:0"}, entries)
	}
	probeFormat := func(entries string) string {
		return probeFFprobe(ctx, r.cfg.FFprobeBin, absPath, nil, entries)
	}

	// 1. stream.nb_frames (set by MP4/MOV, not MKV)
	if n, _ := strconv.Atoi(probeStream("stream=nb_frames")); n > 0 {
		return n, nil
	}

	// 2. MKV stream tag NUMBER_OF_FRAMES
	if n, _ := strconv.Atoi(probeStream("stream_tags=NUMBER_OF_FRAMES")); n > 0 {
		return n, nil
	}

	rate := probeStream("stream=r_frame_rate")
	fps := parseRational(rate)
	if fps <= 0 {
		return 0, nil
	}

	compute := func(dur string) int {
		seconds, err := strconv.ParseFloat(dur, 64)
		if err != nil || seconds <= 0 {
			return 0
		}
		return int(seconds*fps + 0.5)
	}

	// 3. MKV stream tag DURATION (e.g. "00:24:12.345000000")
	if n := compute(parseMKVDuration(probeStream("stream_tags=DURATION"))); n > 0 {
		return n, nil
	}

	// 4. format.duration (container-level; MKV always has this)
	if n := compute(probeFormat("format=duration")); n > 0 {
		return n, nil
	}

	// 5. stream.duration (MP4/MOV fallback)
	if n := compute(probeStream("stream=duration")); n > 0 {
		return n, nil
	}

	return 0, nil
}

// probeFFprobe runs ffprobe with the given selector + entries and returns
// trimmed stdout, or "" on error. Uses default flat output with nokey so lines
// are bare values.
func probeFFprobe(ctx context.Context, bin, absPath string, selector []string, entries string) string {
	args := []string{"-v", "error"}
	args = append(args, selector...)
	args = append(args, "-show_entries", entries, "-of", "default=noprint_wrappers=1:nokey=1", absPath)
	var buf bytes.Buffer
	cmd := exec.CommandContext(ctx, bin, args...)
	cmd.Stdout = &buf
	cmd.Stderr = &buf
	if err := cmd.Run(); err != nil {
		return ""
	}
	return strings.TrimSpace(buf.String())
}

// parseMKVDuration converts the MKV DURATION tag format "HH:MM:SS.fff" to
// a decimal seconds string, or returns the input unchanged if it doesn't match.
func parseMKVDuration(s string) string {
	parts := strings.Split(s, ":")
	if len(parts) != 3 {
		return s
	}
	h, err1 := strconv.ParseFloat(parts[0], 64)
	m, err2 := strconv.ParseFloat(parts[1], 64)
	sec, err3 := strconv.ParseFloat(parts[2], 64)
	if err1 != nil || err2 != nil || err3 != nil {
		return s
	}
	return strconv.FormatFloat(h*3600+m*60+sec, 'f', -1, 64)
}

// parseRational parses ffprobe rational strings like "24000/1001" or "30/1".
// Returns 0 on any malformed input.
func parseRational(s string) float64 {
	parts := strings.SplitN(s, "/", 2)
	if len(parts) != 2 {
		return 0
	}
	num, err1 := strconv.ParseFloat(strings.TrimSpace(parts[0]), 64)
	den, err2 := strconv.ParseFloat(strings.TrimSpace(parts[1]), 64)
	if err1 != nil || err2 != nil || den == 0 {
		return 0
	}
	return num / den
}

// FFprobe runs ffprobe on a file, returns stdout+stderr combined.
func (r *Runner) FFprobe(ctx context.Context, relPath string) (string, error) {
	absPath := r.cfg.BaseDir + "/" + relPath
	var buf bytes.Buffer
	cmd := exec.CommandContext(ctx, r.cfg.FFprobeBin,
		"-v", "error",
		"-show_entries", "stream=codec_type",
		"-of", "csv=p=0",
		absPath,
	)
	cmd.Stdout = &buf
	cmd.Stderr = &buf
	err := cmd.Run()
	return buf.String(), err
}

// FFmpegDecode does a full decode pass to check integrity.
// If onProgress is non-nil, stderr is parsed for progress data.
func (r *Runner) FFmpegDecode(ctx context.Context, relPath string, processName string, onProgress func(Progress)) (string, error) {
	absPath := r.cfg.BaseDir + "/" + relPath
	phase := "Check"
	if processName == "precheck-optimize" {
		phase = "Pre-check"
	}

	cmd := exec.CommandContext(ctx, r.cfg.FFmpegBin,
		"-progress", "pipe:2", "-nostats", "-v", "error",
		"-i", absPath,
		"-f", "null",
		"-",
	)

	var errBuf bytes.Buffer
	var out io.Writer = &errBuf
	if onProgress != nil {
		out = io.MultiWriter(&errBuf, newFFmpegProgressWriter(io.Discard, onProgress, phase))
	}
	cmd.Stdout = out
	cmd.Stderr = out

	label := ProcessPrefix + "ffmpeg-decode-" + ephemeralSuffix()
	if processName != "" {
		label = ProcessPrefix + "ffmpeg-" + processName + "-" + ephemeralSuffix()
	}
	registerForCtx(ctx, label, cmd)
	defer tracker.unregister(label)

	err := cmd.Run()
	return errBuf.String(), err
}

// Chown fixes file permissions.
func (r *Runner) Chown(ctx context.Context, dir, filename string) error {
	return os.Chown(dir+"/"+filename, r.cfg.UserID, r.cfg.GroupID)
}

// ProbeResolution returns the resolution of a single video file.
func (r *Runner) ProbeResolution(ctx context.Context, absPath string) (VideoResolution, error) {
	var buf bytes.Buffer
	cmd := exec.CommandContext(ctx, r.cfg.FFprobeBin,
		"-v", "error",
		"-select_streams", "v:0",
		"-show_entries", "stream=width,height",
		"-of", "csv=p=0",
		absPath,
	)
	cmd.Stdout = &buf
	cmd.Stderr = &buf
	if err := cmd.Run(); err != nil {
		return VideoResolution{}, fmt.Errorf("ffprobe: %w", err)
	}
	dims := strings.SplitN(strings.TrimSpace(buf.String()), ",", 2)
	if len(dims) != 2 {
		return VideoResolution{}, fmt.Errorf("unexpected ffprobe output: %s", buf.String())
	}
	w, err1 := strconv.Atoi(strings.TrimSpace(dims[0]))
	h, err2 := strconv.Atoi(strings.TrimSpace(dims[1]))
	if err1 != nil || err2 != nil {
		return VideoResolution{}, fmt.Errorf("parse resolution: %v / %v", err1, err2)
	}
	return VideoResolution{Width: w, Height: h}, nil
}

// VideoResolution holds the width and height of a video stream.
type VideoResolution struct {
	Width  int
	Height int
}

// AudioTrack holds metadata for an audio stream.
type AudioTrack struct {
	Index    int    `json:"index"`
	Language string `json:"language,omitempty"`
	Title    string `json:"title,omitempty"`
	Codec    string `json:"codec,omitempty"`
	Channels int    `json:"channels,omitempty"`
}

// SubtitleTrack holds metadata for a subtitle stream.
type SubtitleTrack struct {
	Index    int    `json:"index"`
	Language string `json:"language,omitempty"`
	Title    string `json:"title,omitempty"`
	Codec    string `json:"codec,omitempty"`
}

// VideoProbeResult holds full metadata from ffprobe.
type VideoProbeResult struct {
	Width     int
	Height    int
	Audio     []AudioTrack
	Subtitles []SubtitleTrack
}

// ffprobeJSON mirrors the JSON output of ffprobe -show_entries stream=... -of json
type ffprobeJSON struct {
	Streams []ffprobeStream `json:"streams"`
}

type ffprobeStream struct {
	Index     int               `json:"index"`
	CodecType string            `json:"codec_type"`
	CodecName string            `json:"codec_name"`
	Width     int               `json:"width,omitempty"`
	Height    int               `json:"height,omitempty"`
	Channels  int               `json:"channels,omitempty"`
	Tags      map[string]string `json:"tags,omitempty"`
}

// ProbeFullMetadata returns resolution + audio/subtitle track info for a single file.
func (r *Runner) ProbeFullMetadata(ctx context.Context, absPath string) (VideoProbeResult, error) {
	var buf bytes.Buffer
	cmd := exec.CommandContext(ctx, r.cfg.FFprobeBin,
		"-v", "error",
		"-show_entries", "stream=index,codec_type,codec_name,width,height,channels",
		"-show_entries", "stream_tags=language,title",
		"-of", "json",
		absPath,
	)
	cmd.Stdout = &buf
	cmd.Stderr = &buf
	if err := cmd.Run(); err != nil {
		return VideoProbeResult{}, fmt.Errorf("ffprobe: %w", err)
	}

	var probe ffprobeJSON
	if err := json.Unmarshal(buf.Bytes(), &probe); err != nil {
		return VideoProbeResult{}, fmt.Errorf("parse ffprobe json: %w", err)
	}

	var result VideoProbeResult
	for _, s := range probe.Streams {
		switch s.CodecType {
		case "video":
			if result.Width == 0 && result.Height == 0 {
				result.Width = s.Width
				result.Height = s.Height
			}
		case "audio":
			result.Audio = append(result.Audio, AudioTrack{
				Index:    s.Index,
				Language: s.Tags["language"],
				Title:    s.Tags["title"],
				Codec:    s.CodecName,
				Channels: s.Channels,
			})
		case "subtitle":
			result.Subtitles = append(result.Subtitles, SubtitleTrack{
				Index:    s.Index,
				Language: s.Tags["language"],
				Title:    s.Tags["title"],
				Codec:    s.CodecName,
			})
		}
	}
	return result, nil
}

// FFprobeBatchFullMetadataParallel probes all files across multiple directories
// for full metadata (resolution + audio + subtitles), running up to 8 concurrently.
func (r *Runner) FFprobeBatchFullMetadataParallel(ctx context.Context, mounts []DirMount, filesByLabel map[string][]string) (map[string]map[string]VideoProbeResult, error) {
	total := 0
	for _, files := range filesByLabel {
		total += len(files)
	}
	if total == 0 {
		return nil, nil
	}

	type probeJob struct {
		label   string
		hostDir string
		file    string
	}
	var jobs []probeJob
	for _, m := range mounts {
		for _, f := range filesByLabel[m.Label] {
			jobs = append(jobs, probeJob{label: m.Label, hostDir: m.HostDir, file: f})
		}
	}

	type probeResult struct {
		label string
		file  string
		res   VideoProbeResult
	}

	var (
		mu      sync.Mutex
		results []probeResult
		wg      sync.WaitGroup
		sem     = make(chan struct{}, 8)
	)

	for _, j := range jobs {
		if ctx.Err() != nil {
			break
		}
		wg.Add(1)
		go func(j probeJob) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			if ctx.Err() != nil {
				return
			}

			absPath := j.hostDir + "/" + j.file
			res, err := r.ProbeFullMetadata(ctx, absPath)
			if err != nil {
				return
			}

			mu.Lock()
			results = append(results, probeResult{label: j.label, file: j.file, res: res})
			mu.Unlock()
		}(j)
	}
	wg.Wait()

	out := make(map[string]map[string]VideoProbeResult)
	for _, r := range results {
		if out[r.label] == nil {
			out[r.label] = make(map[string]VideoProbeResult)
		}
		out[r.label][r.file] = r.res
	}
	return out, ctx.Err()
}

// DirMount represents a directory for batch ffprobe operations.
type DirMount struct {
	Label   string // "input", "output", "optimized", "source"
	HostDir string
}

type resolutionCacheEntry struct {
	data     map[string]VideoResolution
	cachedAt time.Time
}

const resolutionCacheTTL = 20 * time.Minute

var (
	resolutionCache   = map[string]resolutionCacheEntry{}
	resolutionCacheMu sync.Mutex
)

// FFprobeBatchResolutionMultiDir probes all specified files across multiple
// directories. Returns label -> filename -> resolution.
func (r *Runner) FFprobeBatchResolutionMultiDir(ctx context.Context, mounts []DirMount, filesByLabel map[string][]string) (map[string]map[string]VideoResolution, error) {
	total := 0
	for _, files := range filesByLabel {
		total += len(files)
	}
	if total == 0 {
		return nil, nil
	}

	result := make(map[string]map[string]VideoResolution)

	for _, m := range mounts {
		files := filesByLabel[m.Label]
		for _, f := range files {
			if ctx.Err() != nil {
				return result, ctx.Err()
			}
			absPath := m.HostDir + "/" + f
			var buf bytes.Buffer
			cmd := exec.CommandContext(ctx, r.cfg.FFprobeBin,
				"-v", "error",
				"-select_streams", "v:0",
				"-show_entries", "stream=width,height",
				"-of", "csv=p=0",
				absPath,
			)
			cmd.Stdout = &buf
			cmd.Stderr = &buf
			if err := cmd.Run(); err != nil {
				continue
			}

			dims := strings.SplitN(strings.TrimSpace(buf.String()), ",", 2)
			if len(dims) != 2 {
				continue
			}
			w, err1 := strconv.Atoi(strings.TrimSpace(dims[0]))
			h, err2 := strconv.Atoi(strings.TrimSpace(dims[1]))
			if err1 != nil || err2 != nil {
				continue
			}
			if result[m.Label] == nil {
				result[m.Label] = make(map[string]VideoResolution)
			}
			result[m.Label][f] = VideoResolution{Width: w, Height: h}
		}
	}
	return result, nil
}

// FFprobeBatchResolutionMultiDirParallel is like FFprobeBatchResolutionMultiDir
// but runs up to 8 ffprobe processes concurrently.
func (r *Runner) FFprobeBatchResolutionMultiDirParallel(ctx context.Context, mounts []DirMount, filesByLabel map[string][]string) (map[string]map[string]VideoResolution, error) {
	total := 0
	for _, files := range filesByLabel {
		total += len(files)
	}
	if total == 0 {
		return nil, nil
	}

	type probeJob struct {
		label   string
		hostDir string
		file    string
	}
	var jobs []probeJob
	for _, m := range mounts {
		for _, f := range filesByLabel[m.Label] {
			jobs = append(jobs, probeJob{label: m.Label, hostDir: m.HostDir, file: f})
		}
	}

	type probeResult struct {
		label string
		file  string
		res   VideoResolution
	}

	var (
		mu      sync.Mutex
		results []probeResult
		wg      sync.WaitGroup
		sem     = make(chan struct{}, 8)
	)

	for _, j := range jobs {
		if ctx.Err() != nil {
			break
		}
		wg.Add(1)
		go func(j probeJob) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			if ctx.Err() != nil {
				return
			}

			absPath := j.hostDir + "/" + j.file
			var buf bytes.Buffer
			cmd := exec.CommandContext(ctx, r.cfg.FFprobeBin,
				"-v", "error",
				"-select_streams", "v:0",
				"-show_entries", "stream=width,height",
				"-of", "csv=p=0",
				absPath,
			)
			cmd.Stdout = &buf
			cmd.Stderr = &buf
			if err := cmd.Run(); err != nil {
				return
			}

			dims := strings.SplitN(strings.TrimSpace(buf.String()), ",", 2)
			if len(dims) != 2 {
				return
			}
			w, err1 := strconv.Atoi(strings.TrimSpace(dims[0]))
			h, err2 := strconv.Atoi(strings.TrimSpace(dims[1]))
			if err1 != nil || err2 != nil {
				return
			}

			mu.Lock()
			results = append(results, probeResult{label: j.label, file: j.file, res: VideoResolution{Width: w, Height: h}})
			mu.Unlock()
		}(j)
	}
	wg.Wait()

	out := make(map[string]map[string]VideoResolution)
	for _, r := range results {
		if out[r.label] == nil {
			out[r.label] = make(map[string]VideoResolution)
		}
		out[r.label][r.file] = r.res
	}
	return out, ctx.Err()
}

// FFprobeBatchResolutionMultiDirCached wraps FFprobeBatchResolutionMultiDir with
// a per-directory in-memory cache (20-minute TTL). Returns results + cachedAt.
func (r *Runner) FFprobeBatchResolutionMultiDirCached(ctx context.Context, mounts []DirMount, filesByLabel map[string][]string, forceRefresh bool) (map[string]map[string]VideoResolution, time.Time, error) {
	now := time.Now()

	if !forceRefresh {
		resolutionCacheMu.Lock()
		allCached := true
		var oldestCachedAt time.Time
		result := make(map[string]map[string]VideoResolution)
		for _, m := range mounts {
			entry, ok := resolutionCache[m.HostDir]
			if !ok || now.Sub(entry.cachedAt) > resolutionCacheTTL {
				allCached = false
				break
			}
			if oldestCachedAt.IsZero() || entry.cachedAt.Before(oldestCachedAt) {
				oldestCachedAt = entry.cachedAt
			}
			result[m.Label] = entry.data
		}
		resolutionCacheMu.Unlock()

		if allCached {
			return result, oldestCachedAt, nil
		}
	}

	data, err := r.FFprobeBatchResolutionMultiDir(ctx, mounts, filesByLabel)
	if err != nil {
		return nil, time.Time{}, err
	}

	cachedAt := time.Now()

	resolutionCacheMu.Lock()
	for _, m := range mounts {
		dirData := data[m.Label]
		if dirData == nil {
			dirData = make(map[string]VideoResolution)
		}
		resolutionCache[m.HostDir] = resolutionCacheEntry{
			data:     dirData,
			cachedAt: cachedAt,
		}
	}
	resolutionCacheMu.Unlock()

	return data, cachedAt, nil
}
