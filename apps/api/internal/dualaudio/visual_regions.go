package dualaudio

import (
	"context"
	"fmt"
	"math"
	"sort"
	"strconv"
)

// VisualRegionEvidence describes one local diagnostic. It does not validate
// dubbing, source lip sync, or authorize replacing any part of a timeline.
type VisualRegionEvidence struct {
	BaseStart     float64              `json:"base_start"`
	DubCenter     float64              `json:"dub_center"`
	Rate          int                  `json:"rate"`
	Regions       []VisualRegionVote   `json:"regions"`
	Agreeing      int                  `json:"agreeing"`
	OffsetFrames  float64              `json:"offset_frames"`
	Consensus     bool                 `json:"consensus"`
	FrameWarnings []VisualFrameWarning `json:"frame_warnings,omitempty"`
}

// VisualFrameWarning identifies a sampled frame where the matching regions'
// median similarity is low. It can be caused by an edit, phase or a flash.
type VisualFrameWarning struct {
	Frame            int     `json:"frame"`
	BaseTime         float64 `json:"base_time"`
	MedianSimilarity float64 `json:"median_similarity"`
}

type VisualRegionVote struct {
	X             int     `json:"x"`
	Y             int     `json:"y"`
	OffsetFrames  int     `json:"offset_frames"`
	Similarity    float64 `json:"similarity"`
	Margin        float64 `json:"margin"`
	ReverseMargin float64 `json:"reverse_margin"`
	Accepted      bool    `json:"accepted"`
}

// regionPeakAccepted requires room to compare alternatives on both sides of a
// peak. A peak close to the search boundary may be the visible edge of a
// stronger match outside the decoded interval.
func regionPeakAccepted(offset int, similarity, margin, reverseMargin float64) bool {
	return absInt(offset) <= 32 && similarity >= .8 && margin >= .04 && reverseMargin >= .1
}

const regionWidth, regionHeight, regionRate = 160, 120, 24
const regionPixels = regionWidth * regionHeight

// CompareVisualRegions decodes a two-second base query and a five-second dub
// search around the supplied center. This bounded diagnostic never alters media
// or timelines. Thresholds are experimental, not a calibrated sync gate.
func CompareVisualRegions(ctx context.Context, t Tools, base, dub string, baseStart, dubCenter float64) (VisualRegionEvidence, error) {
	out := VisualRegionEvidence{BaseStart: baseStart, DubCenter: dubCenter, Rate: regionRate}
	if math.IsNaN(baseStart) || math.IsInf(baseStart, 0) || math.IsNaN(dubCenter) || math.IsInf(dubCenter, 0) || baseStart < 0 || dubCenter < 1.5 {
		return out, fmt.Errorf("invalid region query timestamps")
	}
	// Align both seeks to the same 24 fps grid. Without this, the fps filter
	// starts at a different phase for different search centers, so changing
	// only the center can change the inferred source time by almost a frame.
	// Return effective timestamps so callers can reconstruct the match.
	baseStart = snapVisualRegionTime(baseStart)
	dubCenter = snapVisualRegionTime(dubCenter)
	out.BaseStart, out.DubCenter = baseStart, dubCenter
	bd, err := t.Duration(ctx, base)
	if err != nil {
		return out, err
	}
	dd, err := t.Duration(ctx, dub)
	if err != nil {
		return out, err
	}
	if baseStart+2 > bd || dubCenter+3.5 > dd {
		return out, fmt.Errorf("region query and search must fit within both videos")
	}
	decode := func(path string, start, duration float64) ([]byte, error) {
		raw, e := t.run(ctx, t.FFmpeg, nil, "-v", "error", "-nostdin", "-ss", strconv.FormatFloat(start, 'f', 6, 64), "-t", strconv.FormatFloat(duration, 'f', 6, 64), "-threads", "2", "-i", path, "-map", "0:v:0", "-vf", "fps=24,scale=160:120,format=gray", "-an", "-f", "rawvideo", "-")
		if e != nil {
			return nil, e
		}
		if len(raw)%regionPixels != 0 {
			return nil, fmt.Errorf("incomplete decoded video frame")
		}
		return raw, nil
	}
	q, err := decode(base, baseStart, 2)
	if err != nil {
		return out, err
	}
	s, err := decode(dub, dubCenter-1.5, 5)
	if err != nil {
		return out, err
	}
	// The fps filter can emit one extra boundary frame, depending on the
	// source timestamps. Keep the requested interval; never pad a short read.
	if len(q) >= 48*regionPixels {
		q = q[:48*regionPixels]
	}
	if len(s) >= 120*regionPixels {
		s = s[:120*regionPixels]
	}
	return compareRegionPixels(q, s, baseStart, dubCenter)
}

func snapVisualRegionTime(t float64) float64 {
	return math.Round(t*regionRate) / regionRate
}

