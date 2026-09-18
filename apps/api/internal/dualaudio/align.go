package dualaudio

import (
	"math"
	"sort"
)

// Segment maps a stretch of the base timeline onto the dub with a constant
// offset. Lag convention: tBase = tDub + Lag, so tDub = tBase - Lag.
type Segment struct {
	BaseStart  float64 `json:"base_start"`
	BaseEnd    float64 `json:"base_end"`
	Lag        float64 `json:"lag"`
	Confidence float64 `json:"confidence"`
	Windows    int     `json:"windows"`
}

// Gap is a stretch of the base timeline with no counterpart in the dub.
type Gap struct {
	Start float64 `json:"start"`
	End   float64 `json:"end"`
}

// Options tunes the analysis. The zero value is not usable; use DefaultOptions.
type Options struct {
	WindowSec     float64 // analysis window
	HopSec        float64 // step between windows
	SearchSec     float64 // search radius around the global prior
	MinConfidence float64 // windows below this are ignored
	ToleranceSec  float64 // lag spread allowed inside one segment
}

// DefaultOptions are the values the alignment was validated with: one 82-episode
// season, 74 of 82 episodes passing the quality gate.
func DefaultOptions() Options {
	return Options{WindowSec: 30, HopSec: 10, SearchSec: 30, MinConfidence: 6, ToleranceSec: 0.08}
}

// Alignment is the measured sync map between two recordings of one episode.
type Alignment struct {
	BaseDuration  float64   `json:"base_duration"`
	DubDuration   float64   `json:"dub_duration"`
	Segments      []Segment `json:"segments"`
	Gaps          []Gap     `json:"gaps"`
	Coverage      float64   `json:"coverage"`
	WindowsTotal  int       `json:"windows_total"`
	WindowsLocked int       `json:"windows_locked"`
	// DriftSuspected is set when a long segment's lag grows steadily: the two
	// sources run at different rates (e.g. a 24 fps master conformed to
	// 23.976). Constant-offset segments cannot express that, and no such pair
	// has been available to validate a resampling path against, so it is
	// reported instead of silently producing audio that slides out of sync.
	DriftSuspected bool `json:"drift_suspected,omitempty"`
}

type estimate struct {
	frame int     // window start, in dub frames
	lag   float64 // frames
	conf  float64
}

// matchWindow finds the lag (in frames) of a dub window starting at frame p0.
// center/search bound the search; search < 0 means the whole reference.
func matchWindow(ref, win *Features, p0 int, center float64, search int) (lag, conf float64, ok bool) {
	m, n := win.Frames, ref.Frames
	a, b := 0, n
	if search >= 0 {
		a = int(float64(p0) + center - float64(search))
		b = int(float64(p0)+center+float64(search)) + m
		if a < 0 {
			a = 0
		}
		if b > n {
			b = n
		}
	}
	if b-a < m+3 {
		return 0, 0, false
	}
	var scores []float64
	for band := range ref.Bands {
		c := crossCorrelate(ref.Bands[band][a:b], win.Bands[band])
		if scores == nil {
			scores = c
			continue
		}
		for i := range c {
			scores[i] += c[i]
		}
	}
	if len(scores) < 3 {
		return 0, 0, false
	}
	k := 0
	for i, v := range scores {
		if v > scores[k] {
			k = i
		}
	}
	med := median(scores)
	dev := make([]float64, len(scores))
	for i, v := range scores {
		dev[i] = math.Abs(v - med)
	}
	mad := median(dev) + 1e-9
	conf = (scores[k] - med) / (1.4826 * mad)
	return float64(a+k) + parabolic(scores, k) - float64(p0), conf, true
}

// scoreAt is the similarity of a dub window against the reference at a FIXED
// lag. It lets boundary refinement ask "which of these two offsets explains
// this second of audio better".
func scoreAt(ref, dub *Features, dubFrame, m int, lagFrames float64) (float64, bool) {
	k := int(math.Round(float64(dubFrame) + lagFrames))
	if dubFrame < 0 || dubFrame+m > dub.Frames || k < 0 || k+m > ref.Frames {
		return 0, false
	}
	var s float64
	for band := range ref.Bands {
		r, d := ref.Bands[band][k:k+m], dub.Bands[band][dubFrame:dubFrame+m]
		for i := range r {
			s += r[i] * d[i]
		}
	}
	return s / float64(m*len(ref.Bands)), true
}

func parabolic(y []float64, k int) float64 {
	if k <= 0 || k >= len(y)-1 {
		return 0
	}
	d := y[k-1] - 2*y[k] + y[k+1]
	if d == 0 {
		return 0
	}
	return math.Max(-1, math.Min(1, 0.5*(y[k-1]-y[k+1])/d))
}

func median(x []float64) float64 {
	if len(x) == 0 {
		return 0
	}
	s := append([]float64(nil), x...)
	sort.Float64s(s)
	if len(s)%2 == 1 {
		return s[len(s)/2]
	}
	return (s[len(s)/2-1] + s[len(s)/2]) / 2
}

