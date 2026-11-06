package dualaudio

import "testing"

func TestCommonCoverageSegmentsOnlyIntersectsMappedTime(t *testing.T) {
	a := &Alignment{BaseDuration: 100, DubDuration: 110, Segments: []Segment{{BaseStart: 0, BaseEnd: 20}, {BaseStart: 30, BaseEnd: 70}}}
	b := &Alignment{BaseDuration: 100, DubDuration: 110, Segments: []Segment{{BaseStart: 10, BaseEnd: 40}, {BaseStart: 50, BaseEnd: 80}}}
	got, err := CommonCoverageSegments(a, b)
	if err != nil {
		t.Fatal(err)
	}
	want := []Gap{{10, 20}, {30, 40}, {50, 70}}
	if len(got) != len(want) {
		t.Fatalf("got %+v", got)
	}
	for i, g := range got {
		if g.BaseStart != want[i].Start || g.BaseEnd != want[i].End {
			t.Fatalf("got %+v", got)
		}
	}
}
func TestCommonCoverageSegmentsRejectsInvalidInput(t *testing.T) {
	a := &Alignment{BaseDuration: 100, DubDuration: 100, Segments: []Segment{{BaseStart: 0, BaseEnd: 20}, {BaseStart: 19, BaseEnd: 30}}}
	b := &Alignment{BaseDuration: 100, DubDuration: 100}
	if _, err := CommonCoverageSegments(a, b); err == nil {
		t.Fatal("overlap accepted")
	}
	a.Segments = nil
	b.DubDuration = 101
	if _, err := CommonCoverageSegments(a, b); err == nil {
		t.Fatal("different files accepted")
	}
}

func TestCommonCoverageRejectsInvalidSourceOrOverlappingGap(t *testing.T) {
	a := &Alignment{BaseDuration: 100, DubDuration: 100, Segments: []Segment{{BaseStart: 0, BaseEnd: 20, Lag: -90}}}
	b := &Alignment{BaseDuration: 100, DubDuration: 100, Segments: []Segment{{BaseStart: 0, BaseEnd: 20}}}
	if _, err := CommonCoverageSegments(a, b); err == nil {
		t.Fatal("out-of-range dub interval accepted")
	}
	a.Segments[0].Lag = 0
	a.Gaps = []Gap{{Start: 10, End: 15}}
	if _, err := CommonCoverageSegments(a, b); err == nil {
		t.Fatal("gap overlapping mapped audio accepted")
	}
}
