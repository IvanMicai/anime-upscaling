package dualaudio

import (
	"context"
	"fmt"
	"math"
	"sort"
)

// VisualGapFill is an experimental, read-only recovery of unmatched windows.
// Recovery does not validate audio or the dubbed source's own lip sync.
type VisualGapFill struct {
	Timeline  *VisualTimeline `json:"timeline"`
	Attempted int             `json:"attempted"`
	Recovered int             `json:"recovered"`
}

// FillVisualGaps decodes both videos at 12 fps. Its cost is proportional to the
// videos' full duration; limit bounds how many missing windows are searched.
func FillVisualGaps(ctx context.Context, t Tools, basePath, dubPath string, input *VisualTimeline, limit int) (*VisualGapFill, error) {
	if input == nil || input.SampleRate != 12 || limit < 1 || limit > 1000 {
		return nil, fmt.Errorf("12-fps timeline and a limit between 1 and 1000 are required")
	}
	baseDur, err := t.Duration(ctx, basePath)
	if err != nil {
		return nil, err
	}
	dubDur, err := t.Duration(ctx, dubPath)
	if err != nil {
		return nil, err
	}
	if baseDur > 7200 || dubDur > 7200 {
		return nil, fmt.Errorf("visual gap analysis supports videos up to two hours")
	}
	if _, err := recoverVisualGaps(input, nil, nil, baseDur, limit); err != nil {
		return nil, err
	}
	_, basePixels, err := t.visualFramesAndPixelsAt(ctx, basePath, 0, baseDur, 12)
	if err != nil {
		return nil, err
	}
	_, dubPixels, err := t.visualFramesAndPixelsAt(ctx, dubPath, 0, dubDur, 12)
	if err != nil {
		return nil, err
	}
	return recoverVisualGaps(input, basePixels, dubPixels, baseDur, limit)
}