// Align measures how the dub maps onto the base timeline.
//
// A single global offset is not enough. Two releases of the same episode differ
// in where the opening, eyecatch and preview sit, and TV rips reassembled at
// commercial breaks drift by a fraction of a second at each join. Across the
// validation set the duration difference ranged from -209 s to +179 s and
// changed sign between episodes; inside one episode the offset stepped 7 times.
// So the offset is measured per window and grouped into constant-offset
// segments — each cut shows up as a break between two segments.
func Align(base, dub *Features, baseDuration, dubDuration float64, opt Options) *Alignment {
	res := &Alignment{BaseDuration: baseDuration, DubDuration: dubDuration}
	if base.Frames < 100 || dub.Frames < 100 {
		return res
	}
	wm := int(math.Round(opt.WindowSec * fps))
	hm := int(math.Round(opt.HopSec * fps))
	search := int(math.Round(opt.SearchSec * fps))

	// Global prior from three 60 s probes searched over the whole episode. One
	// probe can land in a stretch that exists in only one source; three at
	// different points give a majority.
	pm := int(math.Round(60 * fps))
	prior, bestConf := 0.0, -1.0
	for _, frac := range []float64{0.25, 0.5, 0.75} {
		p0 := int(float64(dub.Frames)*frac) - pm/2
		if p0 < 0 || p0+pm > dub.Frames {
			continue
		}
		if lag, conf, ok := matchWindow(base, dub.slice(p0, p0+pm), p0, 0, -1); ok && conf > bestConf {
			prior, bestConf = lag, conf
		}
	}
	if bestConf < 0 {
		return res
	}

	var est []estimate
	for p0 := 0; p0+wm <= dub.Frames; p0 += hm {
		res.WindowsTotal++
		win := dub.slice(p0, p0+wm)
		lag, conf, ok := matchWindow(base, win, p0, prior, search)
		if !ok || conf < opt.MinConfidence {
			// A large cut throws the stretch far from the prior, where the
			// bounded search would never look. Retry unbounded.
			lag, conf, ok = matchWindow(base, win, p0, 0, -1)
		}
		if ok && conf >= opt.MinConfidence {
			est = append(est, estimate{p0, lag, conf})
		}
	}
	est = dropIsolated(est, opt.ToleranceSec*fps*4)
	res.WindowsLocked = len(est)
	if len(est) == 0 {
		return res
	}

	segs, drift := group(est, opt.ToleranceSec*fps, wm)
	res.DriftSuspected = drift
	sort.Slice(segs, func(i, j int) bool { return segs[i].BaseStart < segs[j].BaseStart })
	for i := 0; i+1 < len(segs); i++ {
		cut := refineBoundary(base, dub, segs[i], segs[i+1])
		segs[i].BaseEnd, segs[i+1].BaseStart = cut, cut
	}

	// Stretch the ends out to where the dub really has audio: the first window
	// starts at 0 s of the dub, but only becomes an estimate 30 s later.
	first, last := &segs[0], &segs[len(segs)-1]
	first.BaseStart = math.Min(first.BaseStart, math.Max(0, first.Lag))
	last.BaseEnd = math.Max(last.BaseEnd, math.Min(baseDuration, dubDuration+last.Lag))

	kept := segs[:0]
	for _, s := range segs {
		if s.BaseEnd-s.BaseStart > 0.5 {
			kept = append(kept, s)
		}
	}
	res.Segments = kept
	res.Gaps, res.Coverage = gapsAndCoverage(kept, baseDuration)
	return res
}

// dropIsolated removes windows whose lag disagrees with both neighbours. A
// stretch of nothing but a recurring music cue matches at several points of the
// episode, with high confidence and an absurd lag; the neighbourhood is what
// gives it away.
func dropIsolated(est []estimate, tol float64) []estimate {
	if len(est) < 3 {
		return est
	}
	var keep []estimate
	for i, w := range est {
		for _, j := range []int{i - 1, i + 1} {
			if j >= 0 && j < len(est) && math.Abs(w.lag-est[j].lag) <= tol {
				keep = append(keep, w)
				break
			}
		}
	}
	if len(keep) == 0 {
		return est
	}
	return keep
}

