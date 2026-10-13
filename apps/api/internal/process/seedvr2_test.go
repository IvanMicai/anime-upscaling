package process

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"anime-upscaling/internal/config"
	"anime-upscaling/internal/logger"
	"anime-upscaling/internal/runner"
)

// fakeSeedVR2 stands in for the venv's python. "-c" is the weight download;
// anything else is an inference_cli.py run, which prints the CLI's progress
// lines and writes a 128x72 video-only mp4 to --output. When $FAKE_OOM names
// a file that does not exist yet, the first run creates it and fails the way
// PyTorch does on a CUDA out-of-memory error.
const fakeSeedVR2 = `#!/bin/sh
if [ "$1" = "-c" ]; then echo "fetch $3 $4" >> "$FAKE_LOG"; exit 0; fi
echo "run $*" >> "$FAKE_LOG"
if [ -n "$FAKE_OOM" ] && [ ! -f "$FAKE_OOM" ]; then
  touch "$FAKE_OOM"
  echo "torch.OutOfMemoryError: CUDA out of memory. Tried to allocate 2.00 GiB"
  exit 1
fi
prev=""; out=""
for a in "$@"; do [ "$prev" = "--output" ] && out="$a"; prev="$a"; done
echo "Chunk 1/1: 3 new + 0 context frames"
echo "Upscaling batch 1/1"
exec ffmpeg -loglevel error -y -i "$2" -map 0:v -vf scale=128:72 -c:v libx264 "$out"
`

func seedVR2TestSetup(t *testing.T) (config.Config, string) {
	t.Helper()
	for _, bin := range []string{"ffmpeg", "ffprobe"} {
		if _, err := exec.LookPath(bin); err != nil {
			t.Skipf("%s not installed", bin)
		}
	}
	base := t.TempDir()
	sv := filepath.Join(base, "seedvr2")
	for _, d := range []string{filepath.Join(sv, "venv", "bin"), filepath.Join(sv, "app"), filepath.Join(base, "input")} {
		if err := os.MkdirAll(d, 0755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(sv, "venv", "bin", "python"), []byte(fakeSeedVR2), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sv, "app", "inference_cli.py"), nil, 0644); err != nil {
		t.Fatal(err)
	}

	// A 3-frame 64x36 clip with an audio track: the audio must survive, since
	// SeedVR2 itself writes video only.
	in := filepath.Join(base, "input", "ep 01.mkv")
	gen := exec.Command("ffmpeg", "-loglevel", "error",
		"-f", "lavfi", "-i", "testsrc=size=64x36:rate=24",
		"-f", "lavfi", "-i", "sine=frequency=440:sample_rate=48000",
		"-frames:v", "3", "-t", "0.125", "-c:v", "libx264", "-c:a", "aac", in)
	if out, err := gen.CombinedOutput(); err != nil {
		t.Fatalf("make input: %v\n%s", err, out)
	}

	cfg := config.Config{
		BaseDir:          base,
		InputDir:         filepath.Join(base, "input"),
		OutputDir:        filepath.Join(base, "output"),
		TempDir:          filepath.Join(base, "temp"),
		FFmpegBin:        "ffmpeg",
		FFprobeBin:       "ffprobe",
		StreamsPerGPU:    1,
		SeedVR2Dir:       sv,
		SeedVR2ModelDir:  filepath.Join(base, "models", "seedvr2"),
		SeedVR2BatchSize: 0, SeedVR2BlocksToSwap: -1,
		UserID: os.Getuid(), GroupID: os.Getgid(),
	}
	fakeLog := filepath.Join(base, "fake.log")
	t.Setenv("FAKE_LOG", fakeLog)
	return cfg, fakeLog
}

func probeStreams(t *testing.T, path string) string {
	t.Helper()
	out, err := exec.Command("ffprobe", "-v", "error", "-show_entries", "stream=codec_type,width,height", "-of", "csv=p=0", path).Output()
	if err != nil {
		t.Fatalf("ffprobe %s: %v", path, err)
	}
	return strings.TrimSpace(string(out))
}

