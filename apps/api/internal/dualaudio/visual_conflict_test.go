package dualaudio

import "testing"

func TestVisualAudioConflictsCountMappedIntersections(t *testing.T) {
	timeline := &VisualTimeline{Windows: []VisualWindow{{Start: 0, End: 2}, {Start: 2, End: 4, Contextual: true}, {Start: 4, End: 6, Lag: .1}}}
	segs := []Segment{{BaseStart: 0, BaseEnd: 3, Lag: .2}, {BaseStart: 3, BaseEnd: 5, Lag: .1}}
	conflicts, seconds := FindVisualAudioConflicts(timeline, segs, .15)
	if seconds != 3 || len(conflicts) != 2 || conflicts[0].Start != 0 || conflicts[1].End != 3 || !conflicts[1].Contextual {
		t.Fatalf("unexpected conflicts: seconds=%f %+v", seconds, conflicts)
	}
	if _, seconds := FindVisualAudioConflicts(nil, segs, .15); seconds != 0 {
		t.Fatal("missing image evidence must not be counted")
	}
}
