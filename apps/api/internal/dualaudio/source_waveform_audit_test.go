package dualaudio

import (
	"context"
	"math/rand"
	"testing"
)

func TestSourceWaveformAuditUsesAdjacentSeedsAndRejectsUnrelatedAudio(t *testing.T) {
	r := rand.New(rand.NewSource(1024))
	audio := make([]int16, 20*featRate*2)
	wrong := make([]int16, len(audio))
	for i := range audio {
		audio[i] = int16(r.Intn(20001) - 10000)
		wrong[i] = int16(r.Intn(20001) - 10000)
	}
	a := &Alignment{BaseDuration: 20, DubDuration: 20, Segments: []Segment{{BaseStart: 0, BaseEnd: 8}, {BaseStart: 10, BaseEnd: 20}}, Gaps: []Gap{{Start: 8, End: 10}}}
	report, err := AuditSourceWaveform(context.Background(), audio, audio, a, 3)
	if err != nil {
		t.Fatal(err)
	}
	if len(report.Windows) == 0 {
		t.Fatal("no windows")
	}
	var seam bool
	for _, w := range report.Windows {
		if w.Status != "match" {
			t.Fatalf("matching source not found: %+v", w)
		}
		if w.BaseStart == 7.5 {
			seam = true
		}
	}
	if !seam {
		t.Fatal("seam window skipped")
	}
	other, err := AuditSourceWaveform(context.Background(), audio, wrong, a, 3)
	if err != nil {
		t.Fatal(err)
	}
	for _, w := range other.Windows {
		if w.Status == "match" {
			t.Fatalf("unrelated source accepted: %+v", w)
		}
	}
	if _, err = AuditSourceWaveform(context.Background(), audio, audio, a, 2); err == nil {
		t.Fatal("overlapping stride accepted")
	}
	if _, err = AuditSourceWaveform(context.Background(), audio[:len(audio)-1], audio, a, 3); err == nil {
		t.Fatal("incomplete stereo frame accepted")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err = AuditSourceWaveform(ctx, audio, audio, a, 3); err != context.Canceled {
		t.Fatalf("cancellation ignored: %v", err)
	}
}

func TestSourceWaveformAuditLeavesUnsupportedWindowsUnresolved(t *testing.T) {
	audio := make([]int16, 40*featRate*2)
	a := &Alignment{BaseDuration: 40, DubDuration: 40, Segments: []Segment{{BaseStart: 0, BaseEnd: 3}}}
	report, err := AuditSourceWaveform(context.Background(), audio, audio, a, 3)
	if err != nil {
		t.Fatal(err)
	}
	if report.Windows[len(report.Windows)-1].Status != "unanchored" {
		t.Fatalf("distant gap treated as supported: %+v", report.Windows[len(report.Windows)-1])
	}
	a.Segments = nil
	for i := 0; i < 5; i++ {
		start := 7.5 + float64(i)*.6
		a.Segments = append(a.Segments, Segment{BaseStart: start, BaseEnd: start + .6, Lag: float64(i) * .1})
	}
	report, err = AuditSourceWaveform(context.Background(), audio, audio, a, 3)
	if err != nil {
		t.Fatal(err)
	}
	for _, w := range report.Windows {
		if w.BaseStart == 7.5 {
			if w.Status != "too_many_seeds" {
				t.Fatalf("multiple offsets silently discarded: %+v", w)
			}
			return
		}
	}
	t.Fatal("seam window missing")
}
