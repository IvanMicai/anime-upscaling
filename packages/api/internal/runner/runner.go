package runner

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
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

// Video2x runs video2x upscale on a specific GPU, writing stdout/stderr to logPath.
// If onProgress is non-nil, the log output is also parsed for progress data.
func (r *Runner) Video2x(ctx context.Context, gpuID int, filename, logPath string, scale int, onProgress func(Progress)) error {
	f, err := os.Create(logPath)
	if err != nil {
		return fmt.Errorf("create log: %w", err)
	}
	defer f.Close()

	inputPath := r.cfg.InputDir + "/" + filename
	outputPath := r.cfg.OutputDir + "/" + filename

	cmd := exec.CommandContext(ctx, r.cfg.Video2xBin,
		"-i", inputPath,
		"-o", outputPath,
		"-p", "realesrgan",
		"-s", strconv.Itoa(scale),
		"--realesrgan-model", "realesr-animevideov3",
	)
	env := os.Environ()
	filtered := make([]string, 0, len(env))
	for _, e := range env {
		if !strings.HasPrefix(e, "CUDA_VISIBLE_DEVICES=") {
			filtered = append(filtered, e)
		}
	}
	cmd.Env = append(filtered, fmt.Sprintf("CUDA_VISIBLE_DEVICES=%d", gpuID))

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

// FFmpegEncode compresses a video with H.265.
// If onProgress is non-nil, stderr/stdout are intercepted to parse progress data.
func (r *Runner) FFmpegEncode(ctx context.Context, inputRelPath, outputRelPath string, crf int, threads int, processName string, copySubtitles bool, scaleDivisor int, onProgress func(Progress)) error {
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
	if copySubtitles {
		args = append(args, "-map", "0")
	}
	if scaleDivisor > 1 {
		args = append(args, "-vf", fmt.Sprintf("scale=iw/%d:ih/%d", scaleDivisor, scaleDivisor))
	}
	args = append(args,
		"-c:v", "libx265",
		"-preset", "fast",
		"-crf", strconv.Itoa(crf),
		"-tune", "animation",
		"-pix_fmt", "yuv420p10le",
		"-threads", strconv.Itoa(threads),
		"-c:a", "copy",
	)
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

// VideoResolution holds the width and height of a video stream.
type VideoResolution struct {
	Width  int
	Height int
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
