package process

import (
	"errors"
	"os"
	"os/exec"
	"strings"
	"testing"

	"anime-upscaling/internal/runner"
)

func TestFallbackEncodeOptions_SwitchesToConservativeDefaults(t *testing.T) {
	orig := runner.EncodeOptions{
		Codec:      "libx265",
		Preset:     "fast",
		Tune:       "animation",
		PixFmt:     "yuv420p10le",
		AudioCodec: "copy",
	}
	fb := fallbackEncodeOptions(orig)

	if fb.Codec != "libx264" {
		t.Errorf("expected fallback codec libx264, got %q", fb.Codec)
	}
	if fb.PixFmt != "yuv420p" {
		t.Errorf("expected fallback pixfmt yuv420p (8-bit), got %q", fb.PixFmt)
	}
	if fb.Preset != "medium" {
		t.Errorf("expected fallback preset medium, got %q", fb.Preset)
	}
	if fb.AudioCodec != "copy" {
		t.Errorf("expected AudioCodec preserved, got %q", fb.AudioCodec)
	}
}

func TestFallbackEncodeOptions_PreservesExtraArgs(t *testing.T) {
	orig := runner.EncodeOptions{
		ExtraArgs: []string{"-x265-params", "pools=none"},
	}
	fb := fallbackEncodeOptions(orig)
	if len(fb.ExtraArgs) != 2 || fb.ExtraArgs[0] != "-x265-params" {
		t.Errorf("expected ExtraArgs preserved, got %v", fb.ExtraArgs)
	}
}

func TestDescribeRunError_SignaledExit(t *testing.T) {
	err := exec.Command("sh", "-c", "kill -SEGV $$").Run()
	if err == nil {
		t.Fatal("expected SIGSEGV error")
	}
	got := describeRunError(err)
	if !strings.Contains(got, "signal=") {
		t.Errorf("expected signal= in description, got %q", got)
	}
}

func TestDescribeRunError_NonZeroExit(t *testing.T) {
	err := exec.Command("sh", "-c", "exit 7").Run()
	if err == nil {
		t.Fatal("expected non-zero exit error")
	}
	got := describeRunError(err)
	if !strings.Contains(got, "exit=7") {
		t.Errorf("expected exit=7 in description, got %q", got)
	}
}

func TestDescribeRunError_PlainError(t *testing.T) {
	got := describeRunError(errors.New("boom"))
	if got != "boom" {
		t.Errorf("expected plain message, got %q", got)
	}
}

func TestInputFileMeta_MissingFile(t *testing.T) {
	got := inputFileMeta("/nonexistent/path/deadbeef.mkv")
	if !strings.Contains(got, "stat falhou") {
		t.Errorf("expected 'stat falhou' for missing file, got %q", got)
	}
}

func TestInputFileMeta_ExistingFile(t *testing.T) {
	tmp := t.TempDir() + "/probe.txt"
	if err := os.WriteFile(tmp, []byte("hello"), 0644); err != nil {
		t.Fatal(err)
	}
	got := inputFileMeta(tmp)
	if !strings.Contains(got, "size=5") {
		t.Errorf("expected size=5 for 5-byte file, got %q", got)
	}
	if !strings.Contains(got, "mtime=") {
		t.Errorf("expected mtime= in meta, got %q", got)
	}
}