// compareRegionPixels uses per-frame normalized correlations, temporal reverse
// controls and a distinct peak. Agreement is evaluated across independently
// located patches but is not statistically independent evidence.
func compareRegionPixels(q, s []byte, baseStart, dubCenter float64) (VisualRegionEvidence, error) {
	out := VisualRegionEvidence{BaseStart: baseStart, DubCenter: dubCenter, Rate: regionRate}
	if len(q) != 48*regionPixels || len(s) != 120*regionPixels {
		return out, fmt.Errorf("expected exactly 48 query and 120 search frames")
	}
	votes := make([]int, 0, 16)
	type regionFrames struct {
		offset int
		scores [48]float64
	}
	var acceptedFrames []regionFrames
	for y := 0; y < 120; y += 30 {
		for x := 0; x < 160; x += 40 {
			normalize := func(data []byte, n int) [][]float32 {
				frames := make([][]float32, n)
				for f := 0; f < n; f++ {
					v := make([]float32, 1200)
					var sum float64
					k := 0
					for yy := 0; yy < 30; yy++ {
						p := f*regionPixels + (y+yy)*regionWidth + x
						for xx := 0; xx < 40; xx++ {
							z := float32(data[p+xx])
							v[k] = z
							sum += float64(z)
							k++
						}
					}
					mean := float32(sum / 1200)
					var power float64
					for i := range v {
						v[i] -= mean
						power += float64(v[i] * v[i])
					}
					if power > 0 {
						scale := float32(1 / math.Sqrt(power))
						for i := range v {
							v[i] *= scale
						}
					}
					frames[f] = v
				}
				return frames
			}
			a, b := normalize(q, 48), normalize(s, 120)
			scores := make([]float64, 73)
			best := 0
			for shift := 0; shift < 73; shift++ {
				var sum float64
				for f := 0; f < 48; f++ {
					var dot float32
					for i, v := range a[f] {
						dot += v * b[f+shift][i]
					}
					sum += float64(dot)
				}
				scores[shift] = sum / 48
				if scores[shift] > scores[best] {
					best = shift
				}
			}
			runner := math.Inf(-1)
			for i, v := range scores {
				if absInt(i-best) > 3 && v > runner {
					runner = v
				}
			}
			var reverse float64
			for f := 0; f < 48; f++ {
				var dot float32
				for i, v := range a[f] {
					dot += v * b[best+47-f][i]
				}
				reverse += float64(dot)
			}
			reverse /= 48
			v := VisualRegionVote{X: x, Y: y, OffsetFrames: best - 36, Similarity: scores[best], Margin: scores[best] - runner, ReverseMargin: scores[best] - reverse}
			v.Accepted = regionPeakAccepted(v.OffsetFrames, v.Similarity, v.Margin, v.ReverseMargin)
			if v.Accepted {
				votes = append(votes, v.OffsetFrames)
				frames := regionFrames{offset: v.OffsetFrames}
				for f := 0; f < 48; f++ {
					var dot float32
					for i, value := range a[f] {
						dot += value * b[f+best][i]
					}
					frames.scores[f] = float64(dot)
				}
				acceptedFrames = append(acceptedFrames, frames)
			}
			out.Regions = append(out.Regions, v)
		}
	}
	center, bestCount := 0, 0
	for c := -36; c <= 36; c++ {
		n := 0
		for _, v := range votes {
			if absInt(v-c) <= 1 {
				n++
			}
		}
		if n > bestCount {
			center, bestCount = c, n
		}
	}
	group := make([]int, 0, bestCount)
	for _, v := range votes {
		if absInt(v-center) <= 1 {
			group = append(group, v)
		}
	}
	sort.Ints(group)
	out.Agreeing = len(group)
	if len(group) > 0 {
		out.OffsetFrames = float64(group[len(group)/2])
		if len(group)%2 == 0 {
			out.OffsetFrames = float64(group[len(group)/2-1]+group[len(group)/2]) / 2
		}
	}
	out.Consensus = out.Agreeing >= 3 && 10*out.Agreeing >= 7*len(votes)
	if out.Consensus {
		for f := 0; f < 48; f++ {
			values := make([]float64, 0, out.Agreeing)
			for _, region := range acceptedFrames {
				if absInt(region.offset-center) <= 1 {
					values = append(values, region.scores[f])
				}
			}
			sort.Float64s(values)
			median := values[len(values)/2]
			if len(values)%2 == 0 {
				median = (values[len(values)/2-1] + median) / 2
			}
			if median < .65 {
				out.FrameWarnings = append(out.FrameWarnings, VisualFrameWarning{Frame: f, BaseTime: baseStart + float64(f)/regionRate, MedianSimilarity: median})
			}
		}
	}
	return out, nil
}
