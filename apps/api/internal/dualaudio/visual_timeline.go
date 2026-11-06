package dualaudio

import (
	"context"
	"fmt"
	"math"
	"sort"
)

// VisualTimeline is diagnostic evidence, not an audio synchronization verdict.
// Timestamps refer to the video streams; the dub's own audio/video sync must
// be established separately. Missing matches must not be interpolated blindly.
type VisualTimeline struct {
	SampleRate         int              `json:"sample_rate"`
	Matches            []VisualLocation `json:"matches"`
	Windows            []VisualWindow   `json:"windows"`
	Rejected           []Gap            `json:"rejected,omitempty"`            // pixel check failed; do not bridge
	RejectedCandidates []VisualWindow   `json:"rejected_candidates,omitempty"` // evidence retained for optional refinement
}

// VisualWindow is a two-second comparison using offsets supported by several
// independent scene matches. A missing window is inconclusive, not a gap.
type VisualWindow struct {
	Start            float64 `json:"start"`
	End              float64 `json:"end"`
	Lag              float64 `json:"lag"` // base timestamp minus dub timestamp
	Distance         float64 `json:"distance"`
	Margin           float64 `json:"margin"`
	Contextual       bool    `json:"contextual,omitempty"`       // inferred using neighboring matches
	PixelSimilarity  float64 `json:"pixel_similarity,omitempty"` // independent grayscale check at midpoint
	FineRefined      bool    `json:"fine_refined,omitempty"`     // 24-fps sequence passed distinct-peak checks
	RefinementMargin float64 `json:"refinement_margin,omitempty"`
}

// pixelCorrelation compares brightness patterns, independently of the
// difference hash. Constant frames cannot establish a correspondence.
func pixelCorrelation(a, b []byte) (float64, bool) {
	if len(a) != visualBytes || len(b) != visualBytes {
		return 0, false
	}
	var sx, sy, sxx, syy, sxy, n float64
	for y := 3; y < 21; y++ {
		for x := 4; x < 28; x++ {
			u, v := float64(a[y*visualWidth+x]), float64(b[y*visualWidth+x])
			sx += u
			sy += v
			sxx += u * u
			syy += v * v
			sxy += u * v
			n++
		}
	}
	dx, dy := n*sxx-sx*sx, n*syy-sy*sy
	if dx <= 0 || dy <= 0 {
		return 0, false
	}
	return (n*sxy - sx*sy) / math.Sqrt(dx*dy), true
}

// filterVisualPixelDisagreements rejects individual windows whose midpoint
// fails a second similarity measure or matches a five-second-away control
// substantially better. It does not prove sub-frame or audio synchronization.
func filterVisualPixelDisagreements(windows []VisualWindow, base, dub []byte) ([]VisualWindow, []Gap) {
	frame := func(data []byte, i int) []byte {
		if i < 0 || (i+1)*visualBytes > len(data) {
			return nil
		}
		return data[i*visualBytes : (i+1)*visualBytes]
	}
	out := make([]VisualWindow, 0, len(windows))
	var rejected []Gap
	for _, w := range windows {
		k := int(math.Round((w.Start + w.End) * 6))
		j := k - int(math.Round(w.Lag*12))
		a := frame(base, k)
		score, ok := pixelCorrelation(a, frame(dub, j))
		if !ok || score < 0.85 {
			rejected = append(rejected, Gap{Start: w.Start, End: w.End})
			continue
		}
		before, okBefore := pixelCorrelation(a, frame(dub, j-60))
		after, okAfter := pixelCorrelation(a, frame(dub, j+60))
		if !okBefore || !okAfter || before > score+0.1 || after > score+0.1 {
			rejected = append(rejected, Gap{Start: w.Start, End: w.End})
			continue
		}
		w.PixelSimilarity = score
		out = append(out, w)
	}
	return out, rejected
}

