package dualaudio

import (
	"fmt"
	"math"
)

// VisualAudioRegion describes audio evidence near a visually supported offset.
// A correction is a hypothesis, not proof that the dubbed source has correct
// lip sync: two releases can use different music and effects edits.
type VisualAudioRegion struct {
	Start        float64   `json:"start"`
	End          float64   `json:"end"`
	VisualLag    float64   `json:"visual_lag"`
	Correction   float64   `json:"audio_correction"`
	Evidence     int       `json:"evidence"`
	Accepted     bool      `json:"accepted"`
	SupportTimes []float64 `json:"support_times,omitempty"`
}

// VisualAudioCandidate is an experimental map. Gaps are filled separately;
// the candidate is not an approval to write or publish the merged episode.
type VisualAudioCandidate struct {
	Alignment *Alignment          `json:"alignment"`
	Regions   []VisualAudioRegion `json:"regions"`
}

// VisualAudioAlign builds an audio-corrected candidate from measured visual
// windows. It bridges at most four seconds between agreeing windows. Audio
// evidence can adjust a region only when at least three non-overlapping
// windows agree; an absent or conflicting correction remains unverified.
func VisualAudioAlign(timeline *VisualTimeline, base, dub *Features, baseDuration, dubDuration, minConfidence float64) (*VisualAudioCandidate, error) {
	if timeline == nil || base == nil || dub == nil || baseDuration <= 0 || dubDuration <= 0 {
		return nil, fmt.Errorf("visual timeline, audio features and positive durations are required")
	}
	type group struct {
		start, end, lag float64
		windows         []VisualWindow
	}
	var groups []group
	lastEnd := 0.0
	for i, w := range timeline.Windows {
		if math.IsNaN(w.Start) || math.IsNaN(w.End) || math.IsNaN(w.Lag) || math.IsInf(w.Start, 0) || math.IsInf(w.End, 0) || math.IsInf(w.Lag, 0) || w.Start < 0 || w.End <= w.Start || w.End > baseDuration || (i > 0 && w.Start < lastEnd) {
			return nil, fmt.Errorf("invalid or overlapping visual window at index %d", i)
		}
		lastEnd = w.End
	}
	for _, w := range rejectIsolatedVisualWindows(timeline.Windows) {
		n := len(groups)
		if n > 0 && w.Start-groups[n-1].end <= 30 && math.Abs(w.Lag-groups[n-1].lag) <= 0.10 {
			groups[n-1].end = w.End
			groups[n-1].windows = append(groups[n-1].windows, w)
		} else {
			groups = append(groups, group{start: w.Start, end: w.End, lag: w.Lag, windows: []VisualWindow{w}})
		}
	}
	result := &VisualAudioCandidate{Alignment: &Alignment{BaseDuration: baseDuration, DubDuration: dubDuration}, Regions: []VisualAudioRegion{}}
	for _, g := range groups {
		region := VisualAudioRegion{Start: g.start, End: g.end, VisualLag: g.lag}
		var corrections []float64
		var correctionTimes []float64
		// Non-overlapping windows avoid counting the same sound repeatedly.
		for at := g.start; at+10 <= g.end; at += 10 {
			// Only measure where the full window is covered by visual evidence.
			coveredUntil := at
			for _, w := range g.windows {
				if w.End <= coveredUntil {
					continue
				}
				if w.Start > coveredUntil+0.001 {
					break
				}
				coveredUntil = w.End
				if coveredUntil >= at+10 {
					break
				}
			}
			if coveredUntil < at+10 {
				continue
			}
			p0 := int(math.Round((at - g.lag) * fps))
			length := int(10 * fps)
			if p0 < 0 || p0+length > dub.Frames {
				continue
			}
			lag, confidence, ok := matchWindow(base, dub.slice(p0, p0+length), p0, g.lag*fps, int(5*fps))
			correction := lag/fps - g.lag
			if ok && confidence >= minConfidence && math.Abs(correction) <= 0.5 {
				corrections = append(corrections, correction)
				correctionTimes = append(correctionTimes, at+5)
			}
		}
		region.Correction, region.Evidence, region.Accepted = consistentAudioCorrection(corrections)
		var supportTimes []float64
		if region.Accepted {
			for i, correction := range corrections {
				if math.Abs(correction-region.Correction) <= 0.04 {
					supportTimes = append(supportTimes, correctionTimes[i])
				}
			}
		}
		region.SupportTimes = supportTimes
		result.Regions = append(result.Regions, region)
		for _, w := range g.windows {
			lag := g.lag
			if nearAudioEvidence((w.Start+w.End)/2, supportTimes) {
				lag += region.Correction
			}
			segs := result.Alignment.Segments
			if n := len(segs); n > 0 && w.Start-segs[n-1].BaseEnd <= 4 && math.Abs(lag-segs[n-1].Lag) < 0.001 {
				result.Alignment.Segments[n-1].BaseEnd = w.End
				result.Alignment.Segments[n-1].Windows++
			} else {
				result.Alignment.Segments = append(segs, Segment{BaseStart: w.Start, BaseEnd: w.End, Lag: lag, Windows: 1})
			}
		}
	}
	result.Alignment.Gaps, result.Alignment.Coverage = gapsAndCoverage(result.Alignment.Segments, baseDuration)
	return result, nil
}

// Do not extend a correction through long stretches without independent
// measurements: the dub or soundtrack may have a local edit in between.
func nearAudioEvidence(at float64, times []float64) bool {
	for _, t := range times {
		if math.Abs(t-at) <= 30 {
			return true
		}
	}
	return false
}

func consistentAudioCorrection(corrections []float64) (float64, int, bool) {
	if len(corrections) == 0 {
		return 0, 0, false
	}
	m := median(corrections)
	var agreeing []float64
	for _, c := range corrections {
		if math.Abs(c-m) <= 0.04 {
			agreeing = append(agreeing, c)
		}
	}
	if len(agreeing) < 3 || len(agreeing)*5 < len(corrections)*3 {
		return 0, len(agreeing), false
	}
	return median(agreeing), len(agreeing), true
}
