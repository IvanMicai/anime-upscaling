package runner

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strings"
	"syscall"
	"testing"
)

func TestSignalFromError_NilError(t *testing.T) {
	if _, ok := SignalFromError(nil); ok {
		t.Fatal("expected ok=false for nil error")
	}
}

func TestSignalFromError_NonExecError(t *testing.T) {
	if _, ok := SignalFromError(errors.New("boom")); ok {
		t.Fatal("expected ok=false for non-exec error")
	}
}

func TestSignalFromError_NormalExitCode(t *testing.T) {
	cmd := exec.Command("sh", "-c", "exit 3")
	err := cmd.Run()
	if err == nil {
		t.Fatal("expected error from exit 3")
	}
	if _, ok := SignalFromError(err); ok {
		t.Fatalf("expected ok=false for normal exit, got signal")
	}
}

func TestSignalFromError_KilledBySignal(t *testing.T) {
	cmd := exec.Command("sh", "-c", "kill -SEGV $$")
	err := cmd.Run()
	if err == nil {
		t.Fatal("expected error from SIGSEGV")
	}
	sig, ok := SignalFromError(err)
	if !ok {
		t.Fatalf("expected ok=true for signaled exit, err=%v", err)
	}
	if sig != syscall.SIGSEGV {
		t.Fatalf("expected SIGSEGV, got %v", sig)
	}
}

func TestSignalFromError_Wrapped(t *testing.T) {
	cmd := exec.Command("sh", "-c", "kill -SEGV $$")
	inner := cmd.Run()
	wrapped := fmt.Errorf("encode failed: %w", inner)
	sig, ok := SignalFromError(wrapped)
	if !ok || sig != syscall.SIGSEGV {
		t.Fatalf("expected SIGSEGV through wrap, got sig=%v ok=%v", sig, ok)
	}
}

func TestGPUEncoderFor(t *testing.T) {
	cases := []struct {
		codec, vendor, want string
	}{
		{"libx265", "nvidia", "hevc_nvenc"},
		{"libx264", "nvidia", "h264_nvenc"},
		{"libx265", "amd", "hevc_amf"},
		{"libx265", "intel", "hevc_qsv"},
		{"libvpx-vp9", "nvidia", ""},
		{"libx265", "", ""},
		{"copy", "nvidia", ""},
	}
	for _, c := range cases {
		got := gpuEncoderFor(c.codec, c.vendor)
		if got != c.want {
			t.Errorf("gpuEncoderFor(%q,%q) = %q, want %q", c.codec, c.vendor, got, c.want)
		}
	}
}

func TestBuildGPUEncodeArgs_NVIDIA_matchesValidatedCommand(t *testing.T) {
	// Mirrors the exact NVENC command the user validated:
	//   -c:v hevc_nvenc -preset p6 -tune hq -rc vbr -cq 26 -b:v 0
	//   -pix_fmt p010le -bf 4 -spatial-aq 1
	opts := EncodeOptions{
		Codec:     "libx265",
		PixFmt:    "yuv420p10le",
		UseGPU:    true,
		GPUVendor: "nvidia",
		GPUDevice: 0,
	}
	got := strings.Join(buildGPUEncodeArgs(opts, 26), " ")
	want := "-c:v hevc_nvenc -preset p6 -tune hq -rc vbr -cq 26 -b:v 0 -pix_fmt p010le -bf 4 -spatial-aq 1"
	if got != want {
		t.Errorf("NVENC args mismatch:\n got:  %s\n want: %s", got, want)
	}
}

func TestBuildVideoFilter(t *testing.T) {
	cases := []struct {
		name         string
		scaleDivisor int
		frameDivisor int
		frameAbs     float64
		codec        string
		want         string
	}{
		{name: "none", scaleDivisor: 1, frameDivisor: 1, codec: "libx265", want: ""},
		{name: "scale", scaleDivisor: 2, frameDivisor: 1, codec: "libx265", want: "scale=iw/2:ih/2"},
		{name: "fps", scaleDivisor: 1, frameDivisor: 4, codec: "libx265", want: "fps=fps=source_fps/4"},
		{name: "scale and fps", scaleDivisor: 2, frameDivisor: 4, codec: "libx265", want: "scale=iw/2:ih/2,fps=fps=source_fps/4"},
		{name: "copy ignores filters", scaleDivisor: 2, frameDivisor: 4, codec: "copy", want: ""},
		{name: "absolute fps", scaleDivisor: 1, frameDivisor: 1, frameAbs: 24, codec: "libx265", want: "fps=fps=min(24\\,source_fps)"},
		{name: "absolute overrides divisor", scaleDivisor: 1, frameDivisor: 4, frameAbs: 30, codec: "libx265", want: "fps=fps=min(30\\,source_fps)"},
		{name: "scale and absolute", scaleDivisor: 2, frameDivisor: 1, frameAbs: 12.5, codec: "libx265", want: "scale=iw/2:ih/2,fps=fps=min(12.5\\,source_fps)"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := buildVideoFilter(c.scaleDivisor, c.frameDivisor, c.frameAbs, c.codec)
			if got != c.want {
				t.Errorf("buildVideoFilter() = %q, want %q", got, c.want)
			}
		})
	}
}

func TestGPUPixFmt_NVIDIA(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{"yuv420p10le", "p010le"},
		{"yuv420p", "nv12"},
		{"yuv444p", "p010le"}, // NVENC doesn't support 4:4:4; we fall back to 10-bit 4:2:0
	}
	for _, c := range cases {
		got := gpuPixFmt(c.in, "nvidia")
		if got != c.want {
			t.Errorf("gpuPixFmt(%q,nvidia) = %q, want %q", c.in, got, c.want)
		}
	}
	if gpuPixFmt("yuv420p10le", "") != "yuv420p10le" {
		t.Error("empty vendor should pass through pixfmt unchanged")
	}
}

// Sanity: ensure context cancellation still yields an exec error we can inspect.
func TestSignalFromError_CanceledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	cmd := exec.CommandContext(ctx, "sleep", "5")
	err := cmd.Run()
	if err == nil {
		t.Skip("sleep finished before cancel could take effect")
	}
	// Either signaled (SIGKILL) or non-exec "context canceled" — both acceptable,
	// just verify SignalFromError doesn't panic or misreport nil errors.
	_, _ = SignalFromError(err)
}
