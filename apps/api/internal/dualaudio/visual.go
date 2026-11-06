package dualaudio

import (
	"context"
	"fmt"
	"math"
	"math/bits"
	"strconv"
)

// VisualLocation is a coarse scene correspondence, not a verdict on audio sync.
type VisualLocation struct {
	TimeA    float64 `json:"t_a"`
	TimeB    float64 `json:"t_b"`
	Distance float64 `json:"distance"`
	Margin   float64 `json:"margin"`
	Matched  bool    `json:"matched"`
}

const visualWidth = 32
const visualHeight = 24
const visualBytes = visualWidth * visualHeight

// visualFingerprint compares adjacent pixels in the central image area. Cropping
// reduces the influence of borders, logos and subtitles. It does not make a
// frame unique: temporal sequences and an ambiguity check remain essential.
func visualFingerprint(frame []byte) [7]uint64 {
	var h [7]uint64
	n := 0
	for y := 3; y < 21; y++ {
		for x := 4; x < 27; x++ {
			if frame[y*visualWidth+x] > frame[y*visualWidth+x+1] {
				h[n/64] |= uint64(1) << (n % 64)
			}
			n++
		}
	}
	return h
}

func visualDistance(a, b [7]uint64) int {
	n := 0
	for i := range a {
		n += bits.OnesCount64(a[i] ^ b[i])
	}
	return n
}

// matchVisual finds a five-second sequence and compares its score against the
// best non-adjacent alternative. A repeated title or static shot is ambiguous.
func matchVisual(query, search [][7]uint64) (int, float64, float64, bool) {
	if len(query) != 5 || len(search) < len(query) {
		return 0, 0, 0, false
	}
	scores := make([]float64, len(search)-len(query)+1)
	best, bestScore := 0, math.Inf(1)
	for i := range scores {
		for j := range query {
			scores[i] += float64(visualDistance(query[j], search[i+j]))
		}
		scores[i] /= float64(len(query) * 414)
		if scores[i] < bestScore {
			best, bestScore = i, scores[i]
		}
	}
	second := math.Inf(1)
	for i, s := range scores {
		if absInt(i-best) > 3 && s < second {
			second = s
		}
	}
	margin := second - bestScore
	if math.IsInf(second, 1) {
		return best, bestScore, 0, false
	}
	// These conservative heuristics are for diagnostics only; do not use the
	// result to bypass the audio gate or to assert subsecond synchronization.
	return best, bestScore, margin, bestScore < 0.28 && margin >= 0.025 && !math.IsInf(second, 1)
}
func absInt(x int) int {
	if x < 0 {
		return -x
	}
	return x
}

func (t Tools) visualFrames(ctx context.Context, path string, start, dur float64) ([][7]uint64, error) {
	raw, err := t.run(ctx, t.FFmpeg, nil, "-v", "error", "-nostdin", "-ss", strconv.FormatFloat(start, 'f', 3, 64), "-t", strconv.FormatFloat(dur, 'f', 3, 64), "-i", path, "-map", "0:v:0", "-vf", "fps=1,scale=32:24,format=gray", "-an", "-f", "rawvideo", "-")
	if err != nil {
		return nil, err
	}
	if len(raw)%visualBytes != 0 {
		return nil, fmt.Errorf("incomplete decoded video frame")
	}
	out := make([][7]uint64, 0, len(raw)/visualBytes)
	for i := 0; i < len(raw); i += visualBytes {
		out = append(out, visualFingerprint(raw[i:i+visualBytes]))
	}
	return out, nil
}

// LocateVisual searches around an approximate timestamp. It deliberately does
// not change any alignment, output or gate. The result has one-second sampling.
func LocateVisual(ctx context.Context, t Tools, pathA, pathB string, at, radius float64) (VisualLocation, error) {
	result := VisualLocation{TimeA: at}
	da, err := t.Duration(ctx, pathA)
	if err != nil {
		return result, err
	}
	db, err := t.Duration(ctx, pathB)
	if err != nil {
		return result, err
	}
	if math.IsNaN(at) || math.IsInf(at, 0) || at < 2 || at+3 > da {
		return result, fmt.Errorf("timestamp must allow five seconds inside the first video")
	}
	if math.IsNaN(radius) || math.IsInf(radius, 0) || radius < 5 || radius > 240 {
		return result, fmt.Errorf("search radius must be between 5 and 240 seconds")
	}
	qStart := math.Floor(at) - 2
	start := math.Max(0, qStart-radius)
	end := math.Min(db, qStart+radius+5)
	query, err := t.visualFrames(ctx, pathA, qStart, 5)
	if err != nil {
		return result, err
	}
	search, err := t.visualFrames(ctx, pathB, start, end-start)
	if err != nil {
		return result, err
	}
	idx, score, margin, ok := matchVisual(query, search)
	result.TimeB = start + float64(idx) + 2 + (at - math.Floor(at))
	result.Distance = score
	result.Margin = margin
	result.Matched = ok
	if !ok {
		result.TimeB = 0
	}
	return result, nil
}
