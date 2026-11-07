package dualaudio

import (
	"fmt"
	"math"
)

// VisualAudioRegion describes audio evidence near a visually supported offset.
// A correction is a hypothesis, not proof that the dubbed source has correct
// lip sync: two releases can use different music and effects edits.
type VisualAudioRegion struct {
	Start        float64            `json:"start"`
	End          float64            `json:"end"`
	VisualLag    float64            `json:"visual_lag"`
	Correction   float64            `json:"audio_correction"`
	Evidence     int                `json:"evidence"`
	Accepted     bool               `json:"accepted"`
	SupportTimes []float64          `json:"support_times,omitempty"`
	Observations []AudioObservation `json:"observations,omitempty"`
}

// AudioObservation is a confident local offset relative to the visual map.
type AudioObservation struct {
	Time       float64 `json:"time"`
	Correction float64 `json:"correction"`
	Confidence float64 `json:"confidence"`
}

// VisualAudioCandidate is an experimental map. Gaps are filled separately;
// the candidate is not an approval to write or publish the merged episode.
type VisualAudioCandidate struct {
	Alignment             *Alignment          `json:"alignment"`
	Regions               []VisualAudioRegion `json:"regions"`
	TrainingWindows       []Gap               `json:"training_windows,omitempty"`
	TrainingSourceWindows []Gap               `json:"training_source_windows,omitempty"`
}

// VisualAudioAlign builds an audio-corrected candidate from measured visual
// windows. It bridges at most four seconds between agreeing windows. Audio
// evidence can adjust a region only when at least three non-overlapping
// windows agree; an absent or conflicting correction remains unverified.
func VisualAudioAlign(timeline *VisualTimeline, base, dub *Features, baseDuration, dubDuration, minConfidence float64) (*VisualAudioCandidate, error) {
	return visualAudioAlign(timeline, base, dub, baseDuration, dubDuration, minConfidence, false)
}

// VisualAudioAlignHoldout estimates corrections only from 10-second windows
// selected away from the 24.5+60*n evaluation windows, leaving a gap between
// the widest reference searches.
// Source-time windows are also recorded to exclude reused dubbed samples.
// Feature normalization still uses whole-track statistics; this is not
// independently annotated truth.
func VisualAudioAlignHoldout(timeline *VisualTimeline, base, dub *Features, baseDuration, dubDuration, minConfidence float64) (*VisualAudioCandidate, error) {
	return visualAudioAlign(timeline, base, dub, baseDuration, dubDuration, minConfidence, true)
}

