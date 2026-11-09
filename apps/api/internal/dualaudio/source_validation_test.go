package dualaudio

import "testing"

func TestValidateDubSourceDetectsIncorrectRendering(t *testing.T) {
	pcm := toPCM(music(70, 44))
	dub := ExtractFeatures(pcm)
	correct := &Alignment{BaseDuration: 60, Segments: []Segment{{BaseStart: 1, BaseEnd: 59, Lag: 1}}}
	rendered := Render(correct, upsampleStereo(pcm), nil, GapFillSilence)
	v := ValidateDubSource(rendered, dub, correct.Segments, 6)
	if v.Checked < 4 || v.Matched != v.Checked || v.Inconclusive != 0 {
		t.Fatalf("correct rendering not verified: %+v", v)
	}
	if v.MappedSec != 58 || v.ExaminedSec != float64(v.Checked*10) || v.ExaminedSec >= v.MappedSec {
		t.Fatalf("checked duration must exclude joins: %+v", v)
	}
	incorrect := []Segment{{BaseStart: 1, BaseEnd: 59, Lag: .75}}
	v = ValidateDubSource(rendered, dub, incorrect, 6)
	if len(v.Misses) == 0 || v.Matched != 0 {
		t.Fatalf("wrong map was not rejected: %+v", v)
	}
}

func TestValidateDubSourceDoesNotVerifySilence(t *testing.T) {
	pcm := make([]int16, 40*featRate)
	segments := []Segment{{BaseStart: 1, BaseEnd: 35}}
	v := ValidateDubSource(upsampleStereo(pcm), ExtractFeatures(pcm), segments, 6)
	if v.Checked == 0 || v.Inconclusive != v.Checked || v.Matched != 0 {
		t.Fatalf("silence must be inconclusive: %+v", v)
	}
}
