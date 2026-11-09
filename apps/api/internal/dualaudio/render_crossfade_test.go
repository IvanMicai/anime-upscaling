package dualaudio

import (
	"math"
	"testing"
)

func TestRenderCrossfadeDoesNotDoubleIdenticalAdjacentAudio(t *testing.T) {
	pcm := make([]int16, 4*outRate*outChannels)
	for i := range pcm {
		pcm[i] = 16000
	}
	a := &Alignment{BaseDuration: 4, Segments: []Segment{{BaseStart: 0, BaseEnd: 2}, {BaseStart: 2, BaseEnd: 4}}}
	out := Render(a, pcm, nil, GapFillSilence)
	for frame := 2*outRate - 1500; frame < 2*outRate+1500; frame++ {
		if v := out[frame*2]; v < 15998 || v > 16001 {
			t.Fatalf("frame %d: got %d, want unchanged level 16000", frame, v)
		}
	}
}

func TestRenderFillsShortUnlistedGapsWithoutGainSpikes(t *testing.T) {
	for _, gapFrames := range []int{1, 480, 4800} {
		// Simulate an older saved alignment whose short gaps were omitted.
		dub, base := make([]int16, 3*outRate*outChannels), make([]int16, 3*outRate*outChannels)
		for i := range dub {
			dub[i] = 10000
			base[i] = 20000
		}
		end := float64(2*outRate+gapFrames) / outRate
		a := &Alignment{BaseDuration: 3, Segments: []Segment{{BaseStart: 0, BaseEnd: 2}, {BaseStart: end, BaseEnd: 3}}}
		out := Render(a, dub, base, GapFillBase)
		for frame := 2*outRate - 1100; frame < 2*outRate+gapFrames+1100; frame++ {
			if v := out[frame*2]; v < 9998 || v > 20001 {
				t.Fatalf("gap %d, frame %d: unexpected level %d", gapFrames, frame, v)
			}
		}
		if gapFrames >= 4800 && out[(2*outRate+gapFrames/2)*2] != 20000 {
			t.Fatal("gap center did not use base audio")
		}
	}
}

func TestRenderDoesNotOverlapBothFadesAcrossShortSegment(t *testing.T) {
	pcm := make([]int16, 3*outRate*outChannels)
	for i := range pcm {
		pcm[i] = 16000
	}
	a := &Alignment{BaseDuration: 3, Segments: []Segment{{BaseStart: 0, BaseEnd: 1}, {BaseStart: 1, BaseEnd: 1.01}, {BaseStart: 1.01, BaseEnd: 3}}}
	out := Render(a, pcm, nil, GapFillSilence)
	for frame := outRate - 1100; frame < outRate+1600; frame++ {
		if v := out[frame*2]; v < 15998 || v > 16001 {
			t.Fatalf("frame %d: level %d", frame, v)
		}
	}
}

func TestRenderLeavesShortGapSilentWhenRequested(t *testing.T) {
	dub := make([]int16, 3*outRate*outChannels)
	for i := range dub {
		dub[i] = 10000
	}
	a := &Alignment{BaseDuration: 3, Segments: []Segment{{BaseStart: 0, BaseEnd: 1}, {BaseStart: 1.1, BaseEnd: 3}}}
	out := Render(a, dub, nil, GapFillSilence)
	if got := out[int(1.05*outRate)*2]; got != 0 {
		t.Fatalf("gap center is not silent: %d", got)
	}
}

func TestGapsAndCoveragePreservesSubquarterSecondHoles(t *testing.T) {
	segs := []Segment{{BaseStart: .01, BaseEnd: 1}, {BaseStart: 1.1, BaseEnd: 1.99}}
	gaps, coverage := gapsAndCoverage(segs, 2)
	if len(gaps) != 3 || math.Abs(gaps[1].Start-1) > .000001 || math.Abs(gaps[1].End-1.1) > .000001 || math.Abs(coverage-.94) > .000001 {
		t.Fatalf("gaps %+v, coverage %v", gaps, coverage)
	}
	// A difference smaller than one rounded output sample is not audible and
	// must not create a zero-length PCM span.
	gaps, _ = gapsAndCoverage([]Segment{{BaseStart: 0, BaseEnd: 1}, {BaseStart: 1 + 0.1/outRate, BaseEnd: 2}}, 2)
	if len(gaps) != 0 {
		t.Fatalf("spurious sub-sample gap: %+v", gaps)
	}
}

func TestRenderCrossfadeBetweenDubAndGapStaysWithinSourceLevels(t *testing.T) {
	dub, base := make([]int16, 4*outRate*outChannels), make([]int16, 4*outRate*outChannels)
	for i := range dub {
		dub[i] = 10000
		base[i] = 20000
	}
	a := &Alignment{BaseDuration: 4, Segments: []Segment{{BaseStart: 0, BaseEnd: 2}}, Gaps: []Gap{{Start: 2, End: 4}}}
	out := Render(a, dub, base, GapFillBase)
	for frame := 2*outRate - 1500; frame < 2*outRate+1500; frame++ {
		if v := out[frame*2]; v < 9998 || v > 20001 {
			t.Fatalf("frame %d: crossfade overshoot %d", frame, v)
		}
	}
}
