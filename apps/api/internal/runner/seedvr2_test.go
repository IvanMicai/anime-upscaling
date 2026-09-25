package runner

import (
	"errors"
	"io"
	"slices"
	"strings"
	"testing"

	"anime-upscaling/internal/config"
)

func TestSeedVR2WithDefaults(t *testing.T) {
	got := UpscaleOptions{Processor: "seedvr2"}.WithDefaults()
	if got.Model != DefaultSeedVR2Model || got.TargetHeight != DefaultSeedVR2Height {
		t.Errorf("WithDefaults = %+v, want model %s at %dp", got, DefaultSeedVR2Model, DefaultSeedVR2Height)
	}
	if n := got.EffectiveNoiseLevel(); n != 0 {
		t.Errorf("SeedVR2 has no noise setting, EffectiveNoiseLevel = %d", n)
	}
	if h := (UpscaleOptions{Processor: "realesrgan"}).WithDefaults().TargetHeight; h != 0 {
		t.Errorf("video2x processors must not get a target height, got %d", h)
	}
}

func TestSeedVR2Tuning(t *testing.T) {
	r := NewRunner(config.Config{SeedVR2BlocksToSwap: -1})
	if got := r.SeedVR2Tuning("seedvr2-7b"); got != (SeedVR2Tuning{BatchSize: 13, BlocksToSwap: 20, VAETileSize: 1024}) {
		t.Errorf("7B defaults = %+v", got)
	}
	r = NewRunner(config.Config{SeedVR2BatchSize: 21, SeedVR2BlocksToSwap: 99})
	if got := r.SeedVR2Tuning("seedvr2-3b"); got.BatchSize != 21 || got.BlocksToSwap != 32 {
		t.Errorf("overrides = %+v, want batch 21 and swap clamped to the 3B's 32 blocks", got)
	}
	if got := SafeSeedVR2Tuning("seedvr2-7b-sharp"); got != (SeedVR2Tuning{BatchSize: 5, BlocksToSwap: 36, VAETileSize: 512}) {
		t.Errorf("safe tuning = %+v", got)
	}
}

func flagValue(args []string, flag string) (string, bool) {
	i := slices.Index(args, flag)
	if i < 0 || i+1 >= len(args) {
		return "", i >= 0
	}
	return args[i+1], true
}

func TestSeedVR2Args(t *testing.T) {
	args := seedVR2Args("/m", "dit.safetensors", "/in.mkv", "/out.mp4", 1440,
		SeedVR2Tuning{BatchSize: 13, BlocksToSwap: 20, VAETileSize: 1024}, []string{"--attention_mode", "sageattn_2"})
	if args[0] != "inference_cli.py" || args[1] != "/in.mkv" {
		t.Fatalf("args must start with the script and the input: %q", args[:2])
	}
	for flag, want := range map[string]string{
		"--output": "/out.mp4", "--model_dir": "/m", "--dit_model": "dit.safetensors",
		"--resolution": "1440", "--batch_size": "13", "--chunk_size": "65",
		"--blocks_to_swap": "20", "--vae_decode_tile_size": "1024", "--vae_decode_tile_overlap": "128",
		"--video_backend": "ffmpeg", "--attention_mode": "sageattn_2",
	} {
		if got, _ := flagValue(args, flag); got != want {
			t.Errorf("%s = %q, want %q", flag, got, want)
		}
	}
	for _, flag := range []string{"--10bit", "--cache_dit", "--cache_vae", "--swap_io_components", "--uniform_batch_size"} {
		if !slices.Contains(args, flag) {
			t.Errorf("missing %s", flag)
		}
	}

	noSwap := seedVR2Args("/m", "d", "/i", "/o", 1080, SeedVR2Tuning{BatchSize: 5, VAETileSize: 512}, nil)
	for _, flag := range []string{"--blocks_to_swap", "--swap_io_components"} {
		if slices.Contains(noSwap, flag) {
			t.Errorf("%s must be omitted without block swapping (the CLI rejects swap without an offload)", flag)
		}
	}
}

func TestSeedVR2MuxArgs(t *testing.T) {
	mkv := strings.Join(seedVR2MuxArgs("/v.mp4", "/src.mkv", "/out.MKV"), " ")
	if !strings.Contains(mkv, "-map 1:t?") {
		t.Errorf("Matroska output should keep attachments (fonts): %s", mkv)
	}
	mp4 := strings.Join(seedVR2MuxArgs("/v.mp4", "/src.mp4", "/out.mp4"), " ")
	if strings.Contains(mp4, "1:t?") {
		t.Errorf("MP4 cannot hold attachments: %s", mp4)
	}
	for _, want := range []string{"-map 0:v:0", "-map 1:a?", "-map 1:s?", "-map_chapters 1", "-c copy"} {
		if !strings.Contains(mp4, want) {
			t.Errorf("mux args missing %q: %s", want, mp4)
		}
	}
}

func TestIsCUDAOutOfMemory(t *testing.T) {
	for msg, want := range map[string]bool{
		"exit status 1: torch.OutOfMemoryError: CUDA out of memory. Tried to allocate": true,
		"exit status 1: RuntimeError: CUDA error: out of memory":                       true,
		"exit status 1: FileNotFoundError: Video file not found":                       false,
	} {
		if got := IsCUDAOutOfMemory(errors.New(msg)); got != want {
			t.Errorf("IsCUDAOutOfMemory(%q) = %v, want %v", msg, got, want)
		}
	}
	if IsCUDAOutOfMemory(nil) {
		t.Error("nil error is not an OOM")
	}
}

// TestSeedVR2ProgressWriter feeds the CLI's log lines and checks the estimated
// percentage: 2 chunks, 4 phases each, so finishing "Upscaling batch 1/1" of
// chunk 1 is (1 + 1) / 4 of the first half = 25%.
func TestSeedVR2ProgressWriter(t *testing.T) {
	var got []Progress
	pw := newSeedVR2ProgressWriter(io.Discard, 200, func(p Progress) { got = append(got, p) })
	pw.minInterval = 0

	_, _ = pw.Write([]byte("🎬 Chunk 1/2: 100 new + 0 context frames\n"))
	_, _ = pw.Write([]byte("🧠 Upscaling ba"))
	_, _ = pw.Write([]byte("tch 1/1\n"))
	_, _ = pw.Write([]byte("🎬 Chunk 2/2: 100 new + 3 context frames\r"))

	if len(got) != 3 {
		t.Fatalf("emits = %d (%+v), want chunk 1, the upscale batch and chunk 2", len(got), got)
	}
	if got[0].Percent != 0 || got[0].Phase != "Chunk 1/2" {
		t.Errorf("chunk 1 start = %+v", got[0])
	}
	if got[1].Percent != 25 || got[1].Frame != 50 || got[1].TotalFrames != 200 || got[1].Phase != "Chunk 1/2 · Upscaling" {
		t.Errorf("after upscaling chunk 1 = %+v, want 25%% = frame 50/200", got[1])
	}
	if got[2].Percent != 50 {
		t.Errorf("chunk 2 start = %+v, want 50%%", got[2])
	}
}
