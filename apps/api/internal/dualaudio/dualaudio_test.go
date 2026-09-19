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
	// The base holds 8 s (50-58 s) the dub does not have. That stretch must be
	// left as a gap — it plays the base audio — not filled with repeated dub.
	endA, startB := al.Segments[0].BaseEnd, al.Segments[1].BaseStart
	if math.Abs(endA-50) > 2 || math.Abs(startB-58) > 2 {
		t.Errorf("join = %.1f -> %.1f s, want ~50 -> ~58", endA, startB)
	}
	// Two gaps: the 0.5 s the dub lacks at the very start, and the hole.
	if len(al.Gaps) != 2 {
		t.Fatalf("gaps = %+v, want the leading 0.5 s and the hole", al.Gaps)
	}
	if h := al.Gaps[1]; math.Abs(h.Start-endA) > 1e-6 || math.Abs(h.End-startB) > 1e-6 {
		t.Errorf("hole = %+v, want exactly %.2f -> %.2f", h, endA, startB)
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
	v := Validate(rendered, baseFeat, al.Segments, 6, DefaultTickSec)
	if v.Hits == 0 {
		t.Fatal("no validation window in place")
	}
	if v.Hits*4 < v.Eligible*3 {
		t.Errorf("only %d/%d validation windows in place on a clean fixture", v.Hits, v.Eligible)
	}
	// P95 is left out on purpose: this fixture's voice is loud against its music,
	// and a lone 10 s window can lock onto noise. One window is not evidence —
	// which is why the gate counts runs of three.
	if v.Median > 0.02 || v.OffSec > 0 {
		t.Errorf("residual median=%.0f ms off=%.0f s, want <=20 ms and 0 s", v.Median*1000, v.OffSec)
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

// Recurring music makes a few neighbouring windows agree on an absurd lag. They
// used to become a segment and — being last in order — get stretched to the end
// of the video. The monotonic chain must drop them.
func TestGroupDropsNonMonotonicMismatch(t *testing.T) {
	const wm = 3000
	var est []estimate
	for f := 0; f < 60000; f += 1000 {
		est = append(est, estimate{f, -5562, 27}) // the real alignment: lag -55.62 s
	}
	// Two 3-window mismatches in the middle of it, as measured on a real episode.
	spurious := []estimate{{3000, 50629, 12}, {4000, 50629, 12}, {5000, 50629, 12}}
	est = append(est[:6], append(spurious, est[6:]...)...)

	chain, _ := chainGroups(est, 8, wm, 117760)
	locked := 0
	for _, g := range chain {
		for _, w := range g {
			locked++
			if w.lag != -5562 {
				t.Errorf("mismatched window survived in the chain: %+v", w)
			}
		}
	}
	if locked != 60 {
		t.Errorf("chain holds %d windows, want the 60 real ones only", locked)
	}
}

func TestGroupDropsLightFarGroup(t *testing.T) {
	const wm = 3000
	var est []estimate
	for f := 0; f < 40000; f += 1000 {
		est = append(est, estimate{f, 100, 30})
	}
	// Monotonic, but light (2 windows) and 237 s away from the only heavy group.
	est = append(est, estimate{41000, 23800, 9}, estimate{42000, 23800, 9})
	chain, _ := chainGroups(est, 8, wm, 200000)
	if len(chain) != 1 || len(chain[0]) != 40 {
		t.Fatalf("chain = %d group(s), want only the heavy one", len(chain))
	}
}

func TestEdgeExtensionIsBounded(t *testing.T) {
	base, dub := fixture()
	// Keep only the middle of the dub measurable by muting its first 60 s.
	for i := 0; i < 60*featRate; i++ {
		dub[i] = 0
	}
	al := Align(ExtractFeatures(toPCM(base)), ExtractFeatures(toPCM(dub)),
		float64(len(base))/featRate, float64(len(dub))/featRate, testOptions())
	if len(al.Segments) == 0 {
		t.Fatal("nothing aligned")
	}
	if al.Segments[0].BaseStart < 5 {
		t.Errorf("first segment starts at %.1f s: extended over a stretch that was never measured", al.Segments[0].BaseStart)
	}
}

// A stretch shorter than the sweep can resolve never yields two agreeing
// windows, so its lag is never proposed and it plays at a neighbour's offset.
// The closed loop has to recover it from what validation measures.
func TestRefineRecoversSegmentTooShortForTheSweep(t *testing.T) {
	// Seed chosen so the synthetic score is not accidentally self-similar at a
	// 0.3 s shift inside the short stretch (random tone bursts sometimes are, and
	// then a wrong lag honestly out-scores the true one).
	m := music(120, 37)
	base := mix(m, voice(len(m), 137))
	// lag +0.5 s up to 60 s, +0.9 s for the next 12 s, +1.3 s after that.
	var dm []float64
	dm = append(dm, m[int(0.5*featRate):60*featRate]...)
	dm = append(dm, m[int(60.4*featRate):72*featRate]...)
	dm = append(dm, m[int(72.4*featRate):]...)
	dub := mix(dm, voice(len(dm), 237))

	opt := Options{WindowSec: 20, HopSec: 10, SearchSec: 15, MinConfidence: 6, ToleranceSec: 0.08}
	baseFeat, dubFeat := ExtractFeatures(toPCM(base)), ExtractFeatures(toPCM(dub))
	al := Align(baseFeat, dubFeat, float64(len(base))/featRate, float64(len(dub))/featRate, opt)
	_, v := Refine(al, baseFeat, dubFeat, upsampleStereo(toPCM(dub)), upsampleStereo(toPCM(base)), GapFillBase, 6, DefaultTickSec)

	if v.OffSec > 0 {
		t.Errorf("%.0f s still playing at a wrong offset after refinement", v.OffSec)
	}
	// Judge by what matters: how long the dub plays at the wrong offset, against
	// the known truth. Splitting the short stretch between its neighbours — what
	// the coarse sweep alone does — leaves ~11 s wrong.
	truth := func(t float64) float64 {
		switch {
		case t < 60.4:
			return 0.5
		case t < 72.4:
			return 0.9
		}
		return 1.3
	}
	wrong := 0.0
	for t := 1.0; t < 119; t += 0.5 {
		for _, s := range al.Segments {
			if s.BaseStart <= t && t < s.BaseEnd && math.Abs(s.Lag-truth(t)) > 0.06 {
				wrong += 0.5
			}
		}
	}
	if wrong > 3 {
		t.Errorf("%.1f s at the wrong offset, want <= 3: %+v", wrong, al.Segments)
	}
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
	if s, notes := grade(good, Validation{Median: 0.001, P95: 0.02, Hits: 40, Eligible: 41}, DefaultGate()); s != StatusOK {
		t.Errorf("clean episode graded %s: %v", s, notes)
	}
	cases := map[string]struct {
		al *Alignment
		v  Validation
	}{
		"low coverage":                           {&Alignment{Coverage: 0.27, WindowsTotal: 10, WindowsLocked: 10}, Validation{Median: 0.001, P95: 0.01, Hits: 10, Eligible: 10}},
		"median residual":                        {good, Validation{Median: 0.083, P95: 0.1, Hits: 40, Eligible: 41}},
		"p95 residual":                           {good, Validation{Median: 0.003, P95: 0.907, Hits: 40, Eligible: 41}},
		"nothing to validate":                    {good, Validation{}},
		"misplaced stretch unseen by the median": {good, Validation{Median: 0.002, P95: 0.01, Hits: 18, Eligible: 69}},
		"short stretch at a neighbour's offset":  {good, Validation{Median: 0.001, P95: 0.02, Hits: 232, Eligible: 243, OffSec: 30}},
		"few windows":                            {&Alignment{Coverage: 0.95, WindowsTotal: 100, WindowsLocked: 30}, Validation{Median: 0.001, P95: 0.01, Hits: 10, Eligible: 10}},
		"drift":                                  {&Alignment{Coverage: 0.95, WindowsTotal: 10, WindowsLocked: 10, DriftSuspected: true}, Validation{Median: 0.001, P95: 0.01, Hits: 10, Eligible: 10}},
	}
	for name, c := range cases {
		if s, _ := grade(c.al, c.v, DefaultGate()); s != StatusReview {
			t.Errorf("%s: graded %s, want review", name, s)
		}
	}
}

// A seam 30 s early, with both lags already right, is invisible to the solver:
// re-solving returns the same map, so the refine loop spins without improving.
// It was worth 35 s at 0.33 s off in one Pokemon episode, graded "review" every
// round. The validation says where the cut is; seamMove is what reads it.
func TestSeamMoveReadsAMisplacedSeam(t *testing.T) {
	// The Pokemon episode, to the second: the solver switched at 1194 s, the
	// cut is past 1230, and six confident windows in between all say 0.33 s.
	segs := []Segment{
		{BaseStart: 790, BaseEnd: 1194, Lag: 10.87},
		{BaseStart: 1194, BaseEnd: 1262, Lag: 11.18},
	}
	i, at, ok := seamMove(segs, OffRun{Start: 1195, End: 1230, Lag: -0.33, Windows: 6})
	if !ok {
		t.Fatal("run not read as a misplaced seam")
	}
	if i != 0 || math.Abs(at-1230) > 0.01 {
		t.Errorf("seam = segment %d at %.1f s, want segment 0 at 1230", i, at)
	}
}

// The mirror case: the run closes a segment and belongs to the next one.
func TestSeamMoveHandlesARunThatClosesASegment(t *testing.T) {
	segs := []Segment{
		{BaseStart: 0, BaseEnd: 600, Lag: 1.0},
		{BaseStart: 600, BaseEnd: 1200, Lag: 5.0},
	}
	i, at, ok := seamMove(segs, OffRun{Start: 560, End: 600, Lag: 4.0, Windows: 6})
	if !ok || i != 0 || math.Abs(at-560) > 0.01 {
		t.Errorf("seam = segment %d at %.1f s (ok=%v), want segment 0 at 560", i, at, ok)
	}
}

func TestSeamMoveLeavesUnrelatedRunsAlone(t *testing.T) {
	segs := []Segment{
		{BaseStart: 0, BaseEnd: 600, Lag: 1.0},
		{BaseStart: 600, BaseEnd: 1200, Lag: 5.0},
	}
	cases := map[string]OffRun{
		// Implied lag is nobody's: a real fault, and moving a seam would bury it.
		"implied lag matches no neighbour": {Start: 605, End: 640, Lag: -2.0, Windows: 6},
		// Starts well inside the segment, so a misplaced seam is not the fault.
		"run does not touch either edge": {Start: 800, End: 835, Lag: -4.0, Windows: 6},
		// Would leave nothing of the segment it moves into.
		"would collapse a segment": {Start: 601, End: 1199.5, Lag: -4.0, Windows: 6},
	}
	for name, r := range cases {
		if i, at, ok := seamMove(segs, r); ok {
			t.Errorf("%s: moved segment %d to %.1f s, want no move", name, i, at)
		}
	}
}

// SnapSeams edits the map in place and keeps the lags. A FALLING lag is used so
// the seam lands exactly where the evidence puts it: a rising one hands the
// join to refineBoundary, which is the existing, separately tested path.
func TestSnapSeamsMovesTheJoinAndKeepsLags(t *testing.T) {
	al := &Alignment{
		BaseDuration: 1300,
		Segments: []Segment{
			{BaseStart: 790, BaseEnd: 1194, Lag: 11.18},
			{BaseStart: 1194, BaseEnd: 1262, Lag: 10.87},
		},
	}
	if !al.SnapSeams(nil, nil, []OffRun{{Start: 1195, End: 1230, Lag: 0.31, Windows: 6}}) {
		t.Fatal("seam did not move")
	}
	if got := al.Segments[0].BaseEnd; math.Abs(got-1230) > 0.01 {
		t.Errorf("join at %.1f s, want 1230", got)
	}
	if al.Segments[1].BaseStart != al.Segments[0].BaseEnd {
		t.Error("segments left with a hole the lag does not call for")
	}
	if al.Segments[0].Lag != 11.18 || al.Segments[1].Lag != 10.87 {
		t.Error("lags changed; only the seam may move")
	}
}
