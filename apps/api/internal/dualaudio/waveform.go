package dualaudio

import (
	"fmt"
	"math"
	"sort"
)

// WaveformVisualDisagreement is repeated evidence that shared sound and the
// image imply different offsets. It does not decide which source is in sync.
type WaveformVisualDisagreement struct {
	Start     float64 `json:"start"`
	End       float64 `json:"end"`
	MinOffset float64 `json:"min_offset_seconds"`
	MaxOffset float64 `json:"max_offset_seconds"`
	Windows   int     `json:"non_overlapping_windows"`
}

// FindWaveformVisualDisagreements groups accepted, non-overlapping 3-second
// probes with consistent signed offsets. Probes are measured against a visual
// timeline by ProbeVisualWaveform. Repeated sound is not proof of lip sync.
func FindWaveformVisualDisagreements(probes []WaveformEvidence) []WaveformVisualDisagreement {
	sorted := append([]WaveformEvidence(nil), probes...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].BaseStart < sorted[j].BaseStart })
	var out []WaveformVisualDisagreement
	var group WaveformVisualDisagreement
	flush := func() {
		if group.Windows >= 3 {
			out = append(out, group)
		}
		group = WaveformVisualDisagreement{}
	}
	for _, p := range sorted {
		if !p.Accepted || !finiteSeam(p.BaseStart) || !finiteSeam(p.OffsetSec) || p.BaseStart < 0 || math.Abs(p.OffsetSec) < .15 {
			flush()
			continue
		}
		if group.Windows > 0 && p.BaseStart < group.End {
			continue
		}
		if group.Windows > 0 && (p.BaseStart-group.End > 15 || math.Max(group.MaxOffset, p.OffsetSec)-math.Min(group.MinOffset, p.OffsetSec) > .08) {
			flush()
		}
		if group.Windows == 0 {
			group = WaveformVisualDisagreement{Start: p.BaseStart, End: p.BaseStart + 3, MinOffset: p.OffsetSec, MaxOffset: p.OffsetSec, Windows: 1}
		} else {
			group.End = p.BaseStart + 3
			group.MinOffset = math.Min(group.MinOffset, p.OffsetSec)
			group.MaxOffset = math.Max(group.MaxOffset, p.OffsetSec)
			group.Windows++
		}
	}
	flush()
	return out
}

// WaveformEvidence is an experimental comparison of shared stereo sound. A
// positive offset means the corresponding sound occurs later in base than in
// the provided dubbed window. It is not a dialogue or lip-sync verdict.
type WaveformEvidence struct {
	BaseStart        float64 `json:"base_start"`
	DubStart         float64 `json:"dub_start"`
	OffsetSec        float64 `json:"offset_sec"`
	LeftOffsetSec    float64 `json:"left_offset_sec"`
	RightOffsetSec   float64 `json:"right_offset_sec"`
	LeftCorrelation  float64 `json:"left_correlation"`
	RightCorrelation float64 `json:"right_correlation"`
	LeftPeakRatio    float64 `json:"left_peak_ratio"`
	RightPeakRatio   float64 `json:"right_peak_ratio"`
	LeftPolarity     int     `json:"left_polarity,omitempty"`
	RightPolarity    int     `json:"right_polarity,omitempty"`
	Accepted         bool    `json:"accepted"`
}

// MatchStereoWaveform compares a three-second dubbed window to a six-second
// base window centered at baseStart, searching +/-1.5s. Inputs are interleaved
// 16-kHz stereo PCM. It keeps separate channels to avoid mono cancellation.
// Thresholds are exploratory; accepted matches are corroborating evidence only.
func MatchStereoWaveform(base, dub []int16, baseStart, dubStart float64) (WaveformEvidence, error) {
	return MatchStereoWaveformWindow(base, dub, baseStart, dubStart, 3)
}

