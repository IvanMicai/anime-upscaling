package dualaudio

import (
	"fmt"
	"math"
)

// VisualComparison measures evidence only where two alignments differ. It is
// a diagnostic, not proof that either dubbed source has correct lip sync.
type VisualComparison struct {
	ChangedSeconds        float64 `json:"changed_seconds"`
	ObservedSeconds       float64 `json:"observed_seconds"`
	FirstConflictSeconds  float64 `json:"first_conflict_seconds"`
	SecondConflictSeconds float64 `json:"second_conflict_seconds"`
	SufficientCoverage    bool    `json:"sufficient_coverage"`
	VisualMetricNotWorse  bool    `json:"visual_metric_not_worse"`
}

// CompareAlignmentVisualEvidence counts directly observed image windows,
// including contextual windows separately confirmed by a motion comparison,
// over mapped intervals whose audio lags differ. Unobserved or rejected image
// regions are never treated as agreement. The coverage threshold is heuristic.
func CompareAlignmentVisualEvidence(timeline *VisualTimeline, a, b *Alignment, threshold float64) (VisualComparison, error) {
	var out VisualComparison
	if timeline == nil || !finiteSeam(threshold) || threshold <= 0 {
		return out, fmt.Errorf("visual timeline and positive threshold are required")
	}
	if _, err := CommonCoverageSegments(a, b); err != nil {
		return out, err
	}
	for i, g := range timeline.Rejected {
		if !finiteSeam(g.Start) || !finiteSeam(g.End) || g.Start < 0 || g.End <= g.Start || g.End > a.BaseDuration+.001 {
			return out, fmt.Errorf("invalid rejected visual interval at index %d", i)
		}
	}
	last := 0.0
	for i, w := range timeline.Windows {
		if !validVisualWindow(w, a.BaseDuration) || (i > 0 && w.Start < last-.001) {
			return out, fmt.Errorf("invalid or overlapping visual window at index %d", i)
		}
		last = w.End
	}
	i, j := 0, 0
	for i < len(a.Segments) && j < len(b.Segments) {
		x, y := a.Segments[i], b.Segments[j]
		start, end := math.Max(x.BaseStart, y.BaseStart), math.Min(x.BaseEnd, y.BaseEnd)
		if end > start && math.Abs(x.Lag-y.Lag) > threshold {
			out.ChangedSeconds += end - start
			for _, w := range timeline.Windows {
				if (w.Contextual && !w.MotionConfirmed) || w.End <= start || w.Start >= end || overlapsRejected(w.Start, w.End, timeline.Rejected) {
					continue
				}
				lo, hi := math.Max(start, w.Start), math.Min(end, w.End)
				if hi <= lo {
					continue
				}
				n := hi - lo
				out.ObservedSeconds += n
				if math.Abs(w.Lag-x.Lag) > threshold {
					out.FirstConflictSeconds += n
				}
				if math.Abs(w.Lag-y.Lag) > threshold {
					out.SecondConflictSeconds += n
				}
			}
		}
		if x.BaseEnd < y.BaseEnd {
			i++
		} else {
			j++
		}
	}
	// Do not present an empty or mostly unobserved change as corroborated.
	out.SufficientCoverage = out.ChangedSeconds > 0 && out.ObservedSeconds >= .9*out.ChangedSeconds
	out.VisualMetricNotWorse = out.SufficientCoverage && out.SecondConflictSeconds <= out.FirstConflictSeconds+.001
	return out, nil
}
