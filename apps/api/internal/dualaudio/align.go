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
// season, 66 of 82 episodes passing both the gate and the finished-file check.
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

	cand map[int]*candidate // candidate lags, keyed in frames (10 ms)
	wm   int                // analysis window, in frames
}

// candidate is a lag the solver may use, and the stretch of the base timeline
// around which it was observed.
type candidate struct {
	lo, hi float64 // base position, frames
	votes  int
}

func (a *Alignment) addCandidate(lagFrames int, basePos float64) {
	c, ok := a.cand[lagFrames]
	if !ok {
		a.cand[lagFrames] = &candidate{basePos, basePos, 1}
		return
	}
	c.lo, c.hi, c.votes = math.Min(c.lo, basePos), math.Max(c.hi, basePos), c.votes+1
}

// Seam repair tuning. A run must OPEN (or close) the segment it sits in — a run
// starting well inside one is a different fault, not a misplaced seam.
const (
	seamSnapSlackSec = validateWinSec
	seamMinSegSec    = 1.0
)

// SnapSeams moves a seam the solver put in the wrong place, using validation
// evidence the refinement itself cannot reach.
//
// refineBoundary searches +-15 s around where the Viterbi path switched. When
// the switch is further off than that the true cut is outside the search and no
// amount of refining finds it: one Pokemon episode had the seam 30 s early and
// wrote 35 s at 0.33 s off, graded "review" every round without ever improving.
//
// The validation knows where it is. A run of confident windows that opens a
// segment, all disagreeing by the same amount, whose IMPLIED lag (the segment's
// lag plus the disagreement) is a NEIGHBOUR's lag, is that neighbour's stretch
// sitting on the wrong side of the seam. Moving the seam past the run and
// re-refining from there puts the cut inside reach.
//
// Only seams move. No lag is invented, and a seam that would leave either side
// shorter than a second is refused. Reports whether anything moved.
func (a *Alignment) SnapSeams(base, dub *Features, runs []OffRun) bool {
	moved := false
	for _, r := range runs {
		if i, t, ok := seamMove(a.Segments, r); ok {
			a.moveSeam(base, dub, i, t)
			moved = true
		}
	}
	if moved {
		a.Gaps, a.Coverage = gapsAndCoverage(a.Segments, a.BaseDuration)
	}
	return moved
}

// seamMove reads a run as a seam in the wrong place: it returns the index of
// the segment that should END at t. ok is false when the run is some other
// fault — the decision, kept apart from the audio so it can be reasoned about
// on the map alone.
func seamMove(segs []Segment, r OffRun) (int, float64, bool) {
	i := -1
	for k, s := range segs {
		if s.BaseStart <= r.Start && r.Start < s.BaseEnd {
			i = k
			break
		}
	}
	if i < 0 {
		return 0, 0, false
	}
	implied := segs[i].Lag + r.Lag
	var j int
	var t float64
	switch {
	// The run OPENS this segment and its implied lag is the previous one's:
	// the stretch belongs before the seam, so the seam moves past the run.
	case i > 0 && math.Abs(implied-segs[i-1].Lag) <= mergeLagSec &&
		r.Start-segs[i].BaseStart <= seamSnapSlackSec && r.End < segs[i].BaseEnd:
		j, t = i-1, r.End
	// The run CLOSES this segment and belongs to the next one.
	case i+1 < len(segs) && math.Abs(implied-segs[i+1].Lag) <= mergeLagSec &&
		segs[i].BaseEnd-r.End <= seamSnapSlackSec && r.Start > segs[i].BaseStart:
		j, t = i, r.Start
	default:
		return 0, 0, false
	}
	if t <= segs[j].BaseStart+seamMinSegSec || t >= segs[j+1].BaseEnd-seamMinSegSec {
		return 0, 0, false
	}
	return j, t, true
}

// moveSeam puts the join between segments i and i+1 at t. When the lag rises
// the base holds content the dub lacks, so refineBoundary places the hole.
//
// The refinement is BOUNDED by t, and that bound is the whole point: its window
// reaches 15 s past the join it is given, which is far enough to walk straight
// back onto the stretch the validation just proved belongs to the other side —
// measured doing exactly that, returning the seam to 1194 s after evidence put
// it at 1230. Where the two disagree the validation wins: it read the rebuilt
// track, the change point only scores the sources.
func (a *Alignment) moveSeam(base, dub *Features, i int, t float64) {
	x, y := &a.Segments[i], &a.Segments[i+1]
	later := t > x.BaseEnd
	x.BaseEnd, y.BaseStart = t, t
	if y.Lag-x.Lag <= 0.05 {
		return
	}
	endA, startB := refineBoundary(base, dub, *x, *y, a.wm)
	if (later && endA < t) || (!later && endA > t) {
		return // walked back over the evidence; keep the seam where it is
	}
	x.BaseEnd, y.BaseStart = endA, startB
}

