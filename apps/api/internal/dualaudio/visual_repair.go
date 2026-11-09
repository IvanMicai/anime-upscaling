package dualaudio

import "math"

// ShouldKeepVisualAudioRepair compares a repair against the same evaluation
// set. It prevents added visual/audio contradictions but is not a holdout or
// an approval to write a final file.
func ShouldKeepVisualAudioRepair(old, next Validation, oldConflict, newConflict float64) bool {
	return !math.IsNaN(oldConflict) && !math.IsNaN(newConflict) && !math.IsInf(oldConflict, 0) && !math.IsInf(newConflict, 0) &&
		old.Eligible > 0 && next.Eligible == old.Eligible && next.Hits >= old.Hits && next.OffSec < old.OffSec &&
		next.Median <= old.Median+.005 && next.P95 <= old.P95+.005 && newConflict <= oldConflict+.001
}

// ProposeVisualAudioRepairs creates a separate candidate by shifting only
// consistently off audio runs. It does not validate the candidate or prove
// lip sync; callers must render and check it independently before using it.
func ProposeVisualAudioRepairs(input *Alignment, runs []OffRun) (*Alignment, []OffRun) {
	if input == nil {
		return nil, nil
	}
	out := *input
	out.Segments = append([]Segment(nil), input.Segments...)
	out.Gaps = append([]Gap(nil), input.Gaps...)
	var applied []OffRun
	lastEnd := 0.0
	for _, run := range runs {
		if math.IsNaN(run.Start) || math.IsNaN(run.End) || math.IsNaN(run.Lag) || math.IsInf(run.Start, 0) || math.IsInf(run.End, 0) || math.IsInf(run.Lag, 0) || run.Start < lastEnd || run.Start < 0 || run.End > input.BaseDuration || run.End-run.Start < 20 || run.Windows < 3 || math.Abs(run.Lag) <= hitToleranceSec || math.Abs(run.Lag) > .35 {
			continue
		}
		// Require continuous mapped coverage: a repair must never fill a gap.
		covered := run.Start
		valid := true
		for _, s := range out.Segments {
			if s.BaseEnd <= covered {
				continue
			}
			if s.BaseStart > covered+.001 {
				valid = false
				break
			}
			end := math.Min(s.BaseEnd, run.End)
			start := math.Max(s.BaseStart, run.Start)
			if start-s.Lag-run.Lag < 0 || end-s.Lag-run.Lag > input.DubDuration {
				valid = false
				break
			}
			covered = end
			if covered >= run.End {
				break
			}
		}
		if !valid || covered < run.End {
			continue
		}
		var segments []Segment
		for _, s := range out.Segments {
			if s.BaseEnd <= run.Start || s.BaseStart >= run.End {
				segments = append(segments, s)
				continue
			}
			if s.BaseStart < run.Start {
				left := s
				left.BaseEnd = run.Start
				segments = append(segments, left)
			}
			middle := s
			middle.BaseStart = math.Max(s.BaseStart, run.Start)
			middle.BaseEnd = math.Min(s.BaseEnd, run.End)
			middle.Lag += run.Lag
			segments = append(segments, middle)
			if s.BaseEnd > run.End {
				right := s
				right.BaseStart = run.End
				segments = append(segments, right)
			}
		}
		out.Segments = segments
		lastEnd = run.End
		applied = append(applied, run)
	}
	return &out, applied
}
