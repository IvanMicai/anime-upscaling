package dualaudio

import "math"

// SourceValidation checks whether rebuilt audio actually contains the dubbed
// source at the times specified by the candidate map. It does not establish
// that the map is correct or that the source itself has good lip sync.
type SourceValidation struct {
	Checked      int     `json:"checked"`
	Matched      int     `json:"matched"`
	Inconclusive int     `json:"inconclusive"`
	MedianSec    float64 `json:"median_sec"`
	P95Sec       float64 `json:"p95_sec"`
	MappedSec    float64 `json:"mapped_sec"`
	ExaminedSec  float64 `json:"examined_sec"`
	Misses       []Miss  `json:"misses,omitempty"`
}

// ValidateDubSource measures non-overlapping 10-second windows wholly inside
// segments, excluding joins and gaps. It independently compares rendered audio
// to the original dub around the source position specified by the map.
func ValidateDubSource(rendered []int16, dub *Features, segs []Segment, minConfidence float64) SourceValidation {
	var result SourceValidation
	if dub == nil || len(rendered) == 0 {
		return result
	}
	output := ExtractFeatures(downmix16k(rendered))
	var residuals []float64
	length := int(10 * fps)
	for _, s := range segs {
		result.MappedSec += math.Max(0, s.BaseEnd-s.BaseStart)
		for at := s.BaseStart + 0.2; at+10 <= s.BaseEnd-0.2; at += 10 {
			p0 := int(math.Round(at * fps))
			if p0 < 0 || p0+length > output.Frames {
				continue
			}
			result.Checked++
			result.ExaminedSec += 10
			lag, confidence, ok := matchWindow(dub, output.slice(p0, p0+length), p0, -s.Lag*fps, int(fps))
			if !ok || confidence < minConfidence {
				result.Inconclusive++
				continue
			}
			residual := lag/fps + s.Lag
			residuals = append(residuals, math.Abs(residual))
			if math.Abs(residual) <= 0.04 {
				result.Matched++
			} else {
				result.Misses = append(result.Misses, Miss{BaseTime: at, Lag: residual, Confidence: confidence})
			}
		}
	}
	if len(residuals) > 0 {
		result.MedianSec, result.P95Sec = median(residuals), percentile(residuals, .95)
	}
	return result
}