// refineVisual compares a three-second sequence at 12 fps around a coarse
// match. Distant alternatives guard against a repeated or static scene.
func refineVisual(query, search [][7]uint64) (int, float64, float64, bool) {
	if len(query) != 36 || len(search) < len(query)+13 {
		return 0, 0, 0, false
	}
	scores := make([]float64, len(search)-len(query)+1)
	best, bestScore := 0, math.Inf(1)
	for i := range scores {
		for j := range query {
			scores[i] += float64(visualDistance(query[j], search[i+j]))
		}
		scores[i] /= float64(len(query) * 414)
		if scores[i] < bestScore {
			best, bestScore = i, scores[i]
		}
	}
	second := math.Inf(1)
	for i, score := range scores {
		if absInt(i-best) > 6 && score < second {
			second = score
		}
	}
	if math.IsInf(second, 1) {
		return best, bestScore, 0, false
	}
	margin := second - bestScore
	return best, bestScore, margin, bestScore < 0.28 && margin > 0.008
}

// LocateVisualTimeline finds independent coarse scene matches and refines
// them to 1/12 second. It deliberately does not extrapolate across edits.
func LocateVisualTimeline(ctx context.Context, t Tools, basePath, dubPath string) (*VisualTimeline, error) {
	baseDur, err := t.Duration(ctx, basePath)
	if err != nil {
		return nil, err
	}
	dubDur, err := t.Duration(ctx, dubPath)
	if err != nil {
		return nil, err
	}
	if baseDur > 7200 || dubDur > 7200 {
		return nil, fmt.Errorf("visual timeline supports videos up to two hours")
	}
	baseCoarse, err := t.visualFramesAt(ctx, basePath, 0, baseDur, 1)
	if err != nil {
		return nil, err
	}
	dubCoarse, err := t.visualFramesAt(ctx, dubPath, 0, dubDur, 1)
	if err != nil {
		return nil, err
	}
	baseFine, basePixels, err := t.visualFramesAndPixelsAt(ctx, basePath, 0, baseDur, 12)
	if err != nil {
		return nil, err
	}
	dubFine, dubPixels, err := t.visualFramesAndPixelsAt(ctx, dubPath, 0, dubDur, 12)
	if err != nil {
		return nil, err
	}
	result := visualTimeline(baseCoarse, dubCoarse, baseFine, dubFine)
	allWindows := result.Windows
	result.Windows, result.Rejected = filterVisualPixelDisagreements(allWindows, basePixels, dubPixels)
	for _, w := range allWindows {
		if overlapsRejected(w.Start, w.End, result.Rejected) {
			result.RejectedCandidates = append(result.RejectedCandidates, w)
		}
	}
	return result, nil
}

func visualTimeline(base, dub, baseFine, dubFine [][7]uint64) *VisualTimeline {
	result := &VisualTimeline{SampleRate: 12, Matches: []VisualLocation{}, Windows: []VisualWindow{}}
	for second := 15; second+3 < len(base); second += 5 {
		lo, hi := max(0, second-200), min(len(dub), second+205)
		if lo >= hi {
			continue
		}
		idx, _, _, ok := matchVisual(base[second-2:second+3], dub[lo:hi])
		if !ok {
			continue
		}
		coarse := lo + idx + 2
		start := (second - 1) * 12
		center := (coarse - 1) * 12
		left, right := max(0, center-30), min(len(dubFine), center+30+36)
		if start < 0 || start+36 > len(baseFine) || left >= right {
			continue
		}
		idx, distance, margin, ok := refineVisual(baseFine[start:start+36], dubFine[left:right])
		if !ok {
			continue
		}
		result.Matches = append(result.Matches, VisualLocation{TimeA: float64(second), TimeB: float64(left+idx)/12 + 1, Distance: distance, Margin: margin, Matched: true})
	}
	result.Windows = visualWindows(baseFine, dubFine, result.Matches)
	return result
}

