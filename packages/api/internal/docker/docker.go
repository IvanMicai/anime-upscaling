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

type resolutionCacheEntry struct {
	data      map[string]VideoResolution
	expiresAt time.Time
}

var (
	resolutionCache   = map[string]resolutionCacheEntry{}
	resolutionCacheMu sync.Mutex
)

// FFprobeBatchResolution runs a single Docker container that probes all video
// files in hostDir and returns a map of filename -> resolution.
func (d *Docker) FFprobeBatchResolution(ctx context.Context, hostDir string, filenames []string) (map[string]VideoResolution, error) {
	if len(filenames) == 0 {
		return nil, nil
	}

	// Build a shell script that runs ffprobe on each file and outputs "filename\twidth,height"
	var sb strings.Builder
	for _, f := range filenames {
		// Use printf to avoid newline issues in filenames; output tab-separated
		sb.WriteString(fmt.Sprintf(
			`res=$(ffprobe -v error -select_streams v:0 -show_entries stream=width,height -of csv=p=0 "/work/%s" 2>/dev/null) && printf '%%s\t%%s\n' '%s' "$res"; `,
			f, strings.ReplaceAll(f, "'", "'\\''"),
		))
	}

	var buf bytes.Buffer
	cmd := exec.CommandContext(ctx, "docker", "run", "--rm",
		"--runtime=runc",
		"--name", ContainerPrefix+"ffprobe-batch-"+ephemeralSuffix(),
		"-e", "PUID="+strconv.Itoa(d.cfg.UserID),
		"-e", "PGID="+strconv.Itoa(d.cfg.GroupID),
		"-v", hostDir+":/work:ro",
		"--entrypoint", "sh",
		d.cfg.FFmpegImage,
		"-c", sb.String(),
	)
	cmd.Stdout = &buf
	cmd.Stderr = &buf
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("ffprobe batch: %w", err)
	}

	result := make(map[string]VideoResolution, len(filenames))
	for _, line := range strings.Split(strings.TrimSpace(buf.String()), "\n") {
		if line == "" {
			continue
		}
		parts := strings.SplitN(line, "\t", 2)
		if len(parts) != 2 {
			continue
		}
		name := parts[0]
		dims := strings.SplitN(strings.TrimSpace(parts[1]), ",", 2)
		if len(dims) != 2 {
			continue
		}
		w, err1 := strconv.Atoi(strings.TrimSpace(dims[0]))
		h, err2 := strconv.Atoi(strings.TrimSpace(dims[1]))
		if err1 != nil || err2 != nil {
			continue
		}
		result[name] = VideoResolution{Width: w, Height: h}
	}
	return result, nil
}

// FFprobeBatchResolutionCached is like FFprobeBatchResolution but uses an
// in-memory cache with a 60-second TTL keyed by directory path.
func (d *Docker) FFprobeBatchResolutionCached(ctx context.Context, hostDir string, filenames []string) (map[string]VideoResolution, error) {
	resolutionCacheMu.Lock()
	if entry, ok := resolutionCache[hostDir]; ok && time.Now().Before(entry.expiresAt) {
		resolutionCacheMu.Unlock()
		return entry.data, nil
	}
	resolutionCacheMu.Unlock()

	data, err := d.FFprobeBatchResolution(ctx, hostDir, filenames)
	if err != nil {
		return nil, err
	}

	resolutionCacheMu.Lock()
	resolutionCache[hostDir] = resolutionCacheEntry{
		data:      data,
		expiresAt: time.Now().Add(60 * time.Second),
	}
	resolutionCacheMu.Unlock()

	return data, nil
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
