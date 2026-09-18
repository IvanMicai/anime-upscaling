package dualaudio

import (
	"math"
	"sort"
)

// GapFill selects what plays where the dub has no counterpart.
type GapFill string

const (
	// GapFillBase plays the base track. A stretch that exists only in the base
	// (credits, preview, a scene the dub's rip cut) would otherwise be a minute
	// of silence, which reads as a playback fault; the original language says at
	// once that this part was never dubbed.
	GapFillBase    GapFill = "base"
	GapFillSilence GapFill = "silence"
)

const (
	crossfadeSec    = 0.02
	hitToleranceSec = 0.06
)

// Miss is a validation window whose peak did not land on zero: where to look
// when an episode is held back.
type Miss struct {
	BaseTime   float64 `json:"base_time"`
	Lag        float64 `json:"lag"`
	Confidence float64 `json:"confidence"`
}

// Render rebuilds the dub on the base timeline. dub and base are interleaved
// stereo at outRate; base may be nil when fill is GapFillSilence.
func Render(a *Alignment, dub, base []int16, fill GapFill) []int16 {
	total := int(math.Round(a.BaseDuration * outRate))
	mix := make([]float32, total*outChannels)
	fade := int(math.Round(crossfadeSec * outRate))
	if fade < 1 {
		fade = 1
	}
	// sin² in, cos² out: the two ramps sum to exactly 1 across a join.
	ramp := make([]float32, fade)
	for i := range ramp {
		s := math.Sin(float64(i) / float64(fade) * math.Pi / 2)
		ramp[i] = float32(s * s)
	}

	place := func(src []int16, srcStart, dstStart, dstEnd int) {
		lo, hi := dstStart-fade, dstEnd+fade
		if lo < 0 {
			lo = 0
		}
		if hi > total {
			hi = total
		}
		n := hi - lo
		if n <= 0 {
			return
		}
		srcStart -= dstStart - lo
		for i := 0; i < n; i++ {
			g := float32(1)
			if i < fade {
				g = ramp[i]
			} else if n-1-i < fade {
				g = ramp[n-1-i]
			}
			si := srcStart + i
			if si < 0 || (si+1)*outChannels > len(src) {
				continue // outside the source: leave silence
			}
			for c := 0; c < outChannels; c++ {
				mix[(lo+i)*outChannels+c] += g * float32(src[si*outChannels+c])
			}
		}
	}

	for _, s := range a.Segments {
		e0 := int(math.Round(s.BaseStart * outRate))
		e1 := int(math.Round(s.BaseEnd * outRate))
		place(dub, int(math.Round((s.BaseStart-s.Lag)*outRate)), e0, e1)
	}
	if fill == GapFillBase && base != nil {
		for _, g := range a.Gaps {
			e0 := int(math.Round(g.Start * outRate))
			place(base, e0, e0, int(math.Round(g.End*outRate)))
		}
	}

	out := make([]int16, len(mix))
	for i, v := range mix {
		switch {
		case v > 32767:
			out[i] = 32767
		case v < -32768:
			out[i] = -32768
		default:
			out[i] = int16(v)
		}
	}
	return out
}

// downmix16k folds interleaved stereo at outRate to mono at featRate by
// averaging each group of outRate/featRate frames — a box filter, which is all
// the feature extractor needs.
func downmix16k(pcm []int16) []int16 {
	const ratio = outRate / featRate
	frames := len(pcm) / outChannels / ratio
	out := make([]int16, frames)
	for i := range out {
		var acc int
		for j := 0; j < ratio; j++ {
			for c := 0; c < outChannels; c++ {
				acc += int(pcm[((i*ratio)+j)*outChannels+c])
			}
		}
		out[i] = int16(acc / (ratio * outChannels))
	}
	return out
}

// Validation is what re-correlating the REBUILT track against the base found.
type Validation struct {
	Median, P95 float64 // absolute residual of confident windows, seconds
	Hits        int     // windows whose peak landed on zero
	Eligible    int     // ... out of this many
	Misses      []Miss
	// OffSec is seconds of CONSISTENT error: neighbouring, confident windows
	// that disagree with zero and agree with EACH OTHER — the signature of a
	// stretch playing at a neighbour's offset. The hit share cannot see it (40 s
	// wrong is 3% of an episode); checking finished files did, in 14 of 75
	// episodes graded "ok".
	OffSec float64
	Offs   []Offset // feed these to Alignment.AddCandidates
}

