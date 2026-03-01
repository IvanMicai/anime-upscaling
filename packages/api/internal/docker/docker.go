package docker

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

const ContainerPrefix = "anime-upscaling-"

func ephemeralSuffix() string {
	return strconv.FormatInt(time.Now().UnixNano(), 36)
}

type Docker struct {
	cfg config.Config
}

func NewDocker(cfg config.Config) *Docker {
	return &Docker{cfg: cfg}
}

// Video2x runs video2x upscale on a specific GPU, writing docker stdout/stderr to logPath.
// If onProgress is non-nil, the log output is also parsed for progress data.
func (d *Docker) Video2x(ctx context.Context, gpuID int, filename, logPath string, scale int, onProgress func(Progress)) error {
	f, err := os.Create(logPath)
	if err != nil {
		return fmt.Errorf("create docker log: %w", err)
	}
	defer f.Close()

	cmd := exec.CommandContext(ctx, "docker", "run", "--rm",
		"--name", fmt.Sprintf("%svideo2x-gpu%d", ContainerPrefix, gpuID),
		"-u", fmt.Sprintf("%d:%d", d.cfg.UserID, d.cfg.GroupID),
		"--gpus", fmt.Sprintf("device=%d", gpuID),
		"-v", d.cfg.BaseDir+":/host",
		d.cfg.Video2xImage,
		"-i", "/host/input/"+filename,
		"-o", "/host/output/"+filename,
		"-p", "realesrgan",
		"-s", strconv.Itoa(scale),
		"--realesrgan-model", "realesr-animevideov3",
	)

	var out io.Writer = f
	if onProgress != nil {
		out = newProgressWriter(f, onProgress)
	}
	cmd.Stdout = out
	cmd.Stderr = out
	return cmd.Run()
}

// FFmpegEncode compresses a video with H.265.
// If onProgress is non-nil, stderr/stdout are intercepted to parse progress data.
func (d *Docker) FFmpegEncode(ctx context.Context, inputRelPath, outputRelPath string, crf int, cpus int, containerName string, copySubtitles bool, scaleDivisor int, onProgress func(Progress)) error {
	f, err := os.OpenFile(d.cfg.BaseDir+"/docker_ffmpeg.log", os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return fmt.Errorf("open ffmpeg log: %w", err)
	}
	defer f.Close()

	name := ContainerPrefix + "ffmpeg-encode-" + ephemeralSuffix()
	if containerName != "" {
		name = ContainerPrefix + containerName + "-" + ephemeralSuffix()
	}
	args := []string{"run", "--rm",
		"--runtime=runc",
		"--name", name,
		"--cpus=" + strconv.Itoa(cpus),
		"-e", "PUID=" + strconv.Itoa(d.cfg.UserID),
		"-e", "PGID=" + strconv.Itoa(d.cfg.GroupID),
		"-v", d.cfg.BaseDir + ":/work",
	}
	args = append(args, d.cfg.FFmpegImage,
		"-i", "/work/"+inputRelPath,
	)
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
		"-c:a", "copy",
	)
	if copySubtitles {
		args = append(args, "-c:s", "copy")
	}
	args = append(args, "/work/"+outputRelPath)

	cmd := exec.CommandContext(ctx, "docker", args...)

	var out io.Writer = f
	if onProgress != nil {
		out = newProgressWriter(f, onProgress)
	}
	cmd.Stdout = out
	cmd.Stderr = out
	return cmd.Run()
}

// FFprobe runs ffprobe on a file, returns stdout+stderr combined.
func (d *Docker) FFprobe(ctx context.Context, relPath string) (string, error) {
	var buf bytes.Buffer
	cmd := exec.CommandContext(ctx, "docker", "run", "--rm",
		"--runtime=runc",
		"--name", ContainerPrefix+"ffprobe-"+ephemeralSuffix(),
		"-e", "PUID="+strconv.Itoa(d.cfg.UserID),
		"-e", "PGID="+strconv.Itoa(d.cfg.GroupID),
		"-v", d.cfg.BaseDir+":/work",
		"--entrypoint", "ffprobe",
		d.cfg.FFmpegImage,
		"-v", "error",
		"-show_entries", "stream=codec_type",
		"-of", "csv=p=0",
		"/work/"+relPath,
	)
	cmd.Stdout = &buf
	cmd.Stderr = &buf
	err := cmd.Run()
	return buf.String(), err
}

