package dualaudio

import (
	"context"
	"fmt"
	"math"
	"sort"
)

// VisualRefinement reports an opt-in pass over rejected scene windows. The
// result remains diagnostic and must not bypass the audio gate.
type VisualRefinement struct {
	Timeline  *VisualTimeline `json:"timeline"`
	Attempted int             `json:"attempted"`
	Recovered int             `json:"recovered"`
}

// RefineVisualRejections decodes short 24-fps clips around rejected windows.
// `limit` bounds the number of attempted windows and must be between 1 and 100.
func RefineVisualRejections(ctx context.Context, t Tools, basePath, dubPath string, input *VisualTimeline, limit int) (*VisualRefinement, error) {
	if input == nil || limit < 1 || limit > 100 {
		return nil, fmt.Errorf("timeline and a limit between 1 and 100 are required")
	}
	if input.SampleRate != 12 {
		return nil, fmt.Errorf("visual refinement requires a 12-fps timeline")
	}
	if len(input.Rejected) > 0 && len(input.RejectedCandidates) == 0 {
		return nil, fmt.Errorf("timeline does not include rejected candidate details; regenerate it")
	}
	baseDur, err := t.Duration(ctx, basePath)
	if err != nil {
		return nil, err
	}
	dubDur, err := t.Duration(ctx, dubPath)
	if err != nil {
		return nil, err
	}
	copyTimeline := *input
	copyTimeline.Windows = append([]VisualWindow(nil), input.Windows...)
	copyTimeline.Rejected = append([]Gap(nil), input.Rejected...)
	copyTimeline.RejectedCandidates = nil
	result := &VisualRefinement{Timeline: &copyTimeline}
	for _, w := range input.RejectedCandidates {
		if result.Attempted >= limit {
			copyTimeline.RejectedCandidates = append(copyTimeline.RejectedCandidates, w)
			continue
		}
		if !validVisualWindow(w, baseDur) {
			return nil, fmt.Errorf("invalid rejected candidate")
		}
		gapIndex, overlaps := -1, 0
		for i, g := range copyTimeline.Rejected {
			if g.Start < w.End && g.End > w.Start {
				overlaps++
				if g.Start == w.Start && g.End == w.End {
					gapIndex = i
				}
			}
		}
		if gapIndex < 0 || overlaps != 1 {
			return nil, fmt.Errorf("rejected candidate does not match exactly one rejected interval")
		}
		queryStart := w.Start - .5
		searchStart := queryStart - w.Lag - 5.5
		if queryStart < 0 || queryStart+3 > baseDur || searchStart < 0 || searchStart+14 > dubDur {
			copyTimeline.RejectedCandidates = append(copyTimeline.RejectedCandidates, w)
			continue
		}
		result.Attempted++
		_, query, err := t.visualFramesAndPixelsAt(ctx, basePath, queryStart, 3, 24)
		if err != nil {
			return nil, err
		}
		_, search, err := t.visualFramesAndPixelsAt(ctx, dubPath, searchStart, 14, 24)
		if err != nil {
			return nil, err
		}
		shift, score, margin, ok := refineVisualSequence24(query, search)
		if !ok {
			copyTimeline.RejectedCandidates = append(copyTimeline.RejectedCandidates, w)
			continue
		}
		w.Lag -= float64(shift) / 24
		w.PixelSimilarity, w.RefinementMargin = score, margin
		w.Contextual, w.FineRefined = true, true
		copyTimeline.Windows = append(copyTimeline.Windows, w)
		copyTimeline.Rejected = append(copyTimeline.Rejected[:gapIndex], copyTimeline.Rejected[gapIndex+1:]...)
		result.Recovered++
	}
	sort.Slice(copyTimeline.Windows, func(i, j int) bool { return copyTimeline.Windows[i].Start < copyTimeline.Windows[j].Start })
	for i := 1; i < len(copyTimeline.Windows); i++ {
		if copyTimeline.Windows[i].Start < copyTimeline.Windows[i-1].End {
			return nil, fmt.Errorf("refined windows overlap")
		}
	}
	return result, nil
}

// The three-second probe covers only a two-second window with half a second of
// context on either side. Never extrapolate it to a longer supplied interval.
func validVisualWindow(w VisualWindow, duration float64) bool {
	return !math.IsNaN(w.Start) && !math.IsNaN(w.End) && !math.IsNaN(w.Lag) &&
		!math.IsInf(w.Start, 0) && !math.IsInf(w.End, 0) && !math.IsInf(w.Lag, 0) &&
		w.Start >= 0 && w.End <= duration && math.Abs(w.End-w.Start-2) <= 1e-6
}

