package dualaudio

import "math"

// VisualAudioSeamJump reports an abrupt change in source-audio position at an
// adjacent segment boundary. A jump is a review signal, not proof of an
// audible defect: crossfades, scene cuts and different mixes affect playback.
type VisualAudioSeamJump struct {
	BaseTime      float64 `json:"base_time"`
	PreviousLag   float64 `json:"previous_lag"`
	NextLag       float64 `json:"next_lag"`
	SourceJumpSec float64 `json:"source_jump_sec"`
}

// FindVisualAudioSeamJumps compares adjacent mapped segments only. It does not
// infer anything about gaps, crossfades or source lip sync.
func FindVisualAudioSeamJumps(segments []Segment, threshold float64) []VisualAudioSeamJump {
	if threshold <= 0 || math.IsNaN(threshold) || math.IsInf(threshold, 0) {
		return nil
	}
	var out []VisualAudioSeamJump
	for i := 1; i < len(segments); i++ {
		a, b := segments[i-1], segments[i]
		if math.IsNaN(a.BaseEnd) || math.IsInf(a.BaseEnd, 0) || math.IsNaN(b.BaseStart) || math.IsInf(b.BaseStart, 0) || math.IsNaN(a.Lag) || math.IsInf(a.Lag, 0) || math.IsNaN(b.Lag) || math.IsInf(b.Lag, 0) {
			continue
		}
		jump := a.Lag - b.Lag
		if math.Abs(a.BaseEnd-b.BaseStart) > .001 || math.Abs(jump) < threshold {
			continue
		}
		out = append(out, VisualAudioSeamJump{BaseTime: b.BaseStart, PreviousLag: a.Lag, NextLag: b.Lag, SourceJumpSec: jump})
	}
	return out
}

// VisualBoundaryReview reports a possible misplaced audio boundary as an
// interval, not a precise timestamp or an authorization to change the audio.
// The visual timeline and the dub's audio may not be synchronized themselves.
type VisualBoundaryReview struct {
	AudioBoundary   float64 `json:"audio_boundary"`
	Earliest        float64 `json:"earliest"`
	Latest          float64 `json:"latest"`
	PreviousLag     float64 `json:"previous_lag"`
	NextLag         float64 `json:"next_lag"`
	VisualAudioBias float64 `json:"visual_audio_bias"`
	PreviousWindows int     `json:"previous_windows"`
	NextWindows     int     `json:"next_windows"`
}

// FindVisualBoundaryReviews finds a late image change after an earlier audio
// boundary. It uses directly measured visual windows on both sides, learns a
// local bias before the boundary, and refuses rejected or poorly observed
// regions. It never changes an alignment or approves a merge.
func FindVisualBoundaryReviews(timeline *VisualTimeline, segments []Segment) []VisualBoundaryReview {
	if timeline == nil {
		return nil
	}
	// Duplicate or overlapping observations must never count as independent
	// support for either side of a proposed boundary.
	lastEnd := 0.0
	for i, w := range timeline.Windows {
		if !validVisualWindow(w, math.Inf(1)) || (i > 0 && w.Start < lastEnd-.001) {
			return nil
		}
		lastEnd = w.End
	}
	const tolerance, minLagDifference, maxGap, horizon = .125, .30, 6.0, 90.0
	var out []VisualBoundaryReview
	for i := 1; i < len(segments); i++ {
		a, b := segments[i-1], segments[i]
		if !finiteSeam(a.BaseStart) || !finiteSeam(a.BaseEnd) || !finiteSeam(a.Lag) || !finiteSeam(b.BaseStart) || !finiteSeam(b.BaseEnd) || !finiteSeam(b.Lag) || a.BaseStart >= a.BaseEnd || b.BaseStart >= b.BaseEnd || b.BaseStart < a.BaseEnd-.001 || b.BaseStart-a.BaseEnd > 1 || math.Abs(a.Lag-b.Lag) < minLagDifference {
			continue
		}
		var anchors []float64
		for _, w := range timeline.Windows {
			if w.Contextual || !validVisualWindow(w, a.BaseEnd) || w.Start < a.BaseEnd-20 || w.End > a.BaseEnd || w.Start < a.BaseStart || overlapsRejected(w.Start, w.End, timeline.Rejected) {
				continue
			}
			anchors = append(anchors, w.Lag-a.Lag)
		}
		if len(anchors) < 3 {
			continue
		}
		bias := median(anchors)
		stable := true
		for _, v := range anchors {
			if math.Abs(v-bias) > tolerance {
				stable = false
				break
			}
		}
		if !stable {
			continue
		}
		prevEnd := a.BaseEnd
		nextStart := -1.0
		prevCount, nextCount := 0, 0
		lastObserved := a.BaseEnd
		valid := true
		for _, w := range timeline.Windows {
			if w.Contextual || w.Start < b.BaseStart || w.Start > b.BaseStart+horizon || w.End > b.BaseEnd {
				continue
			}
			if !validVisualWindow(w, b.BaseEnd) || w.Start < lastObserved-.001 || w.Start-lastObserved > maxGap || overlapsRejected(lastObserved, w.End, timeline.Rejected) {
				valid = false
				break
			}
			prevMatch := math.Abs(w.Lag-(a.Lag+bias)) <= tolerance
			nextMatch := math.Abs(w.Lag-(b.Lag+bias)) <= tolerance
			if prevMatch && !nextMatch && nextStart < 0 {
				prevCount++
				prevEnd = w.End
			} else if nextMatch && !prevMatch && prevCount >= 3 {
				if nextStart < 0 {
					nextStart = w.Start
				}
				nextCount++
			} else {
				valid = false
				break
			}
			lastObserved = w.End
			if nextCount >= 3 {
				break
			}
		}
		if valid && prevCount >= 3 && nextCount >= 3 && nextStart >= prevEnd && prevEnd > b.BaseStart {
			out = append(out, VisualBoundaryReview{b.BaseStart, prevEnd, nextStart, a.Lag, b.Lag, bias, prevCount, nextCount})
		}
	}
	return out
}

func finiteSeam(v float64) bool { return !math.IsNaN(v) && !math.IsInf(v, 0) }