// FFmpegDecode does a full decode pass to check integrity.
// If onProgress is non-nil, stderr is parsed for progress data.
func (d *Docker) FFmpegDecode(ctx context.Context, relPath string, containerName string, onProgress func(Progress)) (string, error) {
	name := ContainerPrefix + "ffmpeg-decode-" + ephemeralSuffix()
	if containerName != "" {
		name = ContainerPrefix + "ffmpeg-" + containerName + "-" + ephemeralSuffix()
	}

	cmd := exec.CommandContext(ctx, "docker", "run", "--rm",
		"--runtime=runc",
		"--name", name,
		"-e", "PUID="+strconv.Itoa(d.cfg.UserID),
		"-e", "PGID="+strconv.Itoa(d.cfg.GroupID),
		"-v", d.cfg.BaseDir+":/work",
		d.cfg.FFmpegImage,
		"-stats", "-v", "error",
		"-i", "/work/"+relPath,
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
	err := cmd.Run()
	return errBuf.String(), err
}

// Chown fixes file permissions via alpine container.
func (d *Docker) Chown(ctx context.Context, hostDir, filename string) error {
	cmd := exec.CommandContext(ctx, "docker", "run", "--rm",
		"--runtime=runc",
		"--name", ContainerPrefix+"chown-"+ephemeralSuffix(),
		"-v", hostDir+":/work",
		d.cfg.AlpineImage,
		"chown", fmt.Sprintf("%d:%d", d.cfg.UserID, d.cfg.GroupID), "/work/"+filename,
	)
	return cmd.Run()
}

// StopContainer stops a container by its full name (including prefix).
func (d *Docker) StopContainer(name string) error {
	return exec.Command("docker", "stop", name).Run()
}

// VideoResolution holds the width and height of a video stream.
type VideoResolution struct {
	Width  int
	Height int
}

// DirMount represents a directory to mount in the multi-dir ffprobe container.
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

// FFprobeBatchResolutionMultiDir runs a single Docker container that mounts
// multiple directories and probes all specified files. Returns label -> filename -> resolution.
func (d *Docker) FFprobeBatchResolutionMultiDir(ctx context.Context, mounts []DirMount, filesByLabel map[string][]string) (map[string]map[string]VideoResolution, error) {
	// Count total files
	total := 0
	for _, files := range filesByLabel {
		total += len(files)
	}
	if total == 0 {
		return nil, nil
	}

	// Build volume mounts and script
	var args []string
	args = append(args, "run", "--rm",
		"--runtime=runc",
		"--name", ContainerPrefix+"ffprobe-multi-"+ephemeralSuffix(),
		"-e", "PUID="+strconv.Itoa(d.cfg.UserID),
		"-e", "PGID="+strconv.Itoa(d.cfg.GroupID),
	)
	for _, m := range mounts {
		args = append(args, "-v", m.HostDir+":/work/"+m.Label+":ro")
	}
	args = append(args, "--entrypoint", "sh", d.cfg.FFmpegImage, "-c")

	// Script: output "label\tfilename\twidth,height"
	var sb strings.Builder
	for _, m := range mounts {
		files := filesByLabel[m.Label]
		for _, f := range files {
			escapedF := strings.ReplaceAll(f, "'", "'\\''")
			sb.WriteString(fmt.Sprintf(
				`res=$(ffprobe -v error -select_streams v:0 -show_entries stream=width,height -of csv=p=0 '/work/%s/%s' 2>/dev/null) && printf '%%s\t%%s\t%%s\n' '%s' '%s' "$res"; `,
				m.Label, f, m.Label, escapedF,
			))
		}
	}
	args = append(args, sb.String())

	var buf bytes.Buffer
	cmd := exec.CommandContext(ctx, "docker", args...)
	cmd.Stdout = &buf
	cmd.Stderr = &buf
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("ffprobe multi: %w", err)
	}

	result := make(map[string]map[string]VideoResolution)
	for _, line := range strings.Split(strings.TrimSpace(buf.String()), "\n") {
		if line == "" {
			continue
		}
		parts := strings.SplitN(line, "\t", 3)
		if len(parts) != 3 {
			continue
		}
		label := parts[0]
		name := parts[1]
		dims := strings.SplitN(strings.TrimSpace(parts[2]), ",", 2)
		if len(dims) != 2 {
			continue
		}
		w, err1 := strconv.Atoi(strings.TrimSpace(dims[0]))
		h, err2 := strconv.Atoi(strings.TrimSpace(dims[1]))
		if err1 != nil || err2 != nil {
			continue
		}
		if result[label] == nil {
			result[label] = make(map[string]VideoResolution)
		}
		result[label][name] = VideoResolution{Width: w, Height: h}
	}
	return result, nil
}

// FFprobeBatchResolutionMultiDirCached wraps FFprobeBatchResolutionMultiDir with
// a per-directory in-memory cache (20-minute TTL). Returns results + cachedAt.
func (d *Docker) FFprobeBatchResolutionMultiDirCached(ctx context.Context, mounts []DirMount, filesByLabel map[string][]string, forceRefresh bool) (map[string]map[string]VideoResolution, time.Time, error) {
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

	// Probe all dirs in a single container
	data, err := d.FFprobeBatchResolutionMultiDir(ctx, mounts, filesByLabel)
	if err != nil {
		return nil, time.Time{}, err
	}

	cachedAt := time.Now()

	// Update per-dir cache
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

// StopByPrefix stops all containers whose name matches the given prefix. Returns count stopped.
func (d *Docker) StopByPrefix(ctx context.Context, prefix string) (int, error) {
	out, err := exec.CommandContext(ctx, "docker", "ps", "-q", "--filter", "name="+prefix).Output()
	if err != nil {
		return 0, err
	}
	ids := strings.Fields(strings.TrimSpace(string(out)))
	if len(ids) == 0 {
		return 0, nil
	}
	args := append([]string{"stop"}, ids...)
	cmd := exec.CommandContext(ctx, "docker", args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return len(ids), cmd.Run()
}
