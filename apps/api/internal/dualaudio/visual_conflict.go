package dualaudio

import "math"

// VisualAudioConflict identifies where a candidate takes dub audio from a
// noticeably different source time than the visually matching dub frame. It
// is a warning, not proof of a defect: the dub source may itself be out of
// sync, and the visual estimate is sampled rather than frame-perfect.
type VisualAudioConflict struct {
	Start      float64 `json:"start"`
	End        float64 `json:"end"`
	VisualLag  float64 `json:"visual_lag"`
	AudioLag   float64 `json:"audio_lag"`
	Contextual bool    `json:"contextual,omitempty"`
}

// FindVisualAudioConflicts compares only mapped intersections with actual
// visual windows. Gaps and unobserved image regions are not treated as proof.
func FindVisualAudioConflicts(timeline *VisualTimeline, segments []Segment, threshold float64) ([]VisualAudioConflict, float64) {
	if timeline == nil || threshold <= 0 || math.IsNaN(threshold) || math.IsInf(threshold, 0) {
		return nil, 0
	}
	var conflicts []VisualAudioConflict
	var seconds float64
	for _, w := range timeline.Windows {
		for _, s := range segments {
			start, end := math.Max(w.Start, s.BaseStart), math.Min(w.End, s.BaseEnd)
			if end <= start || math.Abs(w.Lag-s.Lag) <= threshold {
				continue
			}
			seconds += end - start
			n := len(conflicts)
			if n > 0 && math.Abs(conflicts[n-1].End-start) < .001 && math.Abs(conflicts[n-1].VisualLag-w.Lag) < .001 && math.Abs(conflicts[n-1].AudioLag-s.Lag) < .001 && conflicts[n-1].Contextual == w.Contextual {
				conflicts[n-1].End = end
			} else {
				conflicts = append(conflicts, VisualAudioConflict{Start: start, End: end, VisualLag: w.Lag, AudioLag: s.Lag, Contextual: w.Contextual})
			}
		}
	}
	return conflicts, seconds
}