// MatchStereoWaveformWindow permits shorter probes for locating an edit.
// Sub-three-second probes use stricter thresholds; they are diagnostic only
// and must not be substituted for the three-second gap-recovery gate.
func MatchStereoWaveformWindow(base, dub []int16, baseStart, dubStart, window float64) (WaveformEvidence, error) {
	out := WaveformEvidence{BaseStart: baseStart, DubStart: dubStart}
	if math.IsNaN(window) || math.IsInf(window, 0) || window < .5 || window > 3 {
		return out, fmt.Errorf("waveform window must be between 0.5 and 3 seconds")
	}
	const rate = 4000
	count := int(math.Round(window * rate))
	window = float64(count) / rate
	if math.IsNaN(baseStart) || math.IsInf(baseStart, 0) || math.IsNaN(dubStart) || math.IsInf(dubStart, 0) || baseStart < 1.5 || dubStart < 0 || baseStart+window+1.5 > float64(len(base)/2)/featRate || dubStart+window > float64(len(dub)/2)/featRate {
		return out, fmt.Errorf("waveform window and 1.5-second base search must fit in both stereo inputs")
	}
	if len(base)%2 != 0 || len(dub)%2 != 0 || int(math.Round((baseStart-1.5)*featRate))+4*(count+3*rate) > len(base)/2 || int(math.Round(dubStart*featRate))+4*count > len(dub)/2 {
		return out, fmt.Errorf("waveform inputs contain incomplete stereo samples")
	}
	channel := func(pcm []int16, start float64, n, ch int) []float64 {
		out := make([]float64, n)
		off := int(math.Round(start * featRate))
		for i := range out {
			for j := 0; j < 4; j++ {
				out[i] += float64(pcm[2*(off+4*i+j)+ch]) / 4
			}
		}
		return out
	}
	type peak struct {
		offset, score, ratio float64
		polarity             int
	}
	measure := func(ch int) peak {
		ref := channel(base, baseStart-1.5, count+3*rate, ch)
		win := channel(dub, dubStart, count, ch)
		var sum, energy float64
		for _, v := range win {
			sum += v
			energy += v * v
		}
		energy -= sum * sum / float64(len(win))
		if energy <= float64(len(win))*30*30 {
			return peak{}
		}
		prefix, power := make([]float64, len(ref)+1), make([]float64, len(ref)+1)
		for i, v := range ref {
			prefix[i+1] = prefix[i] + v
			power[i+1] = power[i] + v*v
		}
		corr := crossCorrelate(ref, win)
		scores := make([]float64, len(corr))
		signed := make([]float64, len(corr))
		best := 0
		for i, v := range corr {
			s := prefix[i+len(win)] - prefix[i]
			e := power[i+len(win)] - power[i] - s*s/float64(len(win))
			if e <= float64(len(win))*30*30 {
				continue
			}
			signed[i] = (v - s*sum/float64(len(win))) / math.Sqrt(e*energy)
			scores[i] = math.Abs(signed[i])
			if scores[i] > scores[best] {
				best = i
			}
		}
		var runner float64
		for i, v := range scores {
			if absInt(i-best) > rate/20 && v > runner {
				runner = v
			}
		}
		ratio := 0.
		if runner > 1e-12 {
			ratio = scores[best] / runner
		}
		polarity := 0
		if signed[best] > 0 {
			polarity = 1
		} else if signed[best] < 0 {
			polarity = -1
		}
		return peak{float64(best)/rate - 1.5, scores[best], ratio, polarity}
	}
	l, r := measure(0), measure(1)
	out.LeftOffsetSec, out.RightOffsetSec = l.offset, r.offset
	out.LeftCorrelation, out.RightCorrelation = l.score, r.score
	out.LeftPeakRatio, out.RightPeakRatio = l.ratio, r.ratio
	out.LeftPolarity, out.RightPolarity = l.polarity, r.polarity
	out.OffsetSec = (l.offset + r.offset) / 2
	minScore, minRatio := .20, 1.8
	if window < 3 {
		minScore, minRatio = .75, 2.5
	}
	out.Accepted = l.score >= minScore && r.score >= minScore && l.ratio >= minRatio && r.ratio >= minRatio && math.Abs(l.offset-r.offset) <= .005 && math.Abs(out.OffsetSec) <= .5
	return out, nil
}

// ProbeVisualWaveform compares stereo audio only where a full three-second
// window is covered by visual observations with a consistent visual lag.
func ProbeVisualWaveform(timeline *VisualTimeline, base, dub []int16, at float64) (WaveformEvidence, error) {
	if timeline == nil || math.IsNaN(at) || math.IsInf(at, 0) || at < 0 || overlapsRejected(at, at+3, timeline.Rejected) {
		return WaveformEvidence{}, fmt.Errorf("valid non-rejected visual coverage is required")
	}
	cursor, lag, have := at, 0.0, false
	for _, w := range timeline.Windows {
		if w.End <= cursor {
			continue
		}
		if w.Start > cursor+0.001 {
			break
		}
		if !have {
			lag, have = w.Lag, true
		} else if math.Abs(w.Lag-lag) > .02 {
			return WaveformEvidence{}, fmt.Errorf("visual lag changes inside the probe")
		}
		cursor = math.Min(at+3, w.End)
		if cursor >= at+3 {
			break
		}
	}
	if !have || cursor < at+3 {
		return WaveformEvidence{}, fmt.Errorf("visual observations do not cover the full probe")
	}
	return MatchStereoWaveform(base, dub, at, at-lag)
}
