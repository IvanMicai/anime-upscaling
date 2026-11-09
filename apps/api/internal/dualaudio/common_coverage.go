package dualaudio

import (
	"fmt"
	"math"
)

// CommonCoverageSegments returns intersections of two alignments' mapped
// intervals. It is intended to make validation compare the same timestamps;
// its returned lags are placeholders and must not be used for rendering.
func CommonCoverageSegments(a, b *Alignment) ([]Segment, error) {
	if a == nil || b == nil || !finiteSeam(a.BaseDuration) || !finiteSeam(b.BaseDuration) || !finiteSeam(a.DubDuration) || !finiteSeam(b.DubDuration) || a.BaseDuration <= 0 || b.BaseDuration <= 0 || a.DubDuration <= 0 || b.DubDuration <= 0 || math.Abs(a.BaseDuration-b.BaseDuration) > .001 || math.Abs(a.DubDuration-b.DubDuration) > .001 {
		return nil, fmt.Errorf("alignment durations must be positive and equal")
	}
	check := func(al *Alignment) error {
		end := 0.0
		for i, s := range al.Segments {
			if !finiteSeam(s.BaseStart) || !finiteSeam(s.BaseEnd) || !finiteSeam(s.Lag) || s.BaseStart < 0 || s.BaseStart < end-.000001 || s.BaseEnd <= s.BaseStart || s.BaseEnd > al.BaseDuration+.001 || s.BaseStart-s.Lag < -.05 || s.BaseEnd-s.Lag > al.DubDuration+.05 {
				return fmt.Errorf("invalid or overlapping segment at index %d", i)
			}
			end = s.BaseEnd
		}
		gapEnd := 0.0
		for i, g := range al.Gaps {
			if !finiteSeam(g.Start) || !finiteSeam(g.End) || g.Start < 0 || g.Start < gapEnd-.000001 || g.End <= g.Start || g.End > al.BaseDuration+.001 {
				return fmt.Errorf("invalid or overlapping gap at index %d", i)
			}
			for _, s := range al.Segments {
				if math.Min(g.End, s.BaseEnd)-math.Max(g.Start, s.BaseStart) > .000001 {
					return fmt.Errorf("gap overlaps mapped audio at index %d", i)
				}
			}
			gapEnd = g.End
		}
		return nil
	}
	if err := check(a); err != nil {
		return nil, err
	}
	if err := check(b); err != nil {
		return nil, err
	}
	out := make([]Segment, 0)
	i, j := 0, 0
	for i < len(a.Segments) && j < len(b.Segments) {
		x, y := a.Segments[i], b.Segments[j]
		start, end := math.Max(x.BaseStart, y.BaseStart), math.Min(x.BaseEnd, y.BaseEnd)
		if end > start {
			out = append(out, Segment{BaseStart: start, BaseEnd: end})
		}
		if x.BaseEnd < y.BaseEnd {
			i++
		} else {
			j++
		}
	}
	return out, nil
}
