package dualaudio

import (
	"fmt"
	"math"
)

// SharedAudioWindow flags unusually similar audio, which can be English
// fallback, shared music, or effects. It is not a speech or synchronization verdict.
type SharedAudioWindow struct {
	Start        float64 `json:"start"`
	End          float64 `json:"end"`
	Correlation  float64 `json:"correlation"`
	LagMS        float64 `json:"lag_ms"`
	ReferenceRMS float64 `json:"reference_rms"`
}

// FindSharedAudioWindows compares timestamp-aligned, 8 kHz mono streams in
// independent one-second windows. A +/-6 ms search tolerates codec/resampler
// phase differences. Quiet windows and incomplete edges are excluded. The
// thresholds are exploratory and do not establish completeness or language.
func FindSharedAudioWindows(reference, candidate []int16) ([]SharedAudioWindow, error) {
	const rate, pad = 8000, 48
	if len(reference) < 2*rate || len(candidate) < 2*rate {
		return nil, fmt.Errorf("each input must contain at least two seconds of 8 kHz mono audio")
	}
	n := len(reference)
	if len(candidate) < n {
		n = len(candidate)
	}
	result := make([]SharedAudioWindow, 0)
	for start := rate; start+rate+pad <= n; start += rate {
		x := make([]float64, rate)
		y := make([]float64, rate+2*pad)
		var sx, sy float64
		for i := range x {
			x[i] = float64(reference[start+i])
			sx += x[i]
		}
		for i := range y {
			y[i] = float64(candidate[start-pad+i])
			sy += y[i]
		}
		mx, my := sx/float64(len(x)), sy/float64(len(y))
		var ex float64
		for i := range x {
			x[i] -= mx
			ex += x[i] * x[i]
		}
		for i := range y {
			y[i] -= my
		}
		rms := math.Sqrt(ex / rate)
		if rms <= 100 {
			continue
		}
		dots := crossCorrelate(y, x)
		var best, lag float64
		for i, dot := range dots {
			var sum, energy float64
			for _, v := range y[i : i+rate] {
				sum += v
				energy += v * v
			}
			energy -= sum * sum / rate
			if energy <= 0 {
				continue
			}
			c := dot / math.Sqrt(ex*energy)
			if c > best {
				best = c
				lag = float64(i-pad) * 1000 / rate
			}
		}
		if best > .975 {
			result = append(result, SharedAudioWindow{float64(start) / rate, float64(start+rate) / rate, best, lag, rms})
		}
	}
	return result, nil
}
