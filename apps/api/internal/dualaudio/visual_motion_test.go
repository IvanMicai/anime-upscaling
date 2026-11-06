package dualaudio

import (
	"context"
	"math/rand"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestVisualMotionDecodesRealFrames(t *testing.T) {
	ffmpeg, err := exec.LookPath("ffmpeg")
	if err != nil {
		t.Skip("ffmpeg unavailable")
	}
	ffprobe, err := exec.LookPath("ffprobe")
	if err != nil {
		t.Skip("ffprobe unavailable")
	}
	path := filepath.Join(t.TempDir(), "motion.mkv")
	cmd := exec.Command(ffmpeg, "-v", "error", "-f", "lavfi", "-i", "testsrc2=size=160x120:rate=24:duration=12", "-c:v", "mpeg4", "-threads", "2", path)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("create video: %v: %s", err, out)
	}
	got, err := CompareVisualMotion(context.Background(), Tools{FFmpeg: ffmpeg, FFprobe: ffprobe}, path, path, 4, 4)
	if err != nil || !got.Accepted || got.OffsetFrames != 0 {
		t.Fatalf("video did not match itself: %+v %v", got, err)
	}
}

func motionTestFrames(frames int, seed int64) []byte {
	r := rand.New(rand.NewSource(seed))
	data := make([]byte, frames*regionPixels)
	for i := range data {
		data[i] = 90
	}
	for f := 0; f < frames; f++ {
		x, y := 40+r.Intn(72), 10+r.Intn(72)
		for dy := 0; dy < 7; dy++ {
			for dx := 0; dx < 7; dx++ {
				data[f*regionPixels+(y+dy)*regionWidth+x+dx] = 240
			}
		}
	}
	return data
}

func TestMotionComparisonFindsSmallMovingObject(t *testing.T) {
	a := motionTestFrames(168, 411)
	b := make([]byte, 240*regionPixels)
	for i := range b {
		b[i] = 90
	}
	for f := 0; f < 168; f++ {
		copy(b[(f+41)*regionPixels:(f+42)*regionPixels], a[f*regionPixels:(f+1)*regionPixels])
	}
	got, err := compareMotionPixels(a, b)
	if err != nil || !got.Accepted || got.OffsetFrames != 5 {
		t.Fatalf("small motion did not match: %+v %v", got, err)
	}
	wrong := motionTestFrames(240, 719)
	got, err = compareMotionPixels(a, wrong)
	if err != nil || got.Accepted {
		t.Fatalf("unrelated motion accepted: %+v %v", got, err)
	}
	// A looping image produces multiple equally plausible time offsets.
	loop := motionTestFrames(8, 808)
	repeatedA := make([]byte, 168*regionPixels)
	repeatedB := make([]byte, 240*regionPixels)
	for f := 0; f < 168; f++ {
		copy(repeatedA[f*regionPixels:(f+1)*regionPixels], loop[(f%8)*regionPixels:(f%8+1)*regionPixels])
	}
	for f := 0; f < 240; f++ {
		copy(repeatedB[f*regionPixels:(f+1)*regionPixels], loop[(f%8)*regionPixels:(f%8+1)*regionPixels])
	}
	got, err = compareMotionPixels(repeatedA, repeatedB)
	if err != nil || got.Accepted {
		t.Fatalf("repeated motion accepted: %+v %v", got, err)
	}
	for i := range b {
		b[i] = 90
	}
	got, err = compareMotionPixels(b[:168*regionPixels], b)
	if err != nil || got.Accepted {
		t.Fatalf("static scene accepted: %+v %v", got, err)
	}
	if _, err = compareMotionPixels(a[:len(a)-1], b); err == nil {
		t.Fatal("incomplete frame accepted")
	}
}
