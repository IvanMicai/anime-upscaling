package dualaudio

import (
	"context"
	"fmt"
	"math"
)

// SourceWaveformAudit is a source-content screening, not a sync verdict.
// A matching window can be dominated by music or effects and can contain a
// different source for part of the window. Inconclusive is not a mismatch.
type SourceWaveformAudit struct {
	StrideSeconds float64                     `json:"stride_seconds"`
	Windows       []SourceWaveformAuditWindow `json:"windows"`
}
type SourceWaveformAuditWindow struct {
	BaseStart float64                    `json:"base_start"`
	Status    string                     `json:"status"`
	Seeds     []float64                  `json:"seed_lags,omitempty"`
	Matches   []SourceWaveformAuditMatch `json:"matches,omitempty"`
}
type SourceWaveformAuditMatch struct {
	SeedLag      float64          `json:"seed_lag"`
	EffectiveLag float64          `json:"effective_lag"`
	Probe        WaveformEvidence `json:"probe"`
}

// AuditSourceWaveform compares 3-second queries against the rendered track.
// Adjacent map segments supply candidate offsets for windows that cross gaps
// or seams. It does not modify the map, approve a merge or establish lip sync.
func AuditSourceWaveform(ctx context.Context, rendered, dub []int16, a *Alignment, stride float64) (SourceWaveformAudit, error) {
	out := SourceWaveformAudit{StrideSeconds: stride, Windows: []SourceWaveformAuditWindow{}}
	if _, err := CommonCoverageSegments(a, a); err != nil {
		return out, err
	}
	if !finiteSeam(stride) || stride < 3 || stride > 60 || len(rendered)%2 != 0 || len(dub)%2 != 0 {
		return out, fmt.Errorf("stereo PCM and stride between 3 and 60 seconds are required")
	}
	end := float64(len(rendered)/2) / featRate
	dubEnd := float64(len(dub)/2) / featRate
	if end <= 0 || end > a.BaseDuration+.25 || math.Abs(dubEnd-a.DubDuration) > .25 || end > 7200 || dubEnd > 7200 {
		return out, fmt.Errorf("PCM durations do not match alignment or exceed two hours")
	}
	for at := 1.5; at+4.5 <= end; at += stride {
		if err := ctx.Err(); err != nil {
			return out, err
		}
		w := SourceWaveformAuditWindow{BaseStart: at, Status: "inconclusive"}
		add := func(lag float64) {
			for _, x := range w.Seeds {
				if math.Abs(x-lag) < .02 {
					return
				}
			}
			w.Seeds = append(w.Seeds, lag)
		}
		covered := false
		for _, s := range a.Segments {
			if s.BaseStart <= at && s.BaseEnd >= at+3 {
				add(s.Lag)
				covered = true
				break
			}
		}
		if !covered {
			left, right := -1, -1
			for i, s := range a.Segments {
				if s.BaseStart < at+3 && s.BaseEnd > at {
					add(s.Lag)
				}
				if s.BaseEnd <= at {
					left = i
				}
				if right < 0 && s.BaseStart >= at+3 {
					right = i
				}
			}
			if left >= 0 && at-a.Segments[left].BaseEnd <= 30 {
				add(a.Segments[left].Lag)
			}
			if right >= 0 && a.Segments[right].BaseStart-(at+3) <= 30 {
				add(a.Segments[right].Lag)
			}
		}
		if len(w.Seeds) == 0 {
			w.Status = "unanchored"
			out.Windows = append(out.Windows, w)
			continue
		}
		if len(w.Seeds) > 4 {
			w.Status = "too_many_seeds"
			out.Windows = append(out.Windows, w)
			continue
		}
		for _, lag := range w.Seeds {
			start := at - lag
			if start < 0 || start+3 > dubEnd {
				continue
			}
			probe, err := MatchStereoWaveform(rendered, dub, at, start)
			if err != nil {
				return out, err
			}
			if probe.Accepted {
				w.Matches = append(w.Matches, SourceWaveformAuditMatch{SeedLag: lag, EffectiveLag: lag + probe.OffsetSec, Probe: probe})
			}
		}
		if len(w.Matches) > 0 {
			w.Status = "match"
			for _, m := range w.Matches[1:] {
				if math.Abs(m.EffectiveLag-w.Matches[0].EffectiveLag) > .04 {
					w.Status = "ambiguous"
					break
				}
			}
		}
		out.Windows = append(out.Windows, w)
	}
	return out, nil
}
