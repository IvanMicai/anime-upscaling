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

func TestVisualCandidatesRequireMatchingAudio(t *testing.T) {
	rng := rand.New(rand.NewSource(912))
	frames := make([][7]uint64, 100)
	for i := range frames {
		for j := range frames[i] {
			frames[i][j] = rng.Uint64()
		}
	}
	source := ExtractFeatures(toPCM(music(100, 33)))
	got := visualCandidates(frames, frames, source, source, 6)
	if len(got) == 0 {
		t.Fatal("expected an audio-confirmed visual match")
	}
	for _, p := range got {
		if p.lag > 1 || p.lag < -1 {
			t.Fatalf("incorrect offset: %+v", p)
		}
	}
	// Identical video with silent audio cannot establish a precise offset.
	silent := ExtractFeatures(make([]int16, 100*featRate))
	if p := visualCandidates(frames, frames, source, silent, 6); len(p) != 0 {
		t.Fatalf("accepted visual match without audio evidence: %+v", p)
	}
}

func TestPreferVisualResultKeepsAudioEvidence(t *testing.T) {
	old := Validation{Hits: 90, Eligible: 100, Median: 0.01, P95: 0.02, OffSec: 10}
	better := Validation{Hits: 92, Eligible: 100, Median: 0.01, P95: 0.02, OffSec: 0}
	if !preferVisualResult(StatusReview, StatusOK, old, better, 0.95, 0.95) {
		t.Fatal("rejected better validated result")
	}
	worse := better
	worse.Hits = 80
	if preferVisualResult(StatusReview, StatusOK, old, worse, 0.95, 0.95) {
		t.Fatal("accepted fewer validated windows")
	}
	if preferVisualResult(StatusOK, StatusReview, old, better, 0.95, 0.95) {
		t.Fatal("downgraded a passing alignment")
	}
	if preferVisualResult(StatusReview, StatusOK, old, better, 0.95, 0.80) {
		t.Fatal("accepted reduced coverage")
	}
}
