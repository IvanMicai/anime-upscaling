package dualaudio

import (
	"context"
	"fmt"
	"math"
	"sort"
	"strconv"
)

// VisualMotionEvidence is an experimental comparison, not a dubbing verdict.
type VisualMotionEvidence struct {
	BaseStart     float64              `json:"base_start"`
	DubCenter     float64              `json:"dub_center"`
	OffsetFrames  int                  `json:"offset_frames"`
	Score         float64              `json:"score"`
	RunnerScore   float64              `json:"runner_score"`
	ReverseScore  float64              `json:"reverse_score"`
	ReverseFrames int                  `json:"reverse_frames"`
	ValidFrames   int                  `json:"valid_frames"`
	FrameWarnings []MotionFrameWarning `json:"frame_warnings,omitempty"`
	Accepted      bool                 `json:"accepted"`
}

// MotionFrameWarning records a low-similarity frame within an otherwise
// stronger sequence. A flash, an edit or imperfect sampling can trigger it.
type MotionFrameWarning struct {
	Frame    int     `json:"frame"`
	BaseTime float64 `json:"base_time"`
	Score    float64 `json:"score"`
}

// ConfirmContextualMotion returns a copy with one contextual window marked as
// directly checked by motion. The check is experimental and does not prove
// source audio sync. Offsets and timestamps must agree within one 24-fps frame.
func ConfirmContextualMotion(input *VisualTimeline, at float64, motion VisualMotionEvidence) (*VisualTimeline, error) {
	if input == nil || input.SampleRate != 12 || !finiteSeam(at) || !finiteSeam(motion.BaseStart) || !finiteSeam(motion.DubCenter) || !motion.Accepted || len(motion.FrameWarnings) != 0 || math.Abs(motion.BaseStart-at) > 1e-6 {
		return nil, fmt.Errorf("accepted motion evidence for the requested window is required")
	}
	out := *input
	out.Windows = append([]VisualWindow(nil), input.Windows...)
	index := -1
	for i, w := range input.Windows {
		if math.Abs(w.Start-at) < 1e-6 {
			if index >= 0 {
				return nil, fmt.Errorf("duplicate visual window")
			}
			index = i
		}
	}
	if index < 0 {
		return nil, fmt.Errorf("visual window not found")
	}
	w := input.Windows[index]
	if !w.Contextual || !validVisualWindow(w, math.Inf(1)) || overlapsRejected(w.Start, w.End, input.Rejected) {
		return nil, fmt.Errorf("window must be contextual, valid, and unrejected")
	}
	motionLag := motion.BaseStart - (motion.DubCenter + float64(motion.OffsetFrames)/24)
	if math.Abs(motionLag-w.Lag) > 1.0/24+1e-6 {
		return nil, fmt.Errorf("motion offset conflicts with visual window")
	}
	out.Windows[index].MotionConfirmed = true
	return &out, nil
}

// CompareVisualMotion suppresses a static background using temporal medians,
// then compares motion in the central 80x80 region. It decodes bounded 24-fps
// excerpts and is diagnostic only; small objects elsewhere may be missed.
func CompareVisualMotion(ctx context.Context, t Tools, base, dub string, baseStart, dubCenter float64) (VisualMotionEvidence, error) {
	var out VisualMotionEvidence
	if !finiteSeam(baseStart) || !finiteSeam(dubCenter) || baseStart < 2 || dubCenter < 3.5 {
		return out, fmt.Errorf("invalid motion query timestamps")
	}
	baseStart, dubCenter = snapVisualRegionTime(baseStart), snapVisualRegionTime(dubCenter)
	bd, err := t.Duration(ctx, base)
	if err != nil {
		return out, err
	}
	dd, err := t.Duration(ctx, dub)
	if err != nil {
		return out, err
	}
	if bd > 7200 || dd > 7200 || baseStart+5 > bd || dubCenter+6.5 > dd {
		return out, fmt.Errorf("motion excerpts exceed source bounds")
	}
	decode := func(path string, start, duration float64) ([]byte, error) {
		return t.run(ctx, t.FFmpeg, nil, "-v", "error", "-nostdin", "-ss", strconv.FormatFloat(start, 'f', 6, 64), "-t", strconv.FormatFloat(duration, 'f', 6, 64), "-threads", "2", "-i", path, "-map", "0:v:0", "-vf", "fps=24,scale=160:120,format=gray", "-an", "-f", "rawvideo", "-")
	}
	a, err := decode(base, baseStart-2, 7)
	if err != nil {
		return out, err
	}
	b, err := decode(dub, dubCenter-3.5, 10)
	if err != nil {
		return out, err
	}
	if len(a)%regionPixels != 0 || len(b)%regionPixels != 0 {
		return out, fmt.Errorf("incomplete decoded motion frame")
	}
	if len(a) >= 168*regionPixels {
		a = a[:168*regionPixels]
	}
	if len(b) >= 240*regionPixels {
		b = b[:240*regionPixels]
	}
	out, err = compareMotionPixels(a, b)
	out.BaseStart, out.DubCenter = baseStart, dubCenter
	for i := range out.FrameWarnings {
		out.FrameWarnings[i].BaseTime = baseStart + float64(out.FrameWarnings[i].Frame)/regionRate
	}
	return out, err
}

