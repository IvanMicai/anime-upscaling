package dualaudio

import (
	"math/rand"
	"testing"
)

func TestSharedAudioWindowsDetectsShiftButNotDifferentSound(t *testing.T) {
	const rate = 8000
	rng := rand.New(rand.NewSource(42))
	a, b := make([]int16, 4*rate), make([]int16, 4*rate)
	for i := range a {
		a[i] = int16(rng.Intn(12000) - 6000)
		b[i] = int16(rng.Intn(12000) - 6000)
	}
	// Only the second second is a copy; the candidate is 3 ms later.
	for i := rate - 48; i < 2*rate+48; i++ {
		b[i+24] = a[i]
	}
	got, err := FindSharedAudioWindows(a, b)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Start != 1 || got[0].LagMS != 3 || got[0].Correlation < .999 {
		t.Fatalf("unexpected matches: %+v", got)
	}
}
func TestSharedAudioWindowsExcludesSilenceAndShortInputs(t *testing.T) {
	if _, err := FindSharedAudioWindows(make([]int16, 100), make([]int16, 100)); err == nil {
		t.Fatal("short input accepted")
	}
	got, err := FindSharedAudioWindows(make([]int16, 24000), make([]int16, 24000))
	if err != nil || len(got) != 0 {
		t.Fatalf("silence: %+v, %v", got, err)
	}
}
