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