const (
	validateWinSec = 10.0
	validateHopSec = 5.0
)

// Validate re-correlates the rebuilt track against the base.
//
// Only windows fully inside a mapped segment count. A gap filled with the
// base's own audio would match itself perfectly and flatter the numbers.
//
// A hit is a window whose correlation peak, searched over +-5 s, lands within
// 60 ms of zero — with NO confidence floor. Requiring confidence failed good
// episodes: a stretch of pure dialogue shares almost no score, so the peak is
// weak, but it is still IN PLACE. With the wrong audio the peak lands anywhere
// in the 10 s searched (about a 1% chance of hitting zero by luck). Without the
// hit share, a stretch carrying the wrong audio is invisible: no window locks
// there, so it never enters the median, which then looks perfect.
func Validate(rendered []int16, base *Features, segs []Segment, minConf float64) Validation {
	var v Validation
	feat := ExtractFeatures(downmix16k(rendered))
	if feat.Frames < 100 {
		return v
	}
	wm, hm := int(validateWinSec*fps), int(validateHopSec*fps)
	var abs []float64
	var confident []Offset
	// A window counts when dub audio covers ALL of it — and it MAY span a join
	// between two segments. Requiring it to sit inside a single segment left the
	// +-10 s around every join unvalidated, which is exactly where a misplaced
	// boundary shows.
	var covered []Gap
	for _, s := range segs {
		if n := len(covered); n > 0 && s.BaseStart-covered[n-1].End <= 0.05 {
			covered[n-1].End = math.Max(covered[n-1].End, s.BaseEnd)
			continue
		}
		covered = append(covered, Gap{s.BaseStart, s.BaseEnd})
	}
	for p0 := 0; p0+wm <= feat.Frames; p0 += hm {
		t0, t1 := float64(p0)/fps, float64(p0+wm)/fps
		inside := false
		for _, c := range covered {
			if c.Start <= t0 && t1 <= c.End {
				inside = true
				break
			}
		}
		if !inside {
			continue
		}
		v.Eligible++
		lag, conf, ok := matchWindow(base, feat.slice(p0, p0+wm), p0, 0, int(5*fps))
		if ok && math.Abs(lag) <= hitToleranceSec*fps {
			v.Hits++
		} else {
			v.Misses = append(v.Misses, Miss{BaseTime: t0, Lag: lag / fps, Confidence: conf})
		}
		if ok && conf >= minConf {
			abs = append(abs, math.Abs(lag)/fps)
			confident = append(confident, Offset{BaseTime: t0, Lag: lag / fps})
		}
	}

	var run []Offset
	// Three in a row, not two: windows overlap by half, so two neighbours can
	// share one spurious peak and "agree" without being independent evidence.
	// Three guarantees a disjoint pair.
	flush := func() {
		if len(run) >= 3 {
			v.OffSec += run[len(run)-1].BaseTime - run[0].BaseTime + validateWinSec
		}
	}
	for _, o := range confident {
		off := math.Abs(o.Lag) > hitToleranceSec
		if off {
			v.Offs = append(v.Offs, o)
		}
		if off && (len(run) == 0 || (math.Abs(o.Lag-run[len(run)-1].Lag) <= 0.08 &&
			o.BaseTime-run[len(run)-1].BaseTime <= validateHopSec*1.5)) {
			run = append(run, o)
			continue
		}
		flush()
		run = run[:0]
		if off {
			run = append(run, o)
		}
	}
	flush()
	if len(abs) > 0 {
		v.Median, v.P95 = median(abs), percentile(abs, 0.95)
	}
	return v
}

func percentile(x []float64, q float64) float64 {
	s := append([]float64(nil), x...)
	sortFloats(s)
	pos := q * float64(len(s)-1)
	i := int(math.Floor(pos))
	if i+1 >= len(s) {
		return s[len(s)-1]
	}
	return s[i] + (pos-float64(i))*(s[i+1]-s[i])
}

func sortFloats(s []float64) { sort.Float64s(s) }
