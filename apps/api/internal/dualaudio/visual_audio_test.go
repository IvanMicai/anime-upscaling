package dualaudio

import (
	"math"
	"testing"
)

func TestVisualAudioAlignUsesConsistentAudioAndPreservesGaps(t *testing.T) {
	features := ExtractFeatures(toPCM(music(90, 33)))
	timeline := &VisualTimeline{}
	for s := 4; s < 80; s += 2 {
		if s >= 40 && s < 46 {
			continue
		}
		timeline.Windows = append(timeline.Windows, VisualWindow{Start: float64(s), End: float64(s + 2), Lag: 0})
	}
	got, err := VisualAudioAlign(timeline, features, features, 90, 90, 6)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Regions) != 1 || !got.Regions[0].Accepted || math.Abs(got.Regions[0].Correction) > .02 {
		t.Fatalf("matching audio should yield zero correction: %+v", got.Regions)
	}
	if len(got.Alignment.Segments) != 2 || got.Alignment.Coverage >= .9 {
		t.Fatalf("unsupported visual gap must remain a gap: %+v", got.Alignment)
	}
}

func TestVisualAudioAlignRejectsOverlappingWindows(t *testing.T) {
	f := ExtractFeatures(toPCM(music(20, 1)))
	timeline := &VisualTimeline{Windows: []VisualWindow{{Start: 2, End: 4}, {Start: 3, End: 5}}}
	if _, err := VisualAudioAlign(timeline, f, f, 20, 20, 6); err == nil {
		t.Fatal("expected invalid timeline error")
	}
}

func TestVisualAudioAlignDoesNotTrustSilentAudio(t *testing.T) {
	f := ExtractFeatures(make([]int16, 40*featRate))
	timeline := &VisualTimeline{}
	for s := 2; s < 38; s += 2 {
		timeline.Windows = append(timeline.Windows, VisualWindow{Start: float64(s), End: float64(s + 2)})
	}
	got, err := VisualAudioAlign(timeline, f, f, 40, 40, 6)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Regions) != 1 || got.Regions[0].Accepted {
		t.Fatal("silent audio cannot confirm a correction")
	}
}

func TestVisualAudioAlignMeasuresAudioShift(t *testing.T) {
	pcm := toPCM(music(90, 43))
	delayed := make([]int16, len(pcm))
	copy(delayed[featRate/4:], pcm)
	base, dub := ExtractFeatures(pcm), ExtractFeatures(delayed)
	timeline := &VisualTimeline{}
	for s := 4; s < 80; s += 2 {
		timeline.Windows = append(timeline.Windows, VisualWindow{Start: float64(s), End: float64(s + 2)})
	}
	got, err := VisualAudioAlign(timeline, base, dub, 90, 90, 6)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Regions) != 1 || !got.Regions[0].Accepted || math.Abs(got.Regions[0].Correction+0.25) > 0.03 {
		t.Fatalf("expected correction near -250 ms, got %+v", got.Regions)
	}
}

func TestConsistentAudioCorrectionRequiresThreeAgreeingWindows(t *testing.T) {
	if _, _, ok := consistentAudioCorrection([]float64{0.1, 0.1, 0.4}); ok {
		t.Fatal("two agreeing windows are insufficient")
	}
	correction, count, ok := consistentAudioCorrection([]float64{0.25, 0.26, 0.27, -0.3})
	if !ok || count != 3 || math.Abs(correction-0.26) > 0.001 {
		t.Fatalf("correction=%f count=%d accepted=%v", correction, count, ok)
	}
}

func TestAudioCorrectionRemainsNearEvidence(t *testing.T) {
	times := []float64{1200, 1210, 1220}
	if nearAudioEvidence(900, times) {
		t.Fatal("extrapolated correction into an unsupported stretch")
	}
	if !nearAudioEvidence(1190, times) {
		t.Fatal("discarded nearby evidence")
	}
}

func TestVisualAudioDoesNotExtrapolateCorrectionAcrossEpisode(t *testing.T) {
	pcm := toPCM(music(110, 51))
	delayed := make([]int16, len(pcm))
	copy(delayed[featRate/4:], pcm)
	base, dub := ExtractFeatures(pcm), ExtractFeatures(delayed)
	timeline := &VisualTimeline{}
	for s := 2; s < 50; s += 6 {
		timeline.Windows = append(timeline.Windows, VisualWindow{Start: float64(s), End: float64(s + 2)})
	}
	for s := 50; s < 100; s += 2 {
		timeline.Windows = append(timeline.Windows, VisualWindow{Start: float64(s), End: float64(s + 2)})
	}
	got, err := VisualAudioAlign(timeline, base, dub, 110, 110, 6)
	if err != nil {
		t.Fatal(err)
	}
	segments := got.Alignment.Segments
	if len(segments) < 2 || math.Abs(segments[0].Lag) > .001 || math.Abs(segments[len(segments)-1].Lag+0.25) > .03 {
		t.Fatalf("expected unsupported early audio and corrected later audio: %+v", segments)
	}
}
