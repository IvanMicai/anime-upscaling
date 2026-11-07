package dualaudio

import (
	"context"
	"math"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestWaveformProbeSelectsReferenceTrack(t *testing.T) {
	ffmpeg, err := exec.LookPath("ffmpeg")
	if err != nil {
		t.Skip("ffmpeg unavailable")
	}
	ffprobe, err := exec.LookPath("ffprobe")
	if err != nil {
		t.Skip("ffprobe unavailable")
	}
	path := filepath.Join(t.TempDir(), "tracks.mka")
	cmd := exec.Command(ffmpeg, "-v", "error", "-f", "lavfi", "-i", "anoisesrc=sample_rate=16000:duration=10:seed=47", "-f", "lavfi", "-i", "anoisesrc=sample_rate=16000:duration=10:seed=91", "-map", "0:a", "-map", "1:a", "-c:a", "pcm_s16le", path)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("create tracks: %v: %s", err, out)
	}
	tools := Tools{FFmpeg: ffmpeg, FFprobe: ffprobe}
	matched, err := ProbeStereoWaveformTracks(context.Background(), tools, path, path, 1, 1, 4, 4, 1)
	if err != nil || !matched.Accepted || math.Abs(matched.OffsetSec) > .003 {
		t.Fatalf("selected track: %+v, %v", matched, err)
	}
	wrong, err := ProbeStereoWaveformTracks(context.Background(), tools, path, path, 0, 1, 4, 4, 1)
	if err != nil || wrong.Accepted {
		t.Fatalf("unrelated tracks: %+v, %v", wrong, err)
	}
	if _, err := ProbeStereoWaveformTracks(context.Background(), tools, path, path, -1, 1, 4, 4, 1); err == nil {
		t.Fatal("negative reference track accepted")
	}
}

func TestWaveformGapProposesOnlySharedSound(t *testing.T) {
	ffmpeg, err := exec.LookPath("ffmpeg")
	if err != nil {
		t.Skip("ffmpeg unavailable")
	}
	ffprobe, err := exec.LookPath("ffprobe")
	if err != nil {
		t.Skip("ffprobe unavailable")
	}
	makeAudio := func(seed string) string {
		p := filepath.Join(t.TempDir(), "source.wav")
		c := exec.Command(ffmpeg, "-v", "error", "-f", "lavfi", "-i", "anoisesrc=sample_rate=16000:duration=20:seed="+seed, "-ac", "2", p)
		if out, err := c.CombinedOutput(); err != nil {
			t.Fatalf("create sound: %v: %s", err, out)
		}
		return p
	}
	base, unrelated := makeAudio("47"), makeAudio("91")
	a := &Alignment{BaseDuration: 20, DubDuration: 20, Segments: []Segment{{BaseStart: 4, BaseEnd: 8}, {BaseStart: 10, BaseEnd: 16}}, Gaps: []Gap{{Start: 0, End: 4}, {Start: 8, End: 10}, {Start: 16, End: 20}}}
	v := &VisualTimeline{Windows: []VisualWindow{{Start: 6, End: 8}, {Start: 10, End: 12}}, Rejected: []Gap{{Start: 8, End: 10}}}
	tools := Tools{FFmpeg: ffmpeg, FFprobe: ffprobe}
	p, err := ProposeWaveformGap(context.Background(), tools, base, base, 0, a, v, Gap{Start: 8, End: 10})
	if err != nil {
		t.Fatal(err)
	}
	if !p.VisualRejected || len(p.Alignment.Segments) != 3 || p.Alignment.Segments[1].BaseStart != 8 || p.Alignment.Segments[1].BaseEnd != 10 || len(a.Segments) != 2 {
		t.Fatalf("bad candidate or input mutated: %+v", p)
	}
	if _, err = ProposeWaveformGap(context.Background(), tools, base, unrelated, 0, a, v, Gap{Start: 8, End: 10}); err == nil {
		t.Fatal("unrelated audio accepted")
	}
	delayed := filepath.Join(t.TempDir(), "delayed.wav")
	cmd := exec.Command(ffmpeg, "-v", "error", "-i", base, "-af", "adelay=250|250,atrim=0:20", delayed)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("create delayed sound: %v: %s", err, out)
	}
	a.Segments[0].Lag, a.Segments[1].Lag = -.2, -.2
	v.Windows[0].Lag, v.Windows[1].Lag = -.2, -.2
	p, err = ProposeWaveformGap(context.Background(), tools, base, delayed, 0, a, v, Gap{Start: 8, End: 10})
	if err != nil {
		t.Fatal(err)
	}
	if math.Abs(p.Alignment.Segments[1].Lag+.25) > .003 {
		t.Fatalf("wrong lag sign or magnitude: %+v", p.Probe)
	}
}

func TestWaveformGapRequiresBoundedConsistentVisualAnchors(t *testing.T) {
	a := &Alignment{BaseDuration: 30, DubDuration: 40, Segments: []Segment{{BaseStart: 8, BaseEnd: 10, Lag: -1}, {BaseStart: 12, BaseEnd: 16, Lag: -1.04}}, Gaps: []Gap{{Start: 0, End: 8}, {Start: 10, End: 12}, {Start: 16, End: 30}}}
	v := &VisualTimeline{Windows: []VisualWindow{{Start: 8, End: 10, Lag: -1, Contextual: true, FineRefined: true}, {Start: 12, End: 14, Lag: -1.04}}, Rejected: []Gap{{Start: 10, End: 12}}}
	i, seed, rejected, err := waveformGapAnchors(a, v, Gap{Start: 10, End: 12})
	if err != nil || i != 0 || seed < -1.021 || seed > -1.019 || !rejected {
		t.Fatalf("unexpected result: %d %f %v %v", i, seed, rejected, err)
	}
	v.Windows[0].FineRefined = false
	if _, _, _, err := waveformGapAnchors(a, v, Gap{Start: 10, End: 12}); err == nil {
		t.Fatal("contextual-only anchor accepted")
	}
	v.Windows[0].FineRefined = true
	a.Segments[1].Lag = -2
	if _, _, _, err := waveformGapAnchors(a, v, Gap{Start: 10, End: 12}); err == nil {
		t.Fatal("inconsistent neighbors accepted")
	}
	a.Segments[1].Lag = -1.04
	if _, _, _, err := waveformGapAnchors(a, v, Gap{Start: 10, End: 13}); err == nil {
		t.Fatal("too-long gap accepted")
	}
	if _, _, _, err := waveformGapAnchors(a, v, Gap{Start: 10, End: 11}); err == nil {
		t.Fatal("unbounded gap accepted")
	}
}
