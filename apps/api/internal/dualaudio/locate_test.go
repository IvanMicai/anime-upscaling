package dualaudio

import (
	"context"
	"encoding/binary"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func writeWAV(t *testing.T, path string, samples []float64) {
	t.Helper()
	pcm := toPCM(samples)
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	n := uint32(len(pcm) * 2)
	hdr := make([]byte, 44)
	copy(hdr[0:], "RIFF")
	binary.LittleEndian.PutUint32(hdr[4:], 36+n)
	copy(hdr[8:], "WAVEfmt ")
	binary.LittleEndian.PutUint32(hdr[16:], 16)
	binary.LittleEndian.PutUint16(hdr[20:], 1)
	binary.LittleEndian.PutUint16(hdr[22:], 1)
	binary.LittleEndian.PutUint32(hdr[24:], featRate)
	binary.LittleEndian.PutUint32(hdr[28:], featRate*2)
	binary.LittleEndian.PutUint16(hdr[32:], 2)
	binary.LittleEndian.PutUint16(hdr[34:], 16)
	copy(hdr[36:], "data")
	binary.LittleEndian.PutUint32(hdr[40:], n)
	if _, err := f.Write(hdr); err != nil {
		t.Fatal(err)
	}
	if err := binary.Write(f, binary.LittleEndian, pcm); err != nil {
		t.Fatal(err)
	}
}

func ffmpegTools(t *testing.T) Tools {
	t.Helper()
	for _, bin := range []string{"ffmpeg", "ffprobe"} {
		if _, err := exec.LookPath(bin); err != nil {
			t.Skipf("%s not installed", bin)
		}
	}
	return Tools{FFmpeg: "ffmpeg", FFprobe: "ffprobe"}
}

// The same scene is not at the same timestamp in two releases. B here is A with
// its first 7.3 s missing and a different voice on top.
func TestLocateFindsTheSameMomentInTheOtherFile(t *testing.T) {
	tools := ffmpegTools(t)
	dir := t.TempDir()
	m := music(150, 51)
	a, b := filepath.Join(dir, "a.wav"), filepath.Join(dir, "b.wav")
	writeWAV(t, a, mix(m, voice(len(m), 52)))
	cut := m[int(7.3*featRate):]
	writeWAV(t, b, mix(cut, voice(len(cut), 53)))

	for _, tA := range []float64{20, 75, 140} {
		loc, err := Locate(context.Background(), tools, a, b, tA)
		if err != nil {
			t.Fatal(err)
		}
		if !loc.Matched || math.Abs(loc.TimeB-(tA-7.3)) > 0.05 {
			t.Errorf("t=%.0f: located at %.2f (matched=%v, conf %.1f), want %.2f", tA, loc.TimeB, loc.Matched, loc.Confidence, tA-7.3)
		}
	}
}

// When the other file does not hold that audio, say so instead of pointing at a
// confident-looking wrong frame.
func TestLocateAdmitsWhenItCannotMatch(t *testing.T) {
	tools := ffmpegTools(t)
	dir := t.TempDir()
	a, b := filepath.Join(dir, "a.wav"), filepath.Join(dir, "b.wav")
	writeWAV(t, a, music(100, 61))
	writeWAV(t, b, music(100, 62)) // unrelated content
	loc, err := Locate(context.Background(), tools, a, b, 50)
	if err != nil {
		t.Fatal(err)
	}
	if loc.Matched {
		t.Errorf("unrelated audio reported as matched: %+v", loc)
	}
	if loc.TimeB != 50 {
		t.Errorf("fallback time = %.1f, want the same timestamp (50)", loc.TimeB)
	}
}

func TestLocateRejectsTimeOutsideTheFile(t *testing.T) {
	tools := ffmpegTools(t)
	a := filepath.Join(t.TempDir(), "a.wav")
	writeWAV(t, a, music(30, 71))
	if _, err := Locate(context.Background(), tools, a, a, 500); err == nil {
		t.Error("a time past the end of the file was accepted")
	}
}
