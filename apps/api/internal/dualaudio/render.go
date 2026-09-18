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

const crossfadeSec = 0.02

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

// Residual re-correlates the REBUILT track against the base and returns the
// median and 95th-percentile absolute lag, in seconds.
//
// Only windows fully inside a mapped segment count. A gap filled with the
// base's own audio would match itself perfectly and flatter the number.
func Residual(rendered []int16, base *Features, segs []Segment, minConf float64) (med, p95 float64, n int) {
	feat := ExtractFeatures(downmix16k(rendered))
	if feat.Frames < 100 {
		return 0, 0, 0
	}
	wm, hm := int(20*fps), int(30*fps)
	var abs []float64
	for p0 := 0; p0+wm <= feat.Frames; p0 += hm {
		t0, t1 := float64(p0)/fps, float64(p0+wm)/fps
		inside := false
		for _, s := range segs {
			if s.BaseStart <= t0 && t1 <= s.BaseEnd {
				inside = true
				break
			}
		}
		if !inside {
			continue
		}
		if lag, conf, ok := matchWindow(base, feat.slice(p0, p0+wm), p0, 0, int(5*fps)); ok && conf >= minConf {
			abs = append(abs, math.Abs(lag)/fps)
		}
	}
	if len(abs) == 0 {
		return 0, 0, 0
	}
	return median(abs), percentile(abs, 0.95), len(abs)
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
