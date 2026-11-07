package dualaudio

import (
	"context"
	"math"
	"math/rand"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestVisualRegionsSearchCenterDoesNotChangeMatchPhase(t *testing.T) {
	ffmpeg, err := exec.LookPath("ffmpeg")
	if err != nil {
		t.Skip("ffmpeg unavailable")
	}
	ffprobe, err := exec.LookPath("ffprobe")
	if err != nil {
		t.Skip("ffprobe unavailable")
	}
	video := filepath.Join(t.TempDir(), "motion.mkv")
	cmd := exec.Command(ffmpeg, "-v", "error", "-f", "lavfi", "-i", "testsrc2=size=160x120:rate=24:duration=10", "-c:v", "mpeg4", "-threads", "2", video)
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("create source: %v: %s", err, output)
	}
	tools := Tools{FFmpeg: ffmpeg, FFprobe: ffprobe}
	var previous float64
	for i, center := range []float64{3.3, 3.083333, 3.01, 3.2, 3.4} {
		result, err := CompareVisualRegions(context.Background(), tools, video, video, 3, center)
		if err != nil {
			t.Fatal(err)
		}
		if !result.Consensus {
			t.Fatalf("no consensus for center %.6f: %+v", center, result)
		}
		matched := result.DubCenter + result.OffsetFrames/regionRate
		if i > 0 && math.Abs(matched-previous) > 1e-6 {
			t.Fatalf("search center changed match: %.6f versus %.6f", previous, matched)
		}
		previous = matched
	}
}

func TestVisualRegionSeekTimesShareFrameGrid(t *testing.T) {
	for _, v := range []float64{109.3, 109.083333, 109.123456} {
		got := snapVisualRegionTime(v)
		if math.Abs(got*regionRate-math.Round(got*regionRate)) > 1e-9 || math.Abs(got-v) > .5/regionRate+1e-9 {
			t.Fatalf("%f rounded to invalid frame time %f", v, got)
		}
	}
}

func TestVisualRegionsRequiresMultipleConsistentPatches(t *testing.T) {
	const frames = 120
	random := rand.New(rand.NewSource(42))
	search := make([]byte, frames*regionPixels)
	for i := range search {
		search[i] = byte(random.Intn(180))
	}
	query := make([]byte, 48*regionPixels)
	for i := 0; i < 48; i++ {
		for p := 0; p < regionPixels; p++ {
			query[i*regionPixels+p] = search[(i+40)*regionPixels+p]/2 + 20
		}
	}
	result, err := compareRegionPixels(query, search, 10, 20)
	if err != nil {
		t.Fatal(err)
	}
	if !result.Consensus || result.Agreeing != 16 || result.OffsetFrames != 4 {
		t.Fatalf("expected 16 votes for four-frame shift: %+v", result)
	}
	if len(result.FrameWarnings) != 0 {
		t.Fatalf("matching sequence produced frame warnings: %+v", result.FrameWarnings)
	}
	// A single unrelated frame is hidden by the sequence average. Report its
	// location without changing the experimental aggregate consensus.
	copy(query[24*regionPixels:25*regionPixels], search[4*regionPixels:5*regionPixels])
	result, err = compareRegionPixels(query, search, 10, 20)
	if err != nil {
		t.Fatal(err)
	}
	if !result.Consensus || len(result.FrameWarnings) != 1 || result.FrameWarnings[0].Frame != 24 {
		t.Fatalf("expected a warning at frame 24 despite consensus: %+v", result)
	}
	// Even a perfect match at the edge is ambiguous: the search did not
	// inspect the next offset, so it cannot rule out a better nearby match.
	for i := 0; i < 48; i++ {
		copy(query[i*regionPixels:(i+1)*regionPixels], search[(i+72)*regionPixels:(i+73)*regionPixels])
	}
	result, err = compareRegionPixels(query, search, 10, 20)
	if err != nil {
		t.Fatal(err)
	}
	if result.Consensus {
		t.Fatalf("a perfect boundary match must remain inconclusive: %+v", result)
	}
	// Restore the interior match for the single-active-patch control.
	for i := 0; i < 48; i++ {
		for p := 0; p < regionPixels; p++ {
			query[i*regionPixels+p] = search[(i+40)*regionPixels+p]/2 + 20
		}
	}
	for f := 0; f < frames; f++ {
		for y := 0; y < regionHeight; y++ {
			for x := 0; x < regionWidth; x++ {
				if x >= 40 || y >= 30 {
					search[f*regionPixels+y*regionWidth+x] = 50
				}
			}
		}
	}
	for f := 0; f < 48; f++ {
		for y := 0; y < regionHeight; y++ {
			for x := 0; x < regionWidth; x++ {
				if x >= 40 || y >= 30 {
					query[f*regionPixels+y*regionWidth+x] = 50
				}
			}
		}
	}
	result, err = compareRegionPixels(query, search, 10, 20)
	if err != nil {
		t.Fatal(err)
	}
	if result.Consensus || result.Agreeing != 1 {
		t.Fatalf("single moving patch must be inconclusive: %+v", result)
	}
}

func TestVisualRegionsRejectsIncompleteFrames(t *testing.T) {
	if _, err := compareRegionPixels(make([]byte, 47*regionPixels), make([]byte, 120*regionPixels), 0, 2); err == nil {
		t.Fatal("expected incomplete query to be rejected")
	}
}

func TestVisualRegionsRejectsPeaksNearSearchBoundary(t *testing.T) {
	for _, offset := range []int{-36, -35, -34, -33, 33, 34, 35, 36} {
		if regionPeakAccepted(offset, .99, .5, .5) {
			t.Errorf("accepted boundary peak at %d frames", offset)
		}
	}
	for _, offset := range []int{-32, 0, 32} {
		if !regionPeakAccepted(offset, .99, .5, .5) {
			t.Errorf("rejected well-supported interior peak at %d frames", offset)
		}
	}
}
