package runner

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"anime-upscaling/internal/config"
)

const ProcessPrefix = "anime-upscaling-"

func ephemeralSuffix() string {
	return strconv.FormatInt(time.Now().UnixNano(), 36)
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
func (r *Runner) Video2x(ctx context.Context, gpuID int, filename, logPath string, scale int, opts UpscaleOptions, inputDir, outputDir string, onProgress func(Progress)) error {
	f, err := os.Create(logPath)
	if err != nil {
		return fmt.Errorf("create log: %w", err)
	}
	defer f.Close()

	opts = opts.WithDefaults()
	inputPath := inputDir + "/" + filename
	outputPath := outputDir + "/" + filename

	args := []string{
		"-i", inputPath,
		"-o", outputPath,
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

	var out io.Writer = f
	if onProgress != nil {
		out = newProgressWriter(f, onProgress)
	}
	cmd.Stdout = out
	cmd.Stderr = out

	label := fmt.Sprintf("%svideo2x-gpu%d", ProcessPrefix, gpuID)
	tracker.register(label, cmd)
	defer tracker.unregister(label)

	return cmd.Run()
}

// RifeOptions holds quality-related options for RIFE frame interpolation.
type RifeOptions struct {
	Model       string  // RIFE model name (e.g. "rife-v4.6")
	SceneThresh float64 // Scene detection threshold 0-100 (0=very sensitive, 100=off)
	UHD         bool    // Enable Ultra HD mode
}

// Video2xRife runs RIFE frame interpolation on a specific GPU.
func (r *Runner) Video2xRife(ctx context.Context, gpuID int, filename, logPath string, multiplier int, opts RifeOptions, inputDir, outputDir string, onProgress func(Progress)) error {
	f, err := os.Create(logPath)
	if err != nil {
		return fmt.Errorf("create log: %w", err)
	}
	defer f.Close()

	inputPath := inputDir + "/" + filename
	outputPath := outputDir + "/" + filename

	args := []string{
		"-i", inputPath,
		"-o", outputPath,
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

	var out io.Writer = f
	if onProgress != nil {
		out = newProgressWriter(f, onProgress)
	}
	cmd.Stdout = out
	cmd.Stderr = out

	label := fmt.Sprintf("%svideo2x-rife-gpu%d", ProcessPrefix, gpuID)
	tracker.register(label, cmd)
	defer tracker.unregister(label)

	return cmd.Run()
}

// EncodeOptions holds codec and encoding parameters for FFmpeg.
type EncodeOptions struct {
	Codec      string // "libx265" (default), "libx264", "libvpx-vp9", "copy"
	Preset     string // "fast" (default), "ultrafast"..."veryslow"
	Tune       string // "animation" (default), "film", "grain", "zerolatency", "none" (no tune)
	PixFmt     string // "yuv420p10le" (default), "yuv420p", "yuv444p"
	AudioCodec string // "copy" (default), "aac", "libopus", "libmp3lame"
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

	args := []string{
		"-i", inputPath,
	}
	opts = opts.WithDefaults()

	if copySubtitles {
		args = append(args, "-map", "0")
	}
	if scaleDivisor > 1 && opts.Codec != "copy" {
		args = append(args, "-vf", fmt.Sprintf("scale=iw/%d:ih/%d", scaleDivisor, scaleDivisor))
	}

	if opts.Codec == "copy" {
		args = append(args, "-c:v", "copy")
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
	args = append(args, outputPath)

	cmd := exec.CommandContext(ctx, r.cfg.FFmpegBin, args...)

	var out io.Writer = f
	if onProgress != nil {
		out = newProgressWriter(f, onProgress)
	}
	cmd.Stdout = out
	cmd.Stderr = out

	label := ProcessPrefix + "ffmpeg-encode-" + ephemeralSuffix()
	if processName != "" {
		label = ProcessPrefix + processName + "-" + ephemeralSuffix()
	}
	tracker.register(label, cmd)
	defer tracker.unregister(label)

	return cmd.Run()
}

// FFmpegRemuxAudio combines video from videoPath with audio+subtitles from
// audioSourcePath using stream copy (no re-encoding). Used after video2x
// processing to restore audio/subtitle streams from the original file.
// The output replaces videoPath via a temp file rename.
func (r *Runner) FFmpegRemuxAudio(ctx context.Context, videoPath, audioSourcePath string) error {
	ext := filepath.Ext(videoPath)
	tmpPath := strings.TrimSuffix(videoPath, ext) + ".remux.tmp" + ext
	defer os.Remove(tmpPath)

	args := []string{
		"-i", videoPath,
		"-i", audioSourcePath,
		"-map", "0:v",
		"-map", "1:a?",
		"-map", "1:s?",
		"-c:v", "copy",
		"-c:a", "aac", "-q:a", "2",
		"-c:s", "copy",
		"-y",
		tmpPath,
	}

	cmd := exec.CommandContext(ctx, r.cfg.FFmpegBin, args...)

	f, err := os.OpenFile(r.cfg.BaseDir+"/ffmpeg-remux.log", os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return fmt.Errorf("open remux log: %w", err)
	}
	defer f.Close()
	cmd.Stdout = f
	cmd.Stderr = f

	label := ProcessPrefix + "ffmpeg-remux-" + ephemeralSuffix()
	tracker.register(label, cmd)
	defer tracker.unregister(label)

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("remux audio: %w", err)
	}

	if err := os.Rename(tmpPath, videoPath); err != nil {
		return fmt.Errorf("remux rename: %w", err)
	}
	return nil
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

	cmd := exec.CommandContext(ctx, r.cfg.FFmpegBin,
		"-stats", "-v", "error",
		"-i", absPath,
		"-f", "null",
		"-",
	)

	var errBuf bytes.Buffer
	var out io.Writer = &errBuf
	if onProgress != nil {
		out = io.MultiWriter(&errBuf, newProgressWriter(io.Discard, onProgress))
	}
	cmd.Stdout = out
	cmd.Stderr = out

	label := ProcessPrefix + "ffmpeg-decode-" + ephemeralSuffix()
	if processName != "" {
		label = ProcessPrefix + "ffmpeg-" + processName + "-" + ephemeralSuffix()
	}
	tracker.register(label, cmd)
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
	Index     int            `json:"index"`
	CodecType string         `json:"codec_type"`
	CodecName string         `json:"codec_name"`
	Width     int            `json:"width,omitempty"`
	Height    int            `json:"height,omitempty"`
	Channels  int            `json:"channels,omitempty"`
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