// AddCandidates feeds validation findings back into the solver. A confident
// residual of +0.14 s over a stretch says the right lag there is the segment's
// lag + 0.14. A segment too short for the coarse sweep (30 s windows every 10 s)
// never becomes a candidate on its own; this is how it does. It reports whether
// anything was added.
func (a *Alignment) AddCandidates(offs []Offset) bool {
	added := false
	for _, o := range offs {
		mid := o.BaseTime + validateWinSec/2
		for _, s := range a.Segments {
			if s.BaseStart <= mid && mid < s.BaseEnd {
				a.addCandidate(int(math.Round((s.Lag+o.Lag)*fps)), o.BaseTime*fps)
				added = true
				break
			}
		}
	}
	return added
}

// Offset is a confident validation window that disagrees with zero.
type Offset struct {
	BaseTime float64
	Lag      float64
}

// Solver tuning. The switch toll is worth a few seconds of good score: enough
// that noise never pays for a switch, little enough that a real step does within
// 1-3 s.
const (
	solveStepSec    = 0.5
	solveWinSec     = 3.0
	solveSwitch     = 0.6
	solveOutOfSpan  = -0.2 // score of a candidate far from where it was observed
	candSlackSec    = 60.0
	mergeLagSec     = 0.0205
	excursionMaxSec = 20.0
)

const (
	// edgeSlackSec bounds how far the first and last segment may be extended
	// past their measured windows.
	edgeSlackSec = 45.0
	// heavyWindows is the size from which a group counts as solid evidence.
	heavyWindows = 5
	// farLagSec is how far a light group may sit from every heavy one.
	farLagSec = 20.0
)

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
	if len(est) == 0 {
		return res
	}

	chain, drift := chainGroups(est, opt.ToleranceSec*fps, wm, base.Frames)
	res.DriftSuspected = drift
	res.cand = map[int]*candidate{}
	for _, g := range chain {
		for _, w := range g {
			res.WindowsLocked++
			res.addCandidate(int(math.Round(w.lag)), float64(w.frame)+w.lag)
		}
	}
	res.wm = wm
	res.Solve(base, dub)
	// Two passes: recovering a short segment creates new joins of its own.
	for pass := 0; pass < 2 && res.proposeNearJoins(base, dub, opt.MinConfidence); pass++ {
		res.Solve(base, dub)
	}
	return res
}

