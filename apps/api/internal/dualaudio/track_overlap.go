package dualaudio

import (
	"context"
	"fmt"
	"math"
)

// TrackOverlapWindow identifies similar audio, not a language or speech verdict.
type TrackOverlapWindow struct {
	Start       float64    `json:"start"`
	End         float64    `json:"end"`
	LagSamples  int        `json:"lag_samples"`
	Correlation [2]float64 `json:"correlation"`
	Gain        [2]float64 `json:"gain"`
}

// ScreenTrackOverlap flags half-second stereo windows with strong positive
// waveform similarity at near-zero offsets. Shared music can trigger a match;
// absence of a match does not prove that a window has dubbed speech. Inputs
// must be timestamp-aligned 8 kHz stereo PCM decoded from the same container.
func ScreenTrackOverlap(ctx context.Context, reference, candidate []int16) ([]TrackOverlapWindow, error) {
	const rate, window, radius = 8000, 4000, 320
	if len(reference)%2 != 0 || len(candidate)%2 != 0 || len(reference) == 0 || len(candidate) == 0 || len(reference)/2 > rate*7200 || len(candidate)/2 > rate*7200 {
		return nil, fmt.Errorf("expected nonempty stereo PCM of at most two hours")
	}
	n := min(len(reference), len(candidate)) / 2
	// Box-filter before the coarse lag search so broadband sounds still
	// correlate when the true offset falls between grid points.
	lowpass := func(pcm []int16) ([]int16, error) {
		out := make([]int16, len(pcm))
		var sum [2]int
		for i := 0; i < len(pcm)/2; i++ {
			if i%80000 == 0 {
				if err := ctx.Err(); err != nil {
					return nil, err
				}
			}
			for ch := 0; ch < 2; ch++ {
				sum[ch] += int(pcm[2*i+ch])
				if i >= 8 {
					sum[ch] -= int(pcm[2*(i-8)+ch])
				}
				if i >= 7 {
					out[2*i+ch] = int16(sum[ch] / 8)
				}
			}
		}
		return out, nil
	}
	coarseRef, err := lowpass(reference)
	if err != nil {
		return nil, err
	}
	coarseCandidate, err := lowpass(candidate)
	if err != nil {
		return nil, err
	}
	out := []TrackOverlapWindow{}
	for start := radius; start+window+radius <= n; start += window {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		// Coarse search samples one out of four frames; refine the best lag at
		// full resolution. The 1 ms lag grid is subsequently refined to 0.125 ms.
		best, bestLag := -2.0, 0
		for lag := -radius; lag <= radius; lag += 8 {
			corr, _, ok := trackSimilarity(coarseRef, coarseCandidate, start, window, lag, 8)
			if ok && min(corr[0], corr[1]) > best {
				best, bestLag = min(corr[0], corr[1]), lag
			}
		}
		if best < .4 {
			continue
		}
		var chosen TrackOverlapWindow
		best = -2
		for lag := max(-radius, bestLag-7); lag <= min(radius, bestLag+7); lag++ {
			corr, gain, ok := trackSimilarity(reference, candidate, start, window, lag, 1)
			if ok && min(corr[0], corr[1]) > best {
				best = min(corr[0], corr[1])
				chosen = TrackOverlapWindow{Start: float64(start) / rate, End: float64(start+window) / rate, LagSamples: lag, Correlation: corr, Gain: gain}
			}
		}
		if best >= .9 && chosen.Gain[0] >= .8 && chosen.Gain[0] <= 1.2 && chosen.Gain[1] >= .8 && chosen.Gain[1] <= 1.2 {
			out = append(out, chosen)
		}
	}
	return out, nil
}

func trackSimilarity(a, b []int16, start, count, lag, step int) (corr, gain [2]float64, ok bool) {
	for ch := 0; ch < 2; ch++ {
		var sx, sy, sxx, syy, sxy float64
		n := 0.0
		for i := 0; i < count; i += step {
			x, y := float64(a[2*(start+i)+ch]), float64(b[2*(start+i+lag)+ch])
			sx += x
			sy += y
			sxx += x * x
			syy += y * y
			sxy += x * y
			n++
		}
		vx, vy := sxx-sx*sx/n, syy-sy*sy/n
		if vx/n < 400 || vy/n < 400 {
			return corr, gain, false
		}
		corr[ch] = (sxy - sx*sy/n) / math.Sqrt(vx*vy)
		gain[ch] = math.Sqrt(vy / vx)
	}
	return corr, gain, true
}
