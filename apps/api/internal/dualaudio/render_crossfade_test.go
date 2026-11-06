package dualaudio

import "testing"

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
