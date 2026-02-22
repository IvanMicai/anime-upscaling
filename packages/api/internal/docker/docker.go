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
func (d *Docker) Video2x(ctx context.Context, gpuID int, filename, logPath string, onProgress func(Progress)) error {
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
		"-s", "2",
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
func (d *Docker) FFmpegEncode(ctx context.Context, inputRelPath, outputRelPath string, crf int, cpus int, containerName string, copySubtitles bool, onProgress func(Progress)) error {
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
func (d *Docker) FFmpegDecode(ctx context.Context, relPath string) (string, error) {
	var buf bytes.Buffer
	cmd := exec.CommandContext(ctx, "docker", "run", "--rm",
		"--name", ContainerPrefix+"ffmpeg-decode-"+ephemeralSuffix(),
		"-e", "PUID="+strconv.Itoa(d.cfg.UserID),
		"-e", "PGID="+strconv.Itoa(d.cfg.GroupID),
		"-v", d.cfg.BaseDir+":/work",
		d.cfg.FFmpegImage,
		"-v", "error",
		"-i", "/work/"+relPath,
		"-f", "null",
		"-",
	)
	cmd.Stdout = &buf
	cmd.Stderr = &buf
	err := cmd.Run()
	return buf.String(), err
}

// Chown fixes file permissions via alpine container.
func (d *Docker) Chown(ctx context.Context, hostDir, filename string) error {
	cmd := exec.CommandContext(ctx, "docker", "run", "--rm",
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
