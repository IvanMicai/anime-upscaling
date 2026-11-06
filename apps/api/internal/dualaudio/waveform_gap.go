package dualaudio

import (
	"context"
	"encoding/binary"
	"fmt"
	"math"
	"strconv"
)

// WaveformGapProposal is a review-only candidate. Shared sound can be music
// or effects, and a visually rejected interval may contain an edit.
type WaveformGapProposal struct {
	Alignment      *Alignment       `json:"alignment"`
	Gap            Gap              `json:"gap"`
	Probe          WaveformEvidence `json:"probe"`
	VisualRejected bool             `json:"visual_rejected"`
}

// waveformGapAnchors requires direct or fine-refined visual support on both
// sides and agreeing audio-map offsets. It never treats a gap as an anchor.
func waveformGapAnchors(a *Alignment, timeline *VisualTimeline, gap Gap) (int, float64, bool, error) {
	if timeline == nil || !finiteSeam(gap.Start) || !finiteSeam(gap.End) || gap.Start < 1.5 || gap.End <= gap.Start || gap.End-gap.Start > 2.001 {
		return 0, 0, false, fmt.Errorf("a visual timeline and a gap up to two seconds are required")
	}
	if _, err := CommonCoverageSegments(a, a); err != nil {
		return 0, 0, false, err
	}
	idx := -1
	for i := 0; i+1 < len(a.Segments); i++ {
		if math.Abs(a.Segments[i].BaseEnd-gap.Start) < .001 && math.Abs(a.Segments[i+1].BaseStart-gap.End) < .001 {
			idx = i
			break
		}
	}
	if idx < 0 {
		return 0, 0, false, fmt.Errorf("gap is not bounded by alignment segments")
	}
	l, r := a.Segments[idx].Lag, a.Segments[idx+1].Lag
	if math.Abs(l-r) > .12 {
		return 0, 0, false, fmt.Errorf("neighboring alignment offsets disagree")
	}
	var left, right bool
	last := 0.0
	for i, w := range timeline.Windows {
		if !validVisualWindow(w, a.BaseDuration) || (i > 0 && w.Start < last-.001) {
			return 0, 0, false, fmt.Errorf("invalid visual window")
		}
		last = w.End
		if overlapsRejected(w.Start, w.End, timeline.Rejected) || (w.Contextual && !w.FineRefined) {
			continue
		}
		if math.Abs(w.End-gap.Start) < .001 && math.Abs(w.Lag-l) <= .12 {
			left = true
		}
		if math.Abs(w.Start-gap.End) < .001 && math.Abs(w.Lag-r) <= .12 {
			right = true
		}
	}
	for _, g := range timeline.Rejected {
		if !finiteSeam(g.Start) || !finiteSeam(g.End) || g.Start < 0 || g.End <= g.Start || g.End > a.BaseDuration+.001 {
			return 0, 0, false, fmt.Errorf("invalid rejected visual interval")
		}
	}
	if !left || !right {
		return 0, 0, false, fmt.Errorf("visual anchors on both sides are required")
	}
	return idx, (l + r) / 2, overlapsRejected(gap.Start, gap.End, timeline.Rejected), nil
}