func visualAudioAlign(timeline *VisualTimeline, base, dub *Features, baseDuration, dubDuration, minConfidence float64, holdout bool) (*VisualAudioCandidate, error) {
	if timeline == nil || base == nil || dub == nil || baseDuration <= 0 || dubDuration <= 0 {
		return nil, fmt.Errorf("visual timeline, audio features and positive durations are required")
	}
	if len(base.Bands) == 0 || len(base.Bands) != len(dub.Bands) || base.Frames <= 0 || dub.Frames <= 0 {
		return nil, fmt.Errorf("base and dub audio feature layouts must match and contain frames")
	}
	for i := range base.Bands {
		if len(base.Bands[i]) < base.Frames || len(dub.Bands[i]) < dub.Frames {
			return nil, fmt.Errorf("incomplete audio feature band %d", i)
		}
	}
	for i, gap := range timeline.Rejected {
		if math.IsNaN(gap.Start) || math.IsNaN(gap.End) || math.IsInf(gap.Start, 0) || math.IsInf(gap.End, 0) || gap.Start < 0 || gap.End <= gap.Start || gap.End > baseDuration {
			return nil, fmt.Errorf("invalid rejected window at index %d", i)
		}
	}
	type group struct {
		start, end, lag float64
		windows         []VisualWindow
		fine            bool
	}
	var groups []group
	lastEnd := 0.0
	for i, w := range timeline.Windows {
		if !validVisualWindow(w, baseDuration) || (i > 0 && w.Start < lastEnd) {
			return nil, fmt.Errorf("invalid or overlapping visual window at index %d", i)
		}
		if overlapsRejected(w.Start, w.End, timeline.Rejected) {
			return nil, fmt.Errorf("visual window %d overlaps rejected evidence", i)
		}
		lastEnd = w.End
	}
	clean := rejectIsolatedVisualWindows(timeline.Windows)
	rejected := append([]Gap(nil), timeline.Rejected...)
	// A second temporal check may discard windows supplied by an older timeline
	// or exposed by filtering. Do not silently bridge over those either.
	next := 0
	for _, w := range timeline.Windows {
		if next < len(clean) && clean[next].Start == w.Start {
			next++
		} else {
			rejected = append(rejected, Gap{Start: w.Start, End: w.End})
		}
	}
	for _, w := range clean {
		n := len(groups)
		measured := w.FineRefined || w.GapRecovered
		differentFineLag := n > 0 && (measured || groups[n-1].fine) && math.Abs(w.Lag-groups[n-1].lag) > 0.02
		if n > 0 && w.Start-groups[n-1].end <= 30 && math.Abs(w.Lag-groups[n-1].lag) <= 0.10 && !differentFineLag {
			groups[n-1].end = w.End
			groups[n-1].windows = append(groups[n-1].windows, w)
			groups[n-1].fine = groups[n-1].fine || measured
		} else {
			groups = append(groups, group{start: w.Start, end: w.End, lag: w.Lag, windows: []VisualWindow{w}, fine: measured})
		}
	}
	result := &VisualAudioCandidate{Alignment: &Alignment{BaseDuration: baseDuration, DubDuration: dubDuration}, Regions: []VisualAudioRegion{}}
	for _, g := range groups {
		region := VisualAudioRegion{Start: g.start, End: g.end, VisualLag: g.lag}
		var corrections []float64
		var correctionTimes []float64
		// Non-overlapping windows avoid counting the same sound repeatedly.
		var positions []float64
		if holdout {
			for at := math.Ceil(g.start); at+10 <= g.end; at++ {
				if holdoutTrainingAllowed(at) {
					positions = append(positions, at)
				}
			}
		} else {
			for at := g.start; at+10 <= g.end; at += 10 {
				positions = append(positions, at)
			}
		}
		nextTraining := math.Inf(-1)
		for _, at := range positions {
			if holdout && at < nextTraining {
				continue
			}
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
			if holdout {
				nextTraining = at + 10
				result.TrainingWindows = append(result.TrainingWindows, Gap{Start: at, End: at + 10})
				sourceStart := float64(p0) / fps
				result.TrainingSourceWindows = append(result.TrainingSourceWindows, Gap{Start: sourceStart, End: sourceStart + 10})
			}
			lag, confidence, ok := matchWindow(base, dub.slice(p0, p0+length), p0, g.lag*fps, int(5*fps))
			correction := lag/fps - g.lag
			if ok && confidence >= minConfidence && math.Abs(correction) <= 0.5 {
				corrections = append(corrections, correction)
				correctionTimes = append(correctionTimes, at+5)
				region.Observations = append(region.Observations, AudioObservation{Time: at + 5, Correction: correction, Confidence: confidence})
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
			if n := len(segs); n > 0 && w.Start-segs[n-1].BaseEnd <= 0.001 && math.Abs(lag-segs[n-1].Lag) < 0.001 && !overlapsRejected(segs[n-1].BaseEnd, w.Start, rejected) {
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

// Each comparison searches the base by +/-5 seconds around a ten-second
// window. Keep the entire training search disjoint from every holdout search.
func holdoutTrainingAllowed(at float64) bool {
	cycle := int(math.Floor(at / 60))
	for i := cycle - 1; i <= cycle+1; i++ {
		if i < 0 {
			continue
		}
		evaluation := 24.5 + float64(i)*60
		if at-5 < evaluation+15 && at+15 > evaluation-5 {
			return false
		}
	}
	return true
}

func overlapsRejected(start, end float64, rejected []Gap) bool {
	for _, g := range rejected {
		if g.Start < end && g.End > start {
			return true
		}
	}
	return false
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