// group folds neighbouring windows with a similar lag into segments, and
// reports whether any long segment looks like rate drift.
func group(est []estimate, tol float64, wm int) ([]Segment, bool) {
	var groups [][]estimate
	var cur []estimate
	for _, w := range est {
		if len(cur) > 0 {
			lags := make([]float64, len(cur))
			for i, c := range cur {
				lags[i] = c.lag
			}
			if math.Abs(w.lag-median(lags)) > tol {
				groups = append(groups, cur)
				cur = nil
			}
		}
		cur = append(cur, w)
	}
	if len(cur) > 0 {
		groups = append(groups, cur)
	}

	// A lone window is noise, or an ambiguous musical stretch; let a neighbour
	// cover it. Unless lone windows are all there is.
	var solid [][]estimate
	longest := groups[0]
	for _, g := range groups {
		if len(g) >= 2 {
			solid = append(solid, g)
		}
		if len(g) > len(longest) {
			longest = g
		}
	}
	if len(solid) == 0 {
		solid = [][]estimate{longest}
	}

	drift := false
	segs := make([]Segment, 0, len(solid))
	for _, g := range solid {
		lags := make([]float64, len(g))
		confs := make([]float64, len(g))
		for i, w := range g {
			lags[i], confs[i] = w.lag, w.conf
		}
		lag := median(lags)
		if driftIn(g) {
			drift = true
		}
		start := float64(g[0].frame) / fps
		end := float64(g[len(g)-1].frame+wm) / fps
		segs = append(segs, Segment{
			BaseStart: start + lag/fps, BaseEnd: end + lag/fps,
			Lag: lag / fps, Confidence: median(confs), Windows: len(g),
		})
	}
	return segs, drift
}

// driftIn asks for EVIDENCE before calling a slope drift: a long stretch, many
// windows, enough accumulated error to hear, and a line that actually explains
// the points. Without that bar, quantisation noise reads as drift.
func driftIn(g []estimate) bool {
	span := float64(g[len(g)-1].frame-g[0].frame) / fps
	if len(g) < 10 || span < 180 {
		return false
	}
	var sx, sy, sxx, sxy float64
	n := float64(len(g))
	for _, w := range g {
		x, y := float64(w.frame), w.lag
		sx, sy, sxx, sxy = sx+x, sy+y, sxx+x*x, sxy+x*y
	}
	den := n*sxx - sx*sx
	if den == 0 {
		return false
	}
	slope := (n*sxy - sx*sy) / den
	icpt := (sy - slope*sx) / n
	var ssRes, ssTot float64
	mean := sy / n
	for _, w := range g {
		pred := icpt + slope*float64(w.frame)
		ssRes += (w.lag - pred) * (w.lag - pred)
		ssTot += (w.lag - mean) * (w.lag - mean)
	}
	return math.Abs(slope)*span > 0.08 && ssRes/(ssTot+1e-9) < 0.25
}

// refineBoundary places the join between two segments.
//
// Finding where b fits better than a is not enough — that returns the middle of
// b's stretch, not the join, and left 17 s of one validation episode playing at
// the wrong offset. What is wanted is the cut c that maximises "explain
// everything before c with a, everything after with b": a change point, solved
// with running sums.
func refineBoundary(base, dub *Features, a, b Segment) float64 {
	fallback := (a.BaseEnd + b.BaseStart) / 2
	lo := math.Max(0, math.Min(a.BaseEnd, b.BaseStart)-45)
	hi := math.Max(a.BaseEnd, b.BaseStart) + 45
	const winSec, step = 3.0, 0.5
	m := int(math.Round(winSec * fps))

	var ts, sa, sb []float64
	for t := lo; t+winSec <= hi; t += step {
		va, okA := scoreAt(base, dub, int(math.Round((t-a.Lag)*fps)), m, a.Lag*fps)
		vb, okB := scoreAt(base, dub, int(math.Round((t-b.Lag)*fps)), m, b.Lag*fps)
		if okA && okB {
			ts, sa, sb = append(ts, t), append(sa, va), append(sb, vb)
		}
	}
	if len(ts) < 3 {
		return fallback
	}
	var totalB float64
	for _, v := range sb {
		totalB += v
	}
	best, bestObj := 0, math.Inf(-1)
	var cumA, cumB float64
	for i := 0; i <= len(ts); i++ {
		if obj := cumA + (totalB - cumB); obj > bestObj {
			best, bestObj = i, obj
		}
		if i < len(ts) {
			cumA, cumB = cumA+sa[i], cumB+sb[i]
		}
	}
	if best >= len(ts) {
		return ts[len(ts)-1] + step
	}
	return ts[best]
}

// gapsAndCoverage measures the UNION of the segments. Summing durations double
// counts overlaps, and a "101% coverage" would sail straight past the quality
// gate.
func gapsAndCoverage(segs []Segment, baseDuration float64) ([]Gap, float64) {
	var gaps []Gap
	var covered, cursor float64
	for _, s := range segs {
		if s.BaseStart-cursor > 0.25 {
			gaps = append(gaps, Gap{cursor, s.BaseStart})
		}
		if s.BaseEnd > cursor {
			covered += s.BaseEnd - math.Max(cursor, s.BaseStart)
			cursor = s.BaseEnd
		}
	}
	if baseDuration-cursor > 0.25 {
		gaps = append(gaps, Gap{cursor, baseDuration})
	}
	if baseDuration <= 0 {
		return gaps, 0
	}
	return gaps, covered / baseDuration
}
