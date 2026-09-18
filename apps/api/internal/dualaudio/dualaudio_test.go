package dualaudio

import (
	"math"
	"math/rand"
	"testing"
)

// music synthesises note-like bursts: decaying tones at random times and
// pitches. It stands in for the music-and-effects master that two dubs share.
func music(seconds float64, seed int64) []float64 {
	r := rand.New(rand.NewSource(seed))
	x := make([]float64, int(seconds*featRate))
	for t := 0.0; t < seconds; t += 0.08 + r.Float64()*0.25 {
		freq := 150 + r.Float64()*3000
		amp := 0.2 + r.Float64()*0.5
		start := int(t * featRate)
		for i := 0; i < featRate/4 && start+i < len(x); i++ {
			ts := float64(i) / featRate
			x[start+i] += amp * math.Exp(-ts*18) * math.Sin(2*math.Pi*freq*ts)
		}
	}
	return x
}

// voice is filtered noise gated into syllable-like bursts of random length,
// different per seed: the part of a dub that does NOT match the other language.
// The gating has to be random too — an earlier version used a fixed 0.7 Hz
// envelope, which both tracks then shared, and the aligner dutifully locked
// onto its 1.43 s period.
func voice(n int, seed int64) []float64 {
	r := rand.New(rand.NewSource(seed))
	x := make([]float64, n)
	var lp, gain float64
	next := 0
	for i := range x {
		if i >= next {
			gain = 0
			if r.Float64() < 0.7 {
				gain = 0.5 + r.Float64()
			}
			next = i + int((0.08+r.Float64()*0.4)*featRate)
		}
		lp = 0.97*lp + 0.03*(r.Float64()*2-1)
		x[i] = lp * gain * 1.5
	}
	return x
}

func toPCM(x []float64) []int16 {
	out := make([]int16, len(x))
	for i, v := range x {
		out[i] = int16(math.Max(-1, math.Min(1, v)) * 30000)
	}
	return out
}

func mix(a, b []float64) []float64 {
	out := make([]float64, len(a))
	for i := range a {
		out[i] = a[i] + b[i]
	}
	return out
}

// fixture builds a base and a dub that share music but not voices. The dub
// drops the first 0.5 s and then an 8 s stretch at base 50-58 s, so the true map
// is lag +0.5 s up to 50 s and lag +8.5 s from 58 s, with a hole in between.
func fixture() (base, dub []float64) {
	m := music(120, 1)
	base = mix(m, voice(len(m), 2))
	var dm []float64
	dm = append(dm, m[int(0.5*featRate):50*featRate]...)
	dm = append(dm, m[58*featRate:]...)
	return base, mix(dm, voice(len(dm), 3))
}

func testOptions() Options {
	return Options{WindowSec: 10, HopSec: 5, SearchSec: 15, MinConfidence: 6, ToleranceSec: 0.08}
}

func TestCrossCorrelateMatchesNaive(t *testing.T) {
	r := rand.New(rand.NewSource(7))
	ref := make([]float64, 300)
	win := make([]float64, 40)
	for i := range ref {
		ref[i] = r.Float64() - 0.5
	}
	for i := range win {
		win[i] = r.Float64() - 0.5
	}
	got := crossCorrelate(ref, win)
	if len(got) != len(ref)-len(win)+1 {
		t.Fatalf("len = %d, want %d", len(got), len(ref)-len(win)+1)
	}
	for k := range got {
		var want float64
		for i := range win {
			want += ref[k+i] * win[i]
		}
		if math.Abs(got[k]-want) > 1e-9 {
			t.Fatalf("k=%d: got %g want %g", k, got[k], want)
		}
	}
}

func TestAlignRecoversStepAndCut(t *testing.T) {
	base, dub := fixture()
	al := Align(ExtractFeatures(toPCM(base)), ExtractFeatures(toPCM(dub)),
		float64(len(base))/featRate, float64(len(dub))/featRate, testOptions())

	if len(al.Segments) != 2 {
		t.Fatalf("segments = %d, want 2: %+v", len(al.Segments), al.Segments)
	}
	for i, want := range []float64{0.5, 8.5} {
		if got := al.Segments[i].Lag; math.Abs(got-want) > 0.03 {
			t.Errorf("segment %d lag = %.3f, want %.3f", i, got, want)
		}
	}
	// The join has to land inside the stretch the dub does not have; outside it
	// one side would play at the other's offset.
	if cut := al.Segments[0].BaseEnd; cut < 48 || cut > 60 {
		t.Errorf("boundary at %.1f s, want within the 50-58 s cut", cut)
	}
	if al.DriftSuspected {
		t.Error("constant offsets reported as drift")
	}
}

func TestRenderedTrackHasNoResidual(t *testing.T) {
	base, dub := fixture()
	baseFeat := ExtractFeatures(toPCM(base))
	al := Align(baseFeat, ExtractFeatures(toPCM(dub)),
		float64(len(base))/featRate, float64(len(dub))/featRate, testOptions())

	rendered := Render(al, upsampleStereo(toPCM(dub)), upsampleStereo(toPCM(base)), GapFillBase)
	med, p95, n := Residual(rendered, baseFeat, al.Segments, 6)
	if n == 0 {
		t.Fatal("no validation window locked")
	}
	if med > 0.02 || p95 > 0.05 {
		t.Errorf("residual median=%.0f ms p95=%.0f ms, want <=20/50", med*1000, p95*1000)
	}
	if want := int(math.Round(al.BaseDuration*outRate)) * outChannels; len(rendered) != want {
		t.Errorf("rendered length = %d, want %d (base timeline)", len(rendered), want)
	}
}