func visualWindows(base, dub [][7]uint64, matches []VisualLocation) []VisualWindow {
	var offsets []int
	for _, m := range matches {
		offsets = append(offsets, int(math.Round((m.TimeA-m.TimeB)*12)))
	}
	sort.Ints(offsets)
	var candidates []int
	for i := 0; i < len(offsets); {
		j := i + 1
		for j < len(offsets) && offsets[j]-offsets[i] <= 2 {
			j++
		}
		if j-i >= 3 {
			candidates = append(candidates, offsets[(i+j)/2])
		}
		i = j
	}
	windows := []VisualWindow{}
	if len(candidates) < 2 {
		return windows
	}
	for start := 0; start+24 <= len(base); start += 24 {
		best, second := math.Inf(1), math.Inf(1)
		bestLag := 0
		scores := make([]float64, len(candidates))
		for i, lag := range candidates {
			scores[i] = math.Inf(1)
			p := start - lag
			if p < 0 || p+24 > len(dub) {
				continue
			}
			distance := 0
			for k := 0; k < 24; k++ {
				distance += visualDistance(base[start+k], dub[p+k])
			}
			score := float64(distance) / float64(24*414)
			scores[i] = score
			if score < best {
				best, bestLag = score, lag
			}
		}
		// Adjacent estimates often describe the same scene with a one-frame
		// quantization difference. Only a distinct offset tests ambiguity.
		for i, lag := range candidates {
			if absInt(lag-bestLag) > 3 && scores[i] < second {
				second = scores[i]
			}
		}
		if best < 0.28 && !math.IsInf(second, 1) && second-best > 0.008 {
			windows = append(windows, VisualWindow{Start: float64(start) / 12, End: float64(start+24) / 12, Lag: float64(bestLag) / 12, Distance: best, Margin: second - best})
		}
	}
	return supplementVisualWindows(base, dub, rejectIsolatedVisualWindows(windows))
}

// supplementVisualWindows examines short inconclusive intervals bounded by
// agreeing, independently matched windows. Every intermediate two-second
// sequence must itself resemble the proposed corresponding frames. These
// windows are contextual: static frames and very short edits can still be
// ambiguous, so they must not become an automatic synchronization verdict.
func supplementVisualWindows(base, dub [][7]uint64, windows []VisualWindow) []VisualWindow {
	if len(windows) < 2 {
		return windows
	}
	out := make([]VisualWindow, 0, len(windows))
	for i, left := range windows {
		out = append(out, left)
		if i+1 == len(windows) {
			continue
		}
		right := windows[i+1]
		gap := right.Start - left.End
		if gap <= 0 || gap > 12 || math.Abs(left.Lag-right.Lag) > 1.0/12+0.001 {
			continue
		}
		lag := int(math.Round((left.Lag + right.Lag) * 6))
		var proposed []VisualWindow
		for start := int(math.Round(left.End * 12)); start+24 <= int(math.Round(right.Start*12)); start += 24 {
			p := start - lag
			if start < 0 || start+24 > len(base) || p < 0 || p+24 > len(dub) {
				proposed = nil
				break
			}
			distance := 0
			for k := 0; k < 24; k++ {
				distance += visualDistance(base[start+k], dub[p+k])
			}
			score := float64(distance) / float64(24*414)
			if score >= 0.18 {
				proposed = nil
				break
			}
			proposed = append(proposed, VisualWindow{Start: float64(start) / 12, End: float64(start+24) / 12, Lag: float64(lag) / 12, Distance: score, Contextual: true})
		}
		out = append(out, proposed...)
	}
	return out
}

// An isolated disagreement between nearby windows with the same offset can be
// a repeated shot or a very short edit. In either case there is not enough
// evidence to choose the offset for that window, so leave it inconclusive.
func rejectIsolatedVisualWindows(windows []VisualWindow) []VisualWindow {
	if len(windows) < 3 {
		return windows
	}
	out := make([]VisualWindow, 0, len(windows))
	for i, w := range windows {
		if i > 0 && i+1 < len(windows) {
			before, after := windows[i-1], windows[i+1]
			if w.Start-before.End <= 4 && after.Start-w.End <= 4 &&
				math.Abs(before.Lag-after.Lag) <= 0.25 &&
				math.Abs(w.Lag-before.Lag) > 0.25 && math.Abs(w.Lag-after.Lag) > 0.25 {
				continue
			}
		}
		out = append(out, w)
	}
	return out
}
