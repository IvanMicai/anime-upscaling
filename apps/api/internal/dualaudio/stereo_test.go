package dualaudio

import (
	"math"
	"testing"
)

func TestStereoFeaturesRetainAntiPhaseAudio(t *testing.T) {
	mono := toPCM(music(40, 177))
	base, dub := make([]int16, len(mono)*2), make([]int16, len(mono)*2)
	delay := featRate / 5
	for i, sample := range mono {
		base[2*i], base[2*i+1] = sample, sample
		if i+delay < len(mono) {
			dub[2*(i+delay)], dub[2*(i+delay)+1] = sample, -sample
		}
	}
	baseFeatures, dubFeatures := ExtractStereoFeatures(base), ExtractStereoFeatures(dub)
	p0, length := 5*int(fps), 10*int(fps)
	lag, confidence, ok := matchWindow(baseFeatures, dubFeatures.slice(p0, p0+length), p0, 0, 2*int(fps))
	if !ok || confidence < 6 || math.Abs(lag/fps+.2) > .02 {
		t.Fatalf("stereo lag=%f confidence=%f ok=%v", lag/fps, confidence, ok)
	}
	monoDub := make([]int16, len(mono))
	for i := range monoDub {
		monoDub[i] = int16((int(dub[2*i]) + int(dub[2*i+1])) / 2)
	}
	monoFeatures := ExtractFeatures(monoDub)
	_, monoConfidence, _ := matchWindow(ExtractFeatures(mono), monoFeatures.slice(p0, p0+length), p0, 0, 2*int(fps))
	if monoConfidence >= 6 {
		t.Fatal("cancelled mono track should not be trusted")
	}
	stereo48 := make([]int16, len(dub)*3)
	for i := range mono {
		for j := 0; j < 3; j++ {
			stereo48[(i*3+j)*2], stereo48[(i*3+j)*2+1] = dub[2*i], dub[2*i+1]
		}
	}
	a := &Alignment{BaseDuration: 35, Segments: []Segment{{BaseStart: 1, BaseEnd: 34, Lag: -.2}}}
	rendered := Render(a, stereo48, nil, GapFillSilence)
	v := ValidateStereo(rendered, baseFeatures, a.Segments, 6, 5)
	if v.Eligible == 0 || v.Hits != v.Eligible {
		t.Fatalf("stereo reconstruction did not validate: %+v", v)
	}
	source := ValidateDubSourceStereo(rendered, dubFeatures, a.Segments, 6)
	if source.Checked == 0 || source.Matched != source.Checked || source.Inconclusive != 0 {
		t.Fatalf("stereo dub source did not validate: %+v", source)
	}
}

func TestVisualAudioAlignRejectsMixedFeatureLayouts(t *testing.T) {
	mono := toPCM(music(15, 178))
	stereo := make([]int16, len(mono)*2)
	for i, s := range mono {
		stereo[2*i], stereo[2*i+1] = s, s
	}
	if _, err := VisualAudioAlign(&VisualTimeline{}, ExtractFeatures(mono), ExtractStereoFeatures(stereo), 15, 15, 6); err == nil {
		t.Fatal("mono and stereo feature layouts must not be mixed")
	}
}

func TestStereoHoldoutKeepsTrainingAndEvaluationSeparate(t *testing.T) {
	mono := toPCM(music(180, 179))
	stereo := make([]int16, len(mono)*2)
	for i, s := range mono {
		stereo[2*i], stereo[2*i+1] = s, s
	}
	features := ExtractStereoFeatures(stereo)
	timeline := &VisualTimeline{}
	for s := 2; s < 178; s += 2 {
		timeline.Windows = append(timeline.Windows, VisualWindow{Start: float64(s), End: float64(s + 2)})
	}
	candidate, err := VisualAudioAlignHoldout(timeline, features, features, 180, 180, 6)
	if err != nil {
		t.Fatal(err)
	}
	if len(candidate.TrainingWindows) < 3 || len(candidate.TrainingWindows) != len(candidate.TrainingSourceWindows) {
		t.Fatalf("unexpected training windows: %+v", candidate.TrainingWindows)
	}
	for i, w := range candidate.TrainingWindows {
		if !holdoutTrainingAllowed(w.Start) || w.End-w.Start != 10 || (i > 0 && w.Start < candidate.TrainingWindows[i-1].End) {
			t.Fatalf("unexpected training position: %+v", w)
		}
		for evaluation := 24.5; evaluation+10 <= 180; evaluation += 60 {
			// Both match searches inspect the reference up to five seconds
			// around their windows; even those reference spans must not overlap.
			if w.Start-5 < evaluation+15 && w.End+5 > evaluation-5 {
				t.Fatalf("training and holdout search spans overlap: %+v at %.1f", w, evaluation)
			}
		}
	}
	for _, region := range candidate.Regions {
		for _, observation := range region.Observations {
			found := false
			for _, w := range candidate.TrainingWindows {
				if math.Abs(observation.Time-(w.Start+5)) < .001 {
					found = true
					break
				}
			}
			if !found {
				t.Fatalf("observation was not drawn from a training window: %+v", observation)
			}
		}
	}
	rendered := Render(candidate.Alignment, upsampleStereo(mono), nil, GapFillSilence)
	v := ValidateStereoHoldout(rendered, features, candidate.Alignment.Segments, candidate.TrainingSourceWindows, 6)
	if v.Eligible < 2 || v.Hits != v.Eligible || v.TickSec != 60 {
		t.Fatalf("holdout mismatch: %+v", v)
	}
	// An edit can map a later base window onto source audio used for training.
	if !sourceOverlapTraining(20.5, 30.5, []Segment{{BaseStart: 20, BaseEnd: 31, Lag: 20.5}}, []Gap{{Start: 0, End: 10}}) {
		t.Fatal("reused source audio must be excluded")
	}
	if sourceOverlapTraining(20.5, 30.5, []Segment{{BaseStart: 20, BaseEnd: 31, Lag: 0}}, []Gap{{Start: 0, End: 10}}) {
		t.Fatal("disjoint source audio was excluded")
	}
}
