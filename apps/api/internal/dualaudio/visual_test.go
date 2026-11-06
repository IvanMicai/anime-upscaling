package dualaudio

import (
	"math/rand"
	"testing"
)

func TestMatchVisualLocatesSequenceAndRejectsAmbiguity(t *testing.T) {
	rng := rand.New(rand.NewSource(17))
	seq := make([][7]uint64, 80)
	for i := range seq {
		for j := range seq[i] {
			seq[i][j] = rng.Uint64()
		}
	}
	query := append([][7]uint64(nil), seq[30:35]...)
	index, distance, _, ok := matchVisual(query, seq)
	if !ok || index != 30 || distance != 0 {
		t.Fatalf("got index=%d distance=%f matched=%v", index, distance, ok)
	}
	copy(seq[60:65], query)
	_, _, _, ok = matchVisual(query, seq)
	if ok {
		t.Fatal("repeated scene must be marked ambiguous")
	}
}

func TestVisualFingerprintIgnoresBorders(t *testing.T) {
	a, b := make([]byte, visualBytes), make([]byte, visualBytes)
	for i := range b {
		b[i] = 255
	}
	for y := 3; y < 21; y++ {
		for x := 4; x <= 27; x++ {
			a[y*visualWidth+x] = byte(x * 4)
			b[y*visualWidth+x] = byte(x * 4)
		}
	}
	if visualFingerprint(a) != visualFingerprint(b) {
		t.Fatal("borders changed fingerprint")
	}
}

func TestMatchVisualRejectsUnrelatedAndShortInput(t *testing.T) {
	q := make([][7]uint64, 5)
	s := make([][7]uint64, 12)
	for i := range s {
		for j := range s[i] {
			s[i][j] = ^uint64(0)
		}
	}
	if _, _, _, ok := matchVisual(q, s); ok {
		t.Fatal("unrelated sequence matched")
	}
	if _, _, _, ok := matchVisual(q, s[:4]); ok {
		t.Fatal("short search matched")
	}
}

func TestMatchVisualNeedsIndependentAlternative(t *testing.T) {
	q := make([][7]uint64, 5)
	_, _, margin, ok := matchVisual(q, q)
	if ok || margin != 0 {
		t.Fatalf("matched=%v margin=%v", ok, margin)
	}
}