// TestUpscaleFileSeedVR2 runs the whole seedvr2 path of UpscaleFile against the
// fake CLI: weights fetched first, the default tuning on the first run, a
// retry with the safe tuning after an out-of-memory error, audio muxed back
// from the source, and the result renamed into output/.
func TestUpscaleFileSeedVR2(t *testing.T) {
	cfg, fakeLog := seedVR2TestSetup(t)
	t.Setenv("FAKE_OOM", filepath.Join(cfg.BaseDir, "oom-once"))

	var mu sync.Mutex
	var logs []logger.JobLog
	var progress []runner.Progress
	onEvent := func(l logger.JobLog) { mu.Lock(); logs = append(logs, l); mu.Unlock() }
	onProgress := func(p runner.Progress) { mu.Lock(); progress = append(progress, p); mu.Unlock() }

	opts := runner.UpscaleOptions{Processor: "seedvr2", Model: "seedvr2-3b", TargetHeight: 720}
	ok := UpscaleFile(context.Background(), cfg, runner.NewRunner(cfg), 1, 0, "ep 01.mkv", 1, 2, opts, cfg.InputDir, cfg.OutputDir, onEvent, onProgress)
	if !ok {
		t.Fatalf("UpscaleFile failed; logs: %+v", logs)
	}

	if got := probeStreams(t, filepath.Join(cfg.OutputDir, "ep 01.mkv")); got != "video,128,72\naudio" {
		t.Errorf("output streams = %q, want the upscaled video plus the source audio", got)
	}

	data, _ := os.ReadFile(fakeLog)
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	if len(lines) != 3 {
		t.Fatalf("fake CLI calls = %q, want fetch + 2 runs", lines)
	}
	if lines[0] != "fetch seedvr2_ema_3b_fp16.safetensors ema_vae_fp16.safetensors" {
		t.Errorf("fetch call = %q", lines[0])
	}
	if !strings.Contains(lines[1], "--batch_size 13 ") || strings.Contains(lines[1], "--blocks_to_swap") {
		t.Errorf("first run should use the 3B defaults (batch 13, no swap): %q", lines[1])
	}
	if !strings.Contains(lines[1], "--resolution 720 ") {
		t.Errorf("first run should target 720p: %q", lines[1])
	}
	if !strings.Contains(lines[2], "--batch_size 5 ") || !strings.Contains(lines[2], "--blocks_to_swap 32 ") {
		t.Errorf("retry should use the safe tuning (batch 5, all 32 blocks swapped): %q", lines[2])
	}

	var sawRetry, sawOK bool
	for _, l := range logs {
		sawRetry = sawRetry || strings.Contains(l.Message, "ran out of GPU memory")
		sawOK = sawOK || l.Level == "OK"
	}
	if !sawRetry || !sawOK {
		t.Errorf("want an out-of-memory retry notice and an OK; logs: %+v", logs)
	}
	var sawChunk bool
	for _, p := range progress {
		if p.Source == "GPU 1" && strings.HasPrefix(p.Phase, "Chunk 1/1") && p.TotalFrames == 3 {
			sawChunk = true
		}
	}
	if !sawChunk {
		t.Errorf("want chunk progress from GPU 1; got %+v", progress)
	}
	if _, err := os.Stat(filepath.Join(cfg.TempDir, "seedvr2", "gpu1-s0.mp4")); !os.IsNotExist(err) {
		t.Errorf("intermediate SeedVR2 video should be removed, stat err = %v", err)
	}
}

// TestUpscaleFileSeedVR2Missing: an image without the runtime fails the file
// with a message saying which image to use, instead of a python exec error.
func TestUpscaleFileSeedVR2Missing(t *testing.T) {
	cfg, _ := seedVR2TestSetup(t)
	cfg.SeedVR2Dir = filepath.Join(cfg.BaseDir, "absent")

	var msgs []string
	onEvent := func(l logger.JobLog) { msgs = append(msgs, l.Level+" "+l.Message) }
	opts := runner.UpscaleOptions{Processor: "seedvr2"}
	if UpscaleFile(context.Background(), cfg, runner.NewRunner(cfg), 0, 0, "ep 01.mkv", 1, 2, opts, cfg.InputDir, cfg.OutputDir, onEvent, func(runner.Progress) {}) {
		t.Fatal("UpscaleFile should fail without the SeedVR2 runtime")
	}
	last := msgs[len(msgs)-1]
	if !strings.HasPrefix(last, "ERRO") || !strings.Contains(last, "--target seedvr2") {
		t.Errorf("last log = %q, want an ERRO naming the seedvr2 image", last)
	}
}
