package process

import (
	"errors"
	"os"
	"os/exec"
	"strings"
	"testing"

	"anime-upscaling/internal/runner"
)

func TestStableEncodeOptions_KeepsCodecProfileAndAddsPoolsNone(t *testing.T) {
	orig := runner.EncodeOptions{
		Codec:      "libx265",
		Preset:     "fast",
		Tune:       "animation",
		PixFmt:     "yuv420p10le",
		AudioCodec: "copy",
	}
	st := stableEncodeOptions(orig)

	if st.Codec != "libx265" {
		t.Errorf("expected codec preserved as libx265, got %q", st.Codec)
	}
	if st.PixFmt != "yuv420p10le" {
		t.Errorf("expected pixfmt preserved as yuv420p10le (10-bit), got %q", st.PixFmt)
	}
	if st.Preset != "fast" {
		t.Errorf("expected preset preserved as fast, got %q", st.Preset)
	}
	if len(st.ExtraArgs) != 2 || st.ExtraArgs[0] != "-x265-params" || st.ExtraArgs[1] != "pools=none" {
		t.Errorf("expected -x265-params pools=none appended, got %v", st.ExtraArgs)
	}
}

func TestStableEncodeOptions_DoesNotMutateOriginalExtraArgs(t *testing.T) {
	orig := runner.EncodeOptions{
		ExtraArgs: []string{"-foo", "bar"},
	}
	_ = stableEncodeOptions(orig)
	if len(orig.ExtraArgs) != 2 {
		t.Errorf("expected original ExtraArgs untouched, got %v", orig.ExtraArgs)
	}
}

func TestStableEncodeOptions_MergesIntoExistingX265Params(t *testing.T) {
	orig := runner.EncodeOptions{
		ExtraArgs: []string{"-x265-params", "wpp=1"},
	}
	st := stableEncodeOptions(orig)
	if len(st.ExtraArgs) != 2 {
		t.Fatalf("expected merge into existing -x265-params (2 args), got %v", st.ExtraArgs)
	}
	if st.ExtraArgs[1] != "wpp=1:pools=none" {
		t.Errorf("expected pools=none merged, got %q", st.ExtraArgs[1])
	}
}

func TestStableEncodeOptions_OverridesExistingPools(t *testing.T) {
	// OptimizeFile sets pools=<budget> on every primary attempt; the stable
	// tier exists to remove the pool, so it must override — not skip.
	orig := runner.EncodeOptions{
		ExtraArgs: []string{"-x265-params", "pools=4"},
	}
	st := stableEncodeOptions(orig)
	if len(st.ExtraArgs) != 2 || st.ExtraArgs[1] != "pools=none" {
		t.Errorf("expected pools=4 overridden to pools=none, got %v", st.ExtraArgs)
	}
}

func TestSetX265Pools_AppendsWhenAbsent(t *testing.T) {
	got := setX265Pools(nil, "4")
	if len(got) != 2 || got[0] != "-x265-params" || got[1] != "pools=4" {
		t.Errorf("expected -x265-params pools=4 appended, got %v", got)
	}
}

func TestSetX265Pools_ReplacesExistingPoolsToken(t *testing.T) {
	got := setX265Pools([]string{"-x265-params", "wpp=1:pools=8:rd=4"}, "none")
	if len(got) != 2 || got[1] != "wpp=1:pools=none:rd=4" {
		t.Errorf("expected pools token replaced in place, got %v", got)
	}
}

func TestSetX265Pools_DoesNotMutateInput(t *testing.T) {
	in := []string{"-x265-params", "pools=8"}
	_ = setX265Pools(in, "none")
	if in[1] != "pools=8" {
		t.Errorf("expected input slice untouched, got %v", in)
	}
}

func TestSetX265Param_AppendsAndReplaces(t *testing.T) {
	got := setX265Param(nil, "frame-threads", "2")
	if len(got) != 2 || got[0] != "-x265-params" || got[1] != "frame-threads=2" {
		t.Fatalf("expected -x265-params frame-threads=2 appended, got %v", got)
	}
	got = setX265Param([]string{"-x265-params", "frame-threads=8:rd=4"}, "frame-threads", "2")
	if len(got) != 2 || got[1] != "frame-threads=2:rd=4" {
		t.Errorf("expected frame-threads token replaced in place, got %v", got)
	}
}

func TestSetX265Param_PoolsAndFrameThreadsCoexist(t *testing.T) {
	// OptimizeFile chama setX265Param duas vezes (pools + frame-threads); ambos
	// devem cair no mesmo token -x265-params, sem um sobrescrever o outro.
	got := setX265Param(nil, "pools", "4")
	got = setX265Param(got, "frame-threads", "2")
	if len(got) != 2 || got[0] != "-x265-params" {
		t.Fatalf("expected single -x265-params arg, got %v", got)
	}
	if got[1] != "pools=4:frame-threads=2" {
		t.Errorf("expected pools=4:frame-threads=2, got %q", got[1])
	}
}

func TestSetX265Param_DoesNotMutateInput(t *testing.T) {
	in := []string{"-x265-params", "frame-threads=8"}
	_ = setX265Param(in, "frame-threads", "2")
	if in[1] != "frame-threads=8" {
		t.Errorf("expected input slice untouched, got %v", in)
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