// refineVisualSequence24 compares a three-second sequence at 24 fps against
// nearby positions and five-second-away controls. An offset is returned only
// when the peak is distinct to within one sampled frame. This is experimental
// visual evidence and does not establish the dub's own audio/video sync.
func refineVisualSequence24(query, search []byte) (int, float64, float64, bool) {
	const queryFrames = 72
	const center = 132 // search begins 5.5 seconds before the predicted match
	if len(query) < queryFrames*visualBytes || len(search) < (center+120+6+queryFrames)*visualBytes {
		return 0, 0, 0, false
	}
	score := func(offset int) (float64, float64, bool) {
		var sum, block float64
		minBlock := math.Inf(1)
		for i := 0; i < queryFrames; i += 4 {
			q := query[i*visualBytes : (i+1)*visualBytes]
			j := center + offset + i
			if j < 0 || (j+1)*visualBytes > len(search) {
				return 0, 0, false
			}
			v, ok := pixelCorrelation(q, search[j*visualBytes:(j+1)*visualBytes])
			if !ok {
				return 0, 0, false
			}
			sum += v
			block += v
			if (i/4+1)%3 == 0 {
				minBlock = math.Min(minBlock, block/3)
				block = 0
			}
		}
		return sum / 18, minBlock, true
	}
	var scores [13]float64
	var blocks [13]float64
	best := -1
	for i := range scores {
		v, block, ok := score(i - 6)
		if !ok {
			return 0, 0, 0, false
		}
		scores[i] = v
		blocks[i] = block
		if best < 0 || v > scores[best] {
			best = i
		}
	}
	second := math.Inf(-1)
	for i, v := range scores {
		if absInt(i-best) > 1 && v > second {
			second = v
		}
	}
	before, _, okBefore := score(best - 6 - 120)
	after, _, okAfter := score(best - 6 + 120)
	margin := scores[best] - second
	ok := okBefore && okAfter && scores[best] >= .90 && blocks[best] >= .85 && margin >= .01 && scores[best]-math.Max(before, after) >= .02
	return best - 6, scores[best], margin, ok
}

// RefineVisualAccepted rechecks explicitly selected accepted windows at 24 fps.
// An ambiguous result leaves the existing window unchanged; it never changes
// rejected ranges or approves a merge.
func RefineVisualAccepted(ctx context.Context, t Tools, basePath, dubPath string, input *VisualTimeline, starts []float64) (*VisualRefinement, error) {
	indices, err := selectAcceptedVisualWindows(input, starts)
	if err != nil {
		return nil, err
	}
	baseDur, err := t.Duration(ctx, basePath)
	if err != nil {
		return nil, err
	}
	dubDur, err := t.Duration(ctx, dubPath)
	if err != nil {
		return nil, err
	}
	copyTimeline := *input
	copyTimeline.Windows = append([]VisualWindow(nil), input.Windows...)
	result := &VisualRefinement{Timeline: &copyTimeline}
	for _, i := range indices {
		w := input.Windows[i]
		if !validVisualWindow(w, baseDur) || overlapsRejected(w.Start, w.End, input.Rejected) {
			return nil, fmt.Errorf("invalid accepted window at index %d", i)
		}
		queryStart := w.Start - .5
		searchStart := queryStart - w.Lag - 5.5
		if queryStart < 0 || queryStart+3 > baseDur || searchStart < 0 || searchStart+14 > dubDur {
			continue
		}
		result.Attempted++
		_, query, err := t.visualFramesAndPixelsAt(ctx, basePath, queryStart, 3, 24)
		if err != nil {
			return nil, err
		}
		_, search, err := t.visualFramesAndPixelsAt(ctx, dubPath, searchStart, 14, 24)
		if err != nil {
			return nil, err
		}
		shift, score, margin, ok := refineVisualSequence24(query, search)
		if !ok {
			continue
		}
		w.Lag -= float64(shift) / 24
		w.PixelSimilarity, w.RefinementMargin = score, margin
		w.Contextual, w.FineRefined = true, true
		copyTimeline.Windows[i] = w
		result.Recovered++
	}
	return result, nil
}

func selectAcceptedVisualWindows(input *VisualTimeline, starts []float64) ([]int, error) {
	if input == nil || input.SampleRate != 12 || len(starts) == 0 || len(starts) > 20 {
		return nil, fmt.Errorf("12-fps timeline and 1–20 accepted start times are required")
	}
	indices := make([]int, 0, len(starts))
	seen := make(map[int]bool)
	for _, at := range starts {
		if math.IsNaN(at) || math.IsInf(at, 0) || at < 0 {
			return nil, fmt.Errorf("invalid accepted start time")
		}
		found := -1
		for i, w := range input.Windows {
			if math.Abs(w.Start-at) < 1e-6 {
				if found >= 0 {
					return nil, fmt.Errorf("duplicate window start %.3f", at)
				}
				found = i
			}
		}
		if found < 0 || seen[found] {
			return nil, fmt.Errorf("accepted window %.3f is missing or repeated", at)
		}
		seen[found] = true
		indices = append(indices, found)
	}
	return indices, nil
}
