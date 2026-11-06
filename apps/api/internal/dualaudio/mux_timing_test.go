package dualaudio

import (
	"context"
	"math"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestDecodePCMHonorsAudioStart(t *testing.T) {
	ffmpeg, err := exec.LookPath("ffmpeg")
	if err != nil {
		t.Skip("ffmpeg unavailable")
	}
	path := filepath.Join(t.TempDir(), "delayed.mkv")
	cmd := exec.Command(ffmpeg, "-v", "error", "-f", "lavfi", "-i", "color=size=64x64:rate=2:duration=4", "-itsoffset", "1", "-f", "lavfi", "-i", "sine=frequency=440:sample_rate=48000:duration=3", "-map", "0:v", "-map", "1:a", "-c:v", "mpeg4", "-c:a", "aac", "-t", "4", path)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("create delayed source: %v: %s", err, out)
	}
	pcm, err := (Tools{FFmpeg: ffmpeg}).DecodePCM(context.Background(), path, 16000, 1, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(pcm) < 3*16000 {
		t.Fatalf("decoded only %d samples", len(pcm))
	}
	var before, after float64
	for _, s := range pcm[:8000] {
		before = math.Max(before, math.Abs(float64(s)))
	}
	for _, s := range pcm[17600:24000] {
		after = math.Max(after, math.Abs(float64(s)))
	}
	if before > 20 || after < 100 {
		t.Fatalf("audio start timestamp lost: before=%f after=%f", before, after)
	}
}

// Exercise the actual native AAC/Matroska path; feature-level tests cannot
// observe encoder priming or timestamps written by the muxer.
func TestMuxKeepsDubAudioTiming(t *testing.T) {
	ffmpeg, err := exec.LookPath("ffmpeg")
	if err != nil {
		t.Skip("ffmpeg unavailable")
	}
	ffprobe, err := exec.LookPath("ffprobe")
	if err != nil {
		t.Skip("ffprobe unavailable")
	}
	dir := t.TempDir()
	base, output := filepath.Join(dir, "base.mkv"), filepath.Join(dir, "out.mkv")
	cmd := exec.Command(ffmpeg, "-v", "error", "-f", "lavfi", "-i", "color=size=64x64:rate=2:duration=16", "-f", "lavfi", "-i", "anullsrc=r=48000:cl=stereo", "-t", "16", "-c:v", "mpeg4", "-c:a", "aac", base)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("create base: %v: %s", err, out)
	}
	tools := Tools{FFmpeg: ffmpeg, FFprobe: ffprobe}
	pcm := toPCM(music(16, 71))
	opt := MuxOptions{BaseLang: "eng", DubLang: "por", Bitrate: "192k"}
	if err := tools.Mux(context.Background(), base, upsampleStereo(pcm), output, opt); err != nil {
		t.Fatal(err)
	}
	decoded, err := tools.DecodePCM(context.Background(), output, outRate, outChannels, 1)
	if err != nil {
		t.Fatal(err)
	}
	v := ValidateDubSource(decoded, ExtractFeatures(pcm), []Segment{{BaseStart: 1, BaseEnd: 15}}, 6)
	if v.Checked == 0 || v.Matched != v.Checked || v.Inconclusive != 0 || v.MedianSec > 0.006 {
		t.Fatalf("mux changed dubbed audio timing: %+v", v)
	}
}