func recoverVisualGaps(input *VisualTimeline, base, dub []byte, duration float64, limit int) (*VisualGapFill, error) {
	if input == nil || input.SampleRate != 12 || limit < 1 || limit > 1000 || duration <= 0 || math.IsNaN(duration) || math.IsInf(duration, 0) || len(base)%visualBytes != 0 || len(dub)%visualBytes != 0 {
		return nil, fmt.Errorf("invalid visual gap input")
	}
	for i, w := range input.Windows {
		if !validVisualWindow(w, duration) || (i > 0 && w.Start < input.Windows[i-1].End) || overlapsRejected(w.Start, w.End, input.Rejected) {
			return nil, fmt.Errorf("invalid or overlapping visual window at index %d", i)
		}
	}
	for i, g := range input.Rejected {
		if math.IsNaN(g.Start) || math.IsNaN(g.End) || math.IsInf(g.Start, 0) || math.IsInf(g.End, 0) || g.Start < 0 || g.End <= g.Start || g.End > duration {
			return nil, fmt.Errorf("invalid rejected window at index %d", i)
		}
	}
	timeline := *input
	timeline.Windows = append([]VisualWindow(nil), input.Windows...)
	result := &VisualGapFill{Timeline: &timeline}
	baseFrames, dubFrames := len(base)/visualBytes, len(dub)/visualBytes
	frame := func(data []byte, i int) []byte {
		if i < 0 || (i+1)*visualBytes > len(data) {
			return nil
		}
		return data[i*visualBytes : (i+1)*visualBytes]
	}
	// Near the start a centered three-second query or its earlier control may
	// fall outside the file. Search non-overlapping two-second windows in
	// half-second steps using only the original anchors and valid controls.
	if baseFrames > 0 && dubFrames > 0 {
		for start := 0.5; start < 6 && start+2 <= duration && result.Attempted < limit; start += 0.5 {
			end := start + 2
			if overlapsRejected(start, end, input.Rejected) {
				continue
			}
			occupied := false
			for _, w := range timeline.Windows {
				if w.Start < end && w.End > start {
					occupied = true
					break
				}
			}
			if occupied {
				continue
			}
			nearest, distance := -1, math.Inf(1)
			for i, w := range input.Windows {
				d := math.Abs((w.Start+w.End)/2 - (start + 1))
				if d < distance {
					nearest, distance = i, d
				}
			}
			if nearest < 0 || distance > 90 {
				continue
			}
			center := int(math.Round((start - input.Windows[nearest].Lag) * 12))
			result.Attempted++
			if w, ok := shortVisualGap(base, dub, start, center); ok {
				timeline.Windows = append(timeline.Windows, w)
				result.Recovered++
			}
		}
	}
	for start := 0; float64(start+2) <= duration && result.Attempted < limit; start += 2 {
		end := float64(start + 2)
		if overlapsRejected(float64(start), end, input.Rejected) {
			continue
		}
		occupied := false
		for _, w := range timeline.Windows {
			if w.Start < end && w.End > float64(start) {
				occupied = true
				break
			}
		}
		if occupied {
			continue
		}
		query := start*12 - 6
		if query < 0 || query+36 > baseFrames {
			continue
		}
		// Original accepted anchors only: recovered windows cannot bootstrap
		// additional matches across an unrelated sequence.
		nearest, bestDist := -1, math.Inf(1)
		for i, w := range input.Windows {
			d := math.Abs((w.Start+w.End)/2 - float64(start+1))
			if d < bestDist {
				nearest, bestDist = i, d
			}
		}
		if nearest < 0 || bestDist > 90 {
			continue
		}
		center := query - int(math.Round(input.Windows[nearest].Lag*12))
		result.Attempted++
		// If the wider query crosses an edit, a strict two-second fallback can
		// still recover the target window without assuming its neighbors match.
		shortFallback := func() {
			if w, ok := shortVisualGap(base, dub, float64(start), center+6); ok {
				timeline.Windows = append(timeline.Windows, w)
				result.Recovered++
			}
		}
		score := func(pos int) (float64, float64, bool) {
			if pos < 0 || pos+36 > dubFrames {
				return 0, 0, false
			}
			var sum, block float64
			minBlock := math.Inf(1)
			for i := 0; i < 36; i += 4 {
				v, ok := pixelCorrelation(frame(base, query+i), frame(dub, pos+i))
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
			return sum / 9, minBlock, true
		}
		var scores [61]float64
		var blocks [61]float64
		best := -1
		for i := range scores {
			scores[i] = math.Inf(-1)
			v, block, ok := score(center + i - 30)
			if ok {
				scores[i], blocks[i] = v, block
				if best < 0 || v > scores[best] {
					best = i
				}
			}
		}
		if best < 0 {
			shortFallback()
			continue
		}
		second := math.Inf(-1)
		for i, v := range scores {
			if absInt(i-best) > 1 && v > second {
				second = v
			}
		}
		pos := center + best - 30
		before, _, okBefore := score(pos - 60)
		after, _, okAfter := score(pos + 60)
		margin := scores[best] - second
		if !okBefore || !okAfter || math.IsInf(second, -1) || scores[best] < .84 || blocks[best] < .72 || margin < .04 || scores[best]-math.Max(before, after) < .10 {
			shortFallback()
			continue
		}
		hash, okHash := gapSequenceHashScore(base, dub, query, pos)
		hashBefore, okHashBefore := gapSequenceHashScore(base, dub, query, pos-60)
		hashAfter, okHashAfter := gapSequenceHashScore(base, dub, query, pos+60)
		hashMargin := math.Min(hashBefore, hashAfter) - hash
		if !okHash || !okHashBefore || !okHashAfter || hash >= .35 || hashMargin < .025 {
			shortFallback()
			continue
		}
		timeline.Windows = append(timeline.Windows, VisualWindow{Start: float64(start), End: end, Lag: float64(query-pos) / 12, Contextual: true, GapRecovered: true, SequenceSimilarity: scores[best], SequenceMargin: margin, SequenceHashDistance: hash, SequenceHashMargin: hashMargin})
		result.Recovered++
	}
	sort.Slice(timeline.Windows, func(i, j int) bool { return timeline.Windows[i].Start < timeline.Windows[j].Start })
	return result, nil
}

// gapSequenceHashScore uses the cropped difference hash as a second metric on
// the same sampled frames; this is not independent source material.
func gapSequenceHashScore(base, dub []byte, query, pos int) (float64, bool) {
	if query < 0 || pos < 0 || (query+36)*visualBytes > len(base) || (pos+36)*visualBytes > len(dub) {
		return 0, false
	}
	distance := 0
	for i := 0; i < 36; i += 4 {
		a := base[(query+i)*visualBytes : (query+i+1)*visualBytes]
		b := dub[(pos+i)*visualBytes : (pos+i+1)*visualBytes]
		distance += visualDistance(visualFingerprint(a), visualFingerprint(b))
	}
	return float64(distance) / float64(9*414), true
}

// shortVisualGap matches only the missing window. Its stricter thresholds and
// distant controls compensate partially for the shorter sequence; the result is
// still contextual evidence, not an independently verified sync decision.
func shortVisualGap(base, dub []byte, start float64, center int) (VisualWindow, bool) {
	query := int(math.Round(start * 12))
	frame := func(data []byte, i int) []byte {
		if i < 0 || (i+1)*visualBytes > len(data) {
			return nil
		}
		return data[i*visualBytes : (i+1)*visualBytes]
	}
	score := func(pos int) (float64, float64, float64, bool) {
		var sum, low, hash float64
		low = math.Inf(1)
		for i := 0; i < 24; i += 6 {
			var block float64
			for j := 0; j < 6; j += 3 {
				a, b := frame(base, query+i+j), frame(dub, pos+i+j)
				v, ok := pixelCorrelation(a, b)
				if !ok {
					return 0, 0, 0, false
				}
				block += v
				hash += float64(visualDistance(visualFingerprint(a), visualFingerprint(b)))
			}
			low = math.Min(low, block/2)
			sum += block
		}
		return sum / 8, low, hash / (8 * 414), true
	}
	var scores [61]float64
	var blocks, hashes [61]float64
	best := -1
	for i := range scores {
		scores[i] = math.Inf(-1)
		a, b, h, ok := score(center + i - 30)
		if ok {
			scores[i], blocks[i], hashes[i] = a, b, h
			if best < 0 || a > scores[best] {
				best = i
			}
		}
	}
	if best < 0 {
		return VisualWindow{}, false
	}
	second := math.Inf(-1)
	for i, v := range scores {
		if absInt(i-best) > 1 && v > second {
			second = v
		}
	}
	pos := center + best - 30
	before, _, hashBefore, okBefore := score(pos - 60)
	after, _, hashAfter, okAfter := score(pos + 60)
	// If a control is outside the source, require a second control ten
	// seconds away on the available side; never omit a control silently.
	if !okBefore && pos-60 < 0 && okAfter {
		before, _, hashBefore, okBefore = score(pos + 120)
	}
	if !okAfter && pos+60+24 > len(dub)/visualBytes && okBefore {
		after, _, hashAfter, okAfter = score(pos - 120)
	}
	margin := scores[best] - second
	hashMargin := math.Min(hashBefore, hashAfter) - hashes[best]
	if !okBefore || !okAfter || math.IsInf(second, -1) || scores[best] < .90 || blocks[best] < .85 || margin < .08 || scores[best]-math.Max(before, after) < .20 || hashes[best] >= .35 || hashMargin < .05 {
		return VisualWindow{}, false
	}
	return VisualWindow{Start: start, End: start + 2, Lag: float64(query-pos) / 12, Contextual: true, GapRecovered: true, SequenceSimilarity: scores[best], SequenceMargin: margin, SequenceHashDistance: hashes[best], SequenceHashMargin: hashMargin}, true
}
