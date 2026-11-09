package dualaudio

import (
	"context"
	"math/rand"
	"testing"
)

func TestTrackOverlapFindsShiftedCopyAndRejectsUnrelatedAudio(t *testing.T) {
	const frames = 24000
	a, b := make([]int16, frames*2), make([]int16, frames*2)
	r := rand.New(rand.NewSource(42))
	for i := range a {
		a[i] = int16(r.Intn(20001) - 10000)
		b[i] = int16(r.Intn(20001) - 10000)
	}
	// Place a distinct copied window at a non-grid-aligned sample shift.
	for i := 4320; i < 8320; i++ {
		for ch := 0; ch < 2; ch++ {
			b[2*(i+3)+ch] = a[2*i+ch]
		}
	}
	got, err := ScreenTrackOverlap(context.Background(), a, b)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].LagSamples != 3 || got[0].Start != .54 {
		t.Fatalf("unexpected matches: %+v", got)
	}
}

func TestTrackOverlapRejectsSilenceAndBadInputs(t *testing.T) {
	a := make([]int16, 32000)
	got, err := ScreenTrackOverlap(context.Background(), a, a)
	if err != nil || len(got) != 0 {
		t.Fatalf("silence: %+v, %v", got, err)
	}
	if _, err = ScreenTrackOverlap(context.Background(), a[:3], a); err == nil {
		t.Fatal("odd PCM accepted")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err = ScreenTrackOverlap(ctx, a, a); err == nil {
		t.Fatal("cancellation ignored")
	}
}