// ProposeWaveformGap decodes only six seconds of reference and three seconds
// of dub, then tests a shared stereo waveform against adjacent visual anchors.
// The returned map is an experimental hypothesis, never a merge approval.
func ProposeWaveformGap(ctx context.Context, t Tools, basePath, dubPath string, stream int, a *Alignment, timeline *VisualTimeline, gap Gap) (*WaveformGapProposal, error) {
	i, seed, rejected, err := waveformGapAnchors(a, timeline, gap)
	if err != nil {
		return nil, err
	}
	if stream < 0 {
		return nil, fmt.Errorf("invalid audio stream")
	}
	bd, err := t.Duration(ctx, basePath)
	if err != nil {
		return nil, err
	}
	dd, err := t.Duration(ctx, dubPath)
	if err != nil {
		return nil, err
	}
	if bd > 7200 || dd > 7200 || math.Abs(bd-a.BaseDuration) > .25 || math.Abs(dd-a.DubDuration) > .25 {
		return nil, fmt.Errorf("alignment durations do not match files or exceed two hours")
	}
	dubStart := gap.Start - seed
	probe, err := ProbeStereoWaveformWindow(ctx, t, basePath, dubPath, stream, gap.Start, dubStart, 3)
	if err != nil {
		return nil, err
	}
	if !probe.Accepted {
		return nil, fmt.Errorf("shared stereo waveform did not pass confidence checks")
	}
	lag := gap.Start - dubStart + probe.OffsetSec
	if math.Abs(lag-a.Segments[i].Lag) > .12 || math.Abs(lag-a.Segments[i+1].Lag) > .12 {
		return nil, fmt.Errorf("shared waveform conflicts with adjacent offsets")
	}
	result := *a
	result.Segments = make([]Segment, 0, len(a.Segments)+1)
	result.Segments = append(result.Segments, a.Segments[:i+1]...)
	result.Segments = append(result.Segments, Segment{BaseStart: gap.Start, BaseEnd: gap.End, Lag: lag, Windows: 1})
	result.Segments = append(result.Segments, a.Segments[i+1:]...)
	result.Gaps, result.Coverage = gapsAndCoverage(result.Segments, a.BaseDuration)
	if _, err := CommonCoverageSegments(&result, &result); err != nil {
		return nil, err
	}
	return &WaveformGapProposal{Alignment: &result, Gap: gap, Probe: probe, VisualRejected: rejected}, nil
}

// ProbeStereoWaveformWindow decodes bounded excerpts; short matches are
// diagnostic only and never authorize a gap fill or an output file.
func ProbeStereoWaveformWindow(ctx context.Context, t Tools, basePath, dubPath string, stream int, baseStart, dubStart, window float64) (WaveformEvidence, error) {
	var empty WaveformEvidence
	if stream < 0 || !finiteSeam(baseStart) || !finiteSeam(dubStart) || !finiteSeam(window) || baseStart < 1.5 || dubStart < 0 || window < .5 || window > 3 {
		return empty, fmt.Errorf("invalid waveform probe")
	}
	bd, err := t.Duration(ctx, basePath)
	if err != nil {
		return empty, err
	}
	dd, err := t.Duration(ctx, dubPath)
	if err != nil {
		return empty, err
	}
	// Demuxer seek and resampler rounding can leave a requested interval a few
	// samples short. Decode 200 ms extra, which the matcher does not consume.
	if bd > 7200 || dd > 7200 || baseStart+window+1.7 > bd || dubStart+window+.2 > dd {
		return empty, fmt.Errorf("waveform probe exceeds source bounds")
	}
	decode := func(path string, start, dur float64, track int) ([]int16, error) {
		raw, e := t.run(ctx, t.FFmpeg, nil, "-v", "error", "-nostdin", "-ss", strconv.FormatFloat(start, 'f', 6, 64), "-t", strconv.FormatFloat(dur, 'f', 6, 64), "-threads", "2", "-i", path, "-map", fmt.Sprintf("0:a:%d", track), "-vn", "-ac", "2", "-ar", "16000", "-af", "aresample=16000:async=1:first_pts=0", "-f", "s16le", "-")
		if e != nil {
			return nil, e
		}
		if len(raw)%4 != 0 {
			return nil, fmt.Errorf("incomplete PCM")
		}
		pcm := make([]int16, len(raw)/2)
		for j := range pcm {
			pcm[j] = int16(binary.LittleEndian.Uint16(raw[2*j:]))
		}
		return pcm, nil
	}
	base, err := decode(basePath, baseStart-1.5, window+3.2, 0)
	if err != nil {
		return empty, err
	}
	dub, err := decode(dubPath, dubStart, window+.2, stream)
	if err != nil {
		return empty, err
	}
	probe, err := MatchStereoWaveformWindow(base, dub, 1.5, 0, window)
	if err != nil {
		return empty, err
	}
	probe.BaseStart, probe.DubStart = baseStart, dubStart
	return probe, nil
}