// proposeNearJoins sweeps short windows around every join and adds the lags it
// finds as candidates. It reports whether it added a lag the solver did not have.
//
// The coarse sweep (30 s windows every 10 s) cannot resolve a segment shorter
// than ~40 s: it never produces two agreeing windows, so its lag is never
// proposed and the stretch is split between its neighbours, off by the step
// size. Validation cannot be relied on to notice, because a window that spans a
// join with a hole in it is not eligible. Short segments live between two
// longer ones, so that is where to look.
func (a *Alignment) proposeNearJoins(base, dub *Features, minConf float64) bool {
	const winSec, hopSec, reachSec = 8.0, 2.0, 25.0
	m := int(winSec * fps)
	added := false
	for i := 0; i+1 < len(a.Segments); i++ {
		sa, sb := a.Segments[i], a.Segments[i+1]
		center := (sa.Lag + sb.Lag) / 2 * fps
		search := int((math.Abs(sb.Lag-sa.Lag)/2 + 2) * fps)
		type found struct {
			k   int
			pos float64
		}
		var hits []found
		for t := sa.BaseEnd - reachSec; t <= sb.BaseStart+reachSec; t += hopSec {
			p0 := int(math.Round(t*fps - center))
			if p0 < 0 || p0+m > dub.Frames {
				continue
			}
			if lag, conf, ok := matchWindow(base, dub.slice(p0, p0+m), p0, center, search); ok && conf >= minConf {
				hits = append(hits, found{int(math.Round(lag)), float64(p0) + lag})
			}
		}
		// Only lags that two windows agree on (within a frame). Where the base
		// holds content the dub lacks, short windows match noise at random lags;
		// a real segment, however short, repeats its lag.
		for _, h := range hits {
			votes := 0
			for _, o := range hits {
				if o.k >= h.k-1 && o.k <= h.k+1 {
					votes++
				}
			}
			if votes < 2 {
				continue
			}
			if _, known := a.cand[h.k]; !known {
				added = true
			}
			a.addCandidate(h.k, h.pos)
		}
	}
	return added
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

// chainGroups folds neighbouring windows with a similar lag into groups and
// keeps the heaviest MONOTONIC chain of them. It also reports whether any long
// group looks like rate drift. The groups only PROPOSE lags; Solve decides.
//
// A real alignment moves forward on both timelines. Recurring music (the
// opening theme, a sting) matches elsewhere in the episode with high
// confidence, and because neighbouring windows overlap by 20 s the mistake
// arrives as a group of 2-4 "consistent" windows that the lone-window filter
// cannot catch. On the validation season 32 of 74 episodes had at least one
// (lags of +506 s, -668 s, +1320 s); one of them, being last in order, was
// stretched to the end of the video — half an episode playing the dub from the
// wrong place, graded "ok". The maximum-weight chain in which the base position
// never steps back by more than a window discards all of them by construction.
func chainGroups(est []estimate, tol float64, wm, baseFrames int) ([][]estimate, bool) {
	var groups [][]estimate
	var cur []estimate
	for _, w := range est {
		if len(cur) > 0 && math.Abs(w.lag-medianLag(cur)) > tol {
			groups = append(groups, cur)
			cur = nil
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

	type node struct {
		g             []estimate
		lag           float64
		bFirst, bLast float64 // base position of the first / last window start
	}
	var nodes []node
	for _, g := range solid {
		lag := medianLag(g)
		n := node{g, lag, float64(g[0].frame) + lag, float64(g[len(g)-1].frame) + lag}
		if n.bLast+float64(wm) <= 0 || n.bFirst >= float64(baseFrames) {
			continue // falls outside the base video altogether
		}
		nodes = append(nodes, n)
	}
	if len(nodes) == 0 {
		return nil, false
	}
	best := make([]int, len(nodes))
	prev := make([]int, len(nodes))
	top := 0
	for j := range nodes {
		best[j], prev[j] = len(nodes[j].g), -1
		for i := 0; i < j; i++ {
			if nodes[j].bFirst >= nodes[i].bLast-float64(wm) && best[i]+len(nodes[j].g) > best[j] {
				best[j], prev[j] = best[i]+len(nodes[j].g), i
			}
		}
		if best[j] > best[top] {
			top = j
		}
	}
	var chain []node
	for j := top; j >= 0; j = prev[j] {
		chain = append([]node{nodes[j]}, chain...)
	}

	// A group that is both light and far from every heavy one is more likely a
	// mismatch than a cut, even when monotonic. Erring this way is cheap: the
	// stretch becomes a gap and plays the base audio instead of a guessed dub.
	var heavy []node
	for _, n := range chain {
		if len(n.g) >= heavyWindows {
			heavy = append(heavy, n)
		}
	}
	if len(heavy) > 0 {
		kept := chain[:0:0]
		for _, n := range chain {
			near := len(n.g) >= heavyWindows
			for _, h := range heavy {
				if math.Abs(n.lag-h.lag) <= farLagSec*fps {
					near = true
				}
			}
			if near {
				kept = append(kept, n)
			}
		}
		chain = kept
	}

	drift := false
	out := make([][]estimate, 0, len(chain))
	for _, n := range chain {
		if driftIn(n.g) {
			drift = true
		}
		out = append(out, n.g)
	}
	return out, drift
}

func medianLag(g []estimate) float64 {
	lags := make([]float64, len(g))
	for i, w := range g {
		lags[i] = w.lag
	}
	return median(lags)
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

// refineBoundary returns where segment a ends and where b starts, on the base
// timeline.
//
// Finding where b fits better than a is not enough — that returns the middle of
// b's stretch, not the join, and left 17 s of one validation episode playing at
// the wrong offset. What is wanted is the change point that maximises "explain
// everything before it with a, everything after with b", solved with running
// sums.
//
// When the lag RISES (delta > 0) the base holds delta seconds the dub does not
// have. The dub is continuous, so b can only resume at cut+delta, and the
// stretch in between is a gap that plays the base audio. Without this it was
// filled with repeated dub audio — 8.8 s of it in one validation episode.
func refineBoundary(base, dub *Features, a, b Segment, wm int) (endA, startB float64) {
	hole := 0.0
	if d := b.Lag - a.Lag; d > 0.05 {
		hole = d
	}
	winLen := float64(wm) / fps
	lo := math.Max(0, math.Min(a.BaseEnd-winLen, b.BaseStart)-15)
	hi := math.Max(a.BaseEnd, b.BaseStart+winLen) + 15 + hole
	const winSec, step = 3.0, 0.5
	m := int(math.Round(winSec * fps))

	var ts, sa, sb []float64
	for t := lo; t+winSec <= hi; t += step {
		va, _ := scoreAt(base, dub, int(math.Round((t-a.Lag)*fps)), m, a.Lag*fps)
		vb, _ := scoreAt(base, dub, int(math.Round((t-b.Lag)*fps)), m, b.Lag*fps)
		ts, sa, sb = append(ts, t), append(sa, va), append(sb, vb)
	}
	if len(ts) < 3 {
		mid := (a.BaseEnd + b.BaseStart) / 2
		return mid, mid + hole
	}
	cumA := make([]float64, len(ts)+1)
	cumB := make([]float64, len(ts)+1)
	for i := range ts {
		cumA[i+1], cumB[i+1] = cumA[i]+sa[i], cumB[i]+sb[i]
	}
	skip := int(math.Round(hole / step))
	best, bestObj := 0, math.Inf(-1)
	for i := 0; i <= len(ts); i++ {
		j := i + skip
		if j > len(ts) {
			j = len(ts)
		}
		if obj := cumA[i] + (cumB[len(ts)] - cumB[j]); obj > bestObj {
			best, bestObj = i, obj
		}
	}
	cut := ts[len(ts)-1] + step
	if best < len(ts) {
		cut = ts[best]
	}
	cut = math.Max(cut, a.BaseStart)
	cut = math.Min(cut, math.Max(a.BaseStart, b.BaseEnd-hole))
	return cut, cut + hole
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

// Solve picks the best path over the candidate lags (Viterbi) and fills in
// Segments, Gaps and Coverage.
//
// Deciding the map from window groups fails on precision. Spectral flux is sharp
// enough that a 30 ms lag error drops the score from 0.13 to ~0, so a tolerant
// group (80 ms, median) blends -10.04 with -10.01 into a lag that fits neither,
// the boundary contest turns into noise, and the join lands anywhere. Checking
// finished files found 19-72 s playing at a neighbour's offset in 14 of 75
// episodes that had been graded "ok".
//
// Here every candidate lag (10 ms resolution) is scored every 0.5 s of video and
// the chosen sequence is the one that best explains the WHOLE episode, paying a
// toll per switch. A stretch with no evidence (dialogue only) pays for no switch
// and inherits the lag in force, which is the right default.
func (a *Alignment) Solve(base, dub *Features) {
	a.Segments, a.Gaps, a.Coverage = nil, nil, 0
	if len(a.cand) == 0 {
		return
	}
	keys := make([]int, 0, len(a.cand))
	lo, hi := math.Inf(1), math.Inf(-1)
	for k, c := range a.cand {
		keys = append(keys, k)
		lo, hi = math.Min(lo, c.lo), math.Max(hi, c.hi)
	}
	sort.Ints(keys)
	m := int(math.Round(solveWinSec * fps))
	tLo := math.Max(0, lo/fps-edgeSlackSec)
	tHi := math.Min(a.BaseDuration, hi/fps+float64(a.wm)/fps+edgeSlackSec)
	var ts []float64
	for t := tLo; t < tHi-solveWinSec; t += solveStepSec {
		ts = append(ts, t)
	}
	if len(ts) == 0 {
		return
	}

	// score[i][j]: similarity at time ts[i] under candidate keys[j], from a
	// running sum of the per-frame band products.
	score := make([][]float64, len(ts))
	for i := range score {
		score[i] = make([]float64, len(keys))
		for j := range score[i] {
			score[i][j] = solveOutOfSpan
		}
	}
	norm := float64(m * len(base.Bands))
	for j, k := range keys {
		f0, f1 := k, dub.Frames+k
		if f0 < 0 {
			f0 = 0
		}
		if f1 > base.Frames {
			f1 = base.Frames
		}
		if f1-f0 <= m {
			continue
		}
		cum := make([]float64, f1-f0+1)
		for f := f0; f < f1; f++ {
			var e float64
			for b := range base.Bands {
				e += base.Bands[b][f] * dub.Bands[b][f-k]
			}
			cum[f-f0+1] = cum[f-f0] + e
		}
		c := a.cand[k]
		spanLo, spanHi := c.lo/fps-candSlackSec, c.hi/fps+float64(a.wm)/fps+candSlackSec
		for i, t := range ts {
			fr := int(math.Round(t * fps))
			if fr >= f0 && fr+m <= f1 && t >= spanLo && t <= spanHi {
				score[i][j] = (cum[fr-f0+m] - cum[fr-f0]) / norm
			}
		}
	}

	dp := append([]float64(nil), score[0]...)
	back := make([][]int, len(ts))
	for i := 1; i < len(ts); i++ {
		best := 0
		for j := range dp {
			if dp[j] > dp[best] {
				best = j
			}
		}
		back[i] = make([]int, len(keys))
		next := make([]float64, len(keys))
		for j := range dp {
			if move := dp[best] - solveSwitch; move > dp[j] {
				back[i][j], next[j] = best, move+score[i][j]
			} else {
				back[i][j], next[j] = j, dp[j]+score[i][j]
			}
		}
		dp = next
	}
	path := make([]int, len(ts))
	for j := range dp {
		if dp[j] > dp[path[len(ts)-1]] {
			path[len(ts)-1] = j
		}
	}
	for i := len(ts) - 1; i > 0; i-- {
		path[i-1] = back[i][path[i]]
	}

	var segs []Segment
	for i0, i := 0, 1; i <= len(ts); i++ {
		if i < len(ts) && path[i] == path[i0] {
			continue
		}
		j := path[i0]
		end := tHi
		if i < len(ts) {
			end = ts[i]
		}
		var mean float64
		for r := i0; r < i; r++ {
			mean += score[r][j]
		}
		segs = append(segs, Segment{BaseStart: ts[i0], BaseEnd: end, Lag: float64(keys[j]) / fps,
			Confidence: mean / float64(i-i0) * 100, Windows: a.cand[keys[j]].votes})
		i0 = i
	}

	// Drop short excursions: a path that leaves lag A for X and comes back to
	// (within 20 ms of) A. Two real cuts would have to cancel exactly for that to
	// be true; what it is, is a few seconds where noise out-scored a weak stretch.
	for changed := true; changed; {
		changed = false
		for i := 1; i+1 < len(segs); i++ {
			if math.Abs(segs[i-1].Lag-segs[i+1].Lag) <= mergeLagSec &&
				segs[i].BaseEnd-segs[i].BaseStart < excursionMaxSec {
				segs[i-1].BaseEnd = segs[i+1].BaseEnd
				segs = append(segs[:i], segs[i+2:]...)
				changed = true
				break
			}
		}
	}

	// Fold neighbours within 20 ms of each other, keeping the longer one's lag.
	// The difference matters for MEASURING — the score is that sharp — but it is
	// inaudible, and every switch would be a needless crossfaded join.
	merged := segs[:0:0]
	for _, s := range segs {
		if n := len(merged); n > 0 && math.Abs(s.Lag-merged[n-1].Lag) <= mergeLagSec {
			y := &merged[n-1]
			if s.BaseEnd-s.BaseStart > y.BaseEnd-y.BaseStart {
				y.Lag, y.Confidence, y.Windows = s.Lag, s.Confidence, s.Windows
			}
			y.BaseEnd = s.BaseEnd
			continue
		}
		merged = append(merged, s)
	}
	segs = merged

	// The dub has no audio before its own start or after its own end.
	for i := range segs {
		segs[i].BaseStart = math.Max(segs[i].BaseStart, math.Max(segs[i].Lag, 0))
		segs[i].BaseEnd = math.Min(segs[i].BaseEnd, math.Min(a.DubDuration+segs[i].Lag, a.BaseDuration))
	}
	// Where the lag rises the base holds content the dub lacks: leave a hole.
	for i := 0; i+1 < len(segs); i++ {
		if segs[i+1].Lag-segs[i].Lag > 0.05 {
			segs[i].BaseEnd, segs[i+1].BaseStart = refineBoundary(base, dub, segs[i], segs[i+1], a.wm)
		}
	}
	for _, s := range segs {
		if s.BaseEnd-s.BaseStart > 0.5 {
			a.Segments = append(a.Segments, s)
		}
	}
	a.Gaps, a.Coverage = gapsAndCoverage(a.Segments, a.BaseDuration)
}