// upsampleStereo repeats samples to reach the render rate; good enough to
// exercise placement, which is what these tests are about.
func upsampleStereo(mono []int16) []int16 {
	const ratio = outRate / featRate
	out := make([]int16, 0, len(mono)*ratio*outChannels)
	for _, s := range mono {
		for i := 0; i < ratio*outChannels; i++ {
			out = append(out, s)
		}
	}
	return out
}

func TestCoverageIsUnionNotSum(t *testing.T) {
	segs := []Segment{{BaseStart: 0, BaseEnd: 60}, {BaseStart: 50, BaseEnd: 100}}
	gaps, cov := gapsAndCoverage(segs, 100)
	if math.Abs(cov-1) > 1e-9 {
		t.Errorf("coverage = %.3f, want 1.0 (overlap must not double count)", cov)
	}
	if len(gaps) != 0 {
		t.Errorf("gaps = %+v, want none", gaps)
	}

	gaps, cov = gapsAndCoverage([]Segment{{BaseStart: 10, BaseEnd: 40}}, 100)
	if math.Abs(cov-0.3) > 1e-9 || len(gaps) != 2 {
		t.Errorf("coverage=%.3f gaps=%+v, want 0.3 and two gaps", cov, gaps)
	}
}

func TestDropIsolatedRemovesLoneOutlier(t *testing.T) {
	est := []estimate{{0, 5, 30}, {500, 5, 30}, {1000, 27400, 40}, {1500, 5, 30}, {2000, 5, 30}}
	got := dropIsolated(est, 32)
	if len(got) != 4 {
		t.Fatalf("kept %d windows, want 4", len(got))
	}
	for _, w := range got {
		if w.lag != 5 {
			t.Errorf("outlier survived: %+v", w)
		}
	}
}

func TestBestMatchPairsByContentNotNumber(t *testing.T) {
	// Base "e52" is really the dub numbered 49 — the situation that makes
	// pairing by number wrong.
	fp := func(seed int64, voiceSeed int64) *Features {
		m := music(90, seed)
		return Fingerprint(ExtractFeatures(toPCM(mix(m, voice(len(m), voiceSeed)))))
	}
	dubs := map[string]*Features{"49": fp(10, 101), "52": fp(11, 102), "53": fp(12, 103)}

	p := BestMatch("e52", fp(10, 201), dubs, 1.6)
	if p.Dub != "49" {
		t.Fatalf("paired with %q (margin %.1f), want 49", p.Dub, p.Margin)
	}

	// No counterpart at all: refuse rather than pick the least bad.
	if p := BestMatch("e65", fp(99, 202), dubs, 1.6); p.Dub != "" {
		t.Errorf("unrelated episode paired with %q (margin %.1f), want refusal", p.Dub, p.Margin)
	}
}

func TestDecidePairsMutualBestRescuesLowMargin(t *testing.T) {
	bases := []string{"b1", "b2", "b3"}
	dubs := []string{"d1", "d2"}
	conf := [][]float64{
		{10, 8},  // margin 1.25 — below 1.6, but d1's best base is b1: accept
		{3, 20},  // clear winner on margin alone
		{5, 5.2}, // no counterpart: its least-bad dub belongs to b2: refuse
	}
	got := decidePairs(bases, dubs, conf, 1.6)
	if got[0].Dub != "d1" {
		t.Errorf("mutual best at 1.25x refused: %+v", got[0])
	}
	if got[1].Dub != "d2" {
		t.Errorf("clear pair refused: %+v", got[1])
	}
	if got[2].Dub != "" {
		t.Errorf("episode without counterpart was paired with %q", got[2].Dub)
	}
}

func TestFlagContested(t *testing.T) {
	pairs := []Pair{{Base: "a", Dub: "x"}, {Base: "b", Dub: "x"}, {Base: "c", Dub: "y"}}
	FlagContested(pairs)
	if pairs[0].Note == "" || pairs[1].Note == "" || pairs[2].Note != "" {
		t.Errorf("contested flags wrong: %+v", pairs)
	}
}

func TestGrade(t *testing.T) {
	good := &Alignment{Coverage: 0.96, WindowsTotal: 100, WindowsLocked: 100}
	if s, notes := grade(good, 0.001, 0.02, 40, DefaultGate()); s != StatusOK {
		t.Errorf("clean episode graded %s: %v", s, notes)
	}
	cases := map[string]struct {
		al       *Alignment
		med, p95 float64
		n        int
	}{
		"low coverage":        {&Alignment{Coverage: 0.27, WindowsTotal: 10, WindowsLocked: 10}, 0.001, 0.01, 10},
		"median residual":     {good, 0.083, 0.1, 40},
		"p95 residual":        {good, 0.003, 0.907, 40},
		"nothing to validate": {good, 0, 0, 0},
		"few windows":         {&Alignment{Coverage: 0.95, WindowsTotal: 100, WindowsLocked: 30}, 0.001, 0.01, 10},
		"drift":               {&Alignment{Coverage: 0.95, WindowsTotal: 10, WindowsLocked: 10, DriftSuspected: true}, 0.001, 0.01, 10},
	}
	for name, c := range cases {
		if s, _ := grade(c.al, c.med, c.p95, c.n, DefaultGate()); s != StatusReview {
			t.Errorf("%s: graded %s, want review", name, s)
		}
	}
}
