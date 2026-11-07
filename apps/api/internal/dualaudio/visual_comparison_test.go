package dualaudio

import "testing"

func TestCompareAlignmentVisualEvidenceCountsOnlyObservedChanges(t *testing.T) {
	a := &Alignment{BaseDuration: 20, DubDuration: 30, Segments: []Segment{{BaseStart: 0, BaseEnd: 10, Lag: 0}}}
	b := &Alignment{BaseDuration: 20, DubDuration: 30, Segments: []Segment{{BaseStart: 0, BaseEnd: 10, Lag: -.5}}}
	v := &VisualTimeline{Windows: []VisualWindow{{Start: 0, End: 2, Lag: 0}, {Start: 2, End: 4, Lag: 0}, {Start: 4, End: 6, Lag: 0}, {Start: 6, End: 8, Lag: 0}, {Start: 8, End: 10, Lag: 0}}}
	got, err := CompareAlignmentVisualEvidence(v, a, b, .15)
	if err != nil {
		t.Fatal(err)
	}
	if got.ChangedSeconds != 10 || got.ObservedSeconds != 10 || got.FirstConflictSeconds != 0 || got.SecondConflictSeconds != 10 || !got.SufficientCoverage || got.VisualMetricNotWorse {
		t.Fatalf("unexpected comparison: %+v", got)
	}
	v.Windows[1].Contextual = true
	got, err = CompareAlignmentVisualEvidence(v, a, b, .15)
	if err != nil {
		t.Fatal(err)
	}
	if got.ObservedSeconds != 8 || got.SufficientCoverage || got.VisualMetricNotWorse {
		t.Fatalf("unobserved area treated as agreement: %+v", got)
	}
	v.Windows[1].MotionConfirmed = true
	got, err = CompareAlignmentVisualEvidence(v, a, b, .15)
	if err != nil || got.ObservedSeconds != 10 || !got.SufficientCoverage {
		t.Fatalf("motion-confirmed window omitted: %+v %v", got, err)
	}
}
func TestCompareAlignmentVisualEvidenceRejectsDuplicateWindows(t *testing.T) {
	a := &Alignment{BaseDuration: 10, DubDuration: 10, Segments: []Segment{{BaseStart: 0, BaseEnd: 5}}}
	v := &VisualTimeline{Windows: []VisualWindow{{Start: 0, End: 2}, {Start: 0, End: 2}}}
	if _, err := CompareAlignmentVisualEvidence(v, a, a, .15); err == nil {
		t.Fatal("duplicate visual evidence accepted")
	}
}