func compareMotionPixels(base, dub []byte) (VisualMotionEvidence, error) {
	var out VisualMotionEvidence
	if len(base) != 168*regionPixels || len(dub) != 240*regionPixels {
		return out, fmt.Errorf("expected 168 and 240 complete motion frames")
	}
	const pixels = 80 * 80
	prepare := func(raw []byte, frames int) ([]float32, []float64) {
		med := make([]float32, pixels)
		values := make([]int, frames)
		for y := 0; y < 80; y++ {
			for x := 0; x < 80; x++ {
				p := (y+10)*regionWidth + x + 40
				for f := 0; f < frames; f++ {
					values[f] = int(raw[f*regionPixels+p])
				}
				sort.Ints(values)
				med[y*80+x] = float32(values[(frames-1)/2]+values[frames/2]) / 2
			}
		}
		vectors := make([]float32, frames*pixels)
		norms := make([]float64, frames)
		for f := 0; f < frames; f++ {
			var energy float64
			for y := 0; y < 80; y++ {
				for x := 0; x < 80; x++ {
					k := y*80 + x
					v := float32(raw[f*regionPixels+(y+10)*regionWidth+x+40]) - med[k]
					if v < 10 && v > -10 {
						v = 0
					}
					if v > 80 {
						v = 80
					}
					if v < -80 {
						v = -80
					}
					vectors[f*pixels+k] = v
					energy += float64(v * v)
				}
			}
			norms[f] = math.Sqrt(energy)
			if norms[f] > 0 {
				scale := float32(1 / norms[f])
				for k := 0; k < pixels; k++ {
					vectors[f*pixels+k] *= scale
				}
			}
		}
		return vectors, norms
	}
	a, an := prepare(base, 168)
	b, bn := prepare(dub, 240)
	type candidate struct {
		shift int
		score float64
		valid int
	}
	var candidates []candidate
	for shift := -36; shift <= 36; shift++ {
		var total float64
		valid := 0
		for i := 0; i < 48; i++ {
			ai, bi := 48+i, 84+i+shift
			if an[ai] <= 300 || bn[bi] <= 300 {
				continue
			}
			var dot float32
			for k := 0; k < pixels; k++ {
				dot += a[ai*pixels+k] * b[bi*pixels+k]
			}
			total += float64(dot)
			valid++
		}
		if valid >= 36 {
			candidates = append(candidates, candidate{shift, total / float64(valid), valid})
		}
	}
	if len(candidates) == 0 {
		return out, nil
	}
	sort.Slice(candidates, func(i, j int) bool { return candidates[i].score > candidates[j].score })
	best := candidates[0]
	out.OffsetFrames, out.Score, out.ValidFrames = best.shift, best.score, best.valid
	for i := 0; i < 48; i++ {
		ai, bi := 48+i, 84+i+best.shift
		if an[ai] <= 300 || bn[bi] <= 300 {
			continue
		}
		var dot float32
		for k := 0; k < pixels; k++ {
			dot += a[ai*pixels+k] * b[bi*pixels+k]
		}
		if dot < .25 {
			out.FrameWarnings = append(out.FrameWarnings, MotionFrameWarning{Frame: i, Score: float64(dot)})
		}
	}
	second := math.Inf(-1)
	for _, c := range candidates[1:] {
		if absInt(c.shift-best.shift) > 2 {
			second = c.score
			break
		}
	}
	if math.IsInf(second, -1) {
		return out, nil
	}
	out.RunnerScore = second
	var rev float64
	count := 0
	for i := 0; i < 48; i++ {
		ai, bi := 48+i, 84+(47-i)+best.shift
		if an[ai] <= 300 || bn[bi] <= 300 {
			continue
		}
		var dot float32
		for k := 0; k < pixels; k++ {
			dot += a[ai*pixels+k] * b[bi*pixels+k]
		}
		rev += float64(dot)
		count++
	}
	out.ReverseFrames = count
	if count < 24 {
		return out, nil
	}
	out.ReverseScore = rev / float64(count)
	out.Accepted = absInt(best.shift) <= 32 && best.score >= .55 && best.score-second >= .12 && best.score-out.ReverseScore >= .2 && len(out.FrameWarnings) == 0
	return out, nil
}
