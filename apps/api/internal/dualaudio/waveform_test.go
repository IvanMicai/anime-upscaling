package dualaudio

import (
	"math"
	"math/rand"
	"testing"
)

func TestMatchStereoWaveformFindsSharedSound(t *testing.T) {
	rng := rand.New(rand.NewSource(941))
	base, dub := make([]int16, 10*featRate*2), make([]int16, 10*featRate*2)
	for i := range base {
		base[i] = int16(rng.Intn(20001) - 10000)
		dub[i] = int16(rng.Intn(20001) - 10000)
	}
	shift := int(.263 * featRate)
	for i := 0; i < 3*featRate; i++ {
		for c := 0; c < 2; c++ {
			dub[2*(3*featRate+i)+c] = base[2*(3*featRate+shift+i)+c]
		}
	}
	got, err := MatchStereoWaveform(base, dub, 3, 3)
	if err != nil {
		t.Fatal(err)
	}
	if !got.Accepted || math.Abs(got.OffsetSec-.263) > .001 {
		t.Fatalf("shared sound not found: %+v", got)
	}
}
func TestMatchStereoWaveformRejectsAmbiguityAndSilence(t *testing.T) {
	base, dub := make([]int16, 10*featRate*2), make([]int16, 10*featRate*2)
	got, err := MatchStereoWaveform(base, dub, 3, 3)
	if err != nil || got.Accepted {
		t.Fatalf("silence accepted: %+v %v", got, err)
	}
	for i := 0; i < len(base)/2; i++ {
		v := int16(10000 * math.Sin(2*math.Pi*200*float64(i)/featRate))
		base[2*i], base[2*i+1] = v, v
		dub[2*i], dub[2*i+1] = v, v
	}
	got, err = MatchStereoWaveform(base, dub, 3, 3)
	if err != nil || got.Accepted {
		t.Fatalf("periodic sound accepted: %+v %v", got, err)
	}
	if _, err = MatchStereoWaveform(base, dub, 0, 3); err == nil {
		t.Fatal("out-of-range base search accepted")
	}
}
func TestMatchStereoWaveformRejectsChannelsWithDifferentOffsets(t *testing.T) {
	rng := rand.New(rand.NewSource(942))
	base, dub := make([]int16, 10*featRate*2), make([]int16, 10*featRate*2)
	for i := range base {
		base[i] = int16(rng.Intn(20001) - 10000)
		dub[i] = int16(rng.Intn(20001) - 10000)
	}
	for i := 0; i < 3*featRate; i++ {
		dub[2*(3*featRate+i)] = base[2*(3*featRate+1600+i)]
		dub[2*(3*featRate+i)+1] = base[2*(3*featRate+4800+i)+1]
	}
	got, err := MatchStereoWaveform(base, dub, 3, 3)
	if err != nil || got.Accepted {
		t.Fatalf("inconsistent channels accepted: %+v %v", got, err)
	}
}

func TestProbeVisualWaveformRequiresContinuousConsistentVisualEvidence(t *testing.T) {
	pcm := make([]int16, 10*featRate*2)
	for _, timeline := range []*VisualTimeline{
		{Windows: []VisualWindow{{Start: 2, End: 4}, {Start: 5, End: 7}}},
		{Windows: []VisualWindow{{Start: 2, End: 4}, {Start: 4, End: 6, Lag: .1}}},
		{Windows: []VisualWindow{{Start: 2, End: 4}, {Start: 4, End: 6}}, Rejected: []Gap{{Start: 3, End: 4}}},
	} {
		if _, err := ProbeVisualWaveform(timeline, pcm, pcm, 2.5); err == nil {
			t.Fatalf("invalid visual evidence accepted: %+v", timeline)
		}
	}
}

func TestFindWaveformVisualDisagreementsRequiresSeparateConsistentProbes(t *testing.T) {
	p := func(t, offset float64) WaveformEvidence {
		return WaveformEvidence{BaseStart: t, OffsetSec: offset, Accepted: true}
	}
	probes := []WaveformEvidence{p(17, -.256), p(5, -.262), p(9, -.308), p(13, -.308), p(21, -.257), p(25, -.249), p(29, -.249), p(5.5, -.262)}
	got := FindWaveformVisualDisagreements(probes)
	if len(got) != 1 || got[0].Start != 5 || got[0].End != 32 || got[0].Windows != 7 {
		t.Fatalf("unexpected groups: %+v", got)
	}
	if got := FindWaveformVisualDisagreements([]WaveformEvidence{p(1, -.3), p(2, -.3), p(3, -.3)}); len(got) != 0 {
		t.Fatalf("overlapping probes counted: %+v", got)
	}
	if got := FindWaveformVisualDisagreements([]WaveformEvidence{p(1, -.3), p(5, -.3), p(9, .3)}); len(got) != 0 {
		t.Fatalf("conflicting signs combined: %+v", got)
	}
}
