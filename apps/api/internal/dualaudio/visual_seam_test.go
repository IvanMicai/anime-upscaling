package dualaudio

import (
	"math"
	"testing"
)

func TestFindVisualAudioSeamJumps(t *testing.T) {
	s := []Segment{{BaseStart: 0, BaseEnd: 2, Lag: 0}, {BaseStart: 2, BaseEnd: 4, Lag: .1}, {BaseStart: 5, BaseEnd: 7, Lag: .5}, {BaseStart: 7, BaseEnd: 9, Lag: .52}, {BaseStart: 9, BaseEnd: 11, Lag: math.NaN()}}
	out := FindVisualAudioSeamJumps(s, .04)
	if len(out) != 1 || out[0].BaseTime != 2 || math.Abs(out[0].SourceJumpSec+.1) > 1e-9 {
		t.Fatalf("wrong adjacent seam jumps: %+v", out)
	}
	if len(FindVisualAudioSeamJumps(s, math.NaN())) != 0 {
		t.Fatal("invalid threshold accepted")
	}
}

func TestVisualBoundaryReviewBracketsLateChange(t *testing.T) {
	segments := []Segment{{BaseStart: 610.5, BaseEnd: 793.5, Lag: -11.12}, {BaseStart: 794, BaseEnd: 1218.5, Lag: -10.61}}
	var windows []VisualWindow
	for t := 780.0; t < 830; t += 2 {
		windows = append(windows, VisualWindow{Start: t, End: t + 2, Lag: -11.1666667})
	}
	for t := 834.0; t < 840; t += 2 {
		windows = append(windows, VisualWindow{Start: t, End: t + 2, Lag: -10.6666667})
	}
	timeline := &VisualTimeline{Windows: windows}
	before := append([]Segment(nil), segments...)
	got := FindVisualBoundaryReviews(timeline, segments)
	if len(got) != 1 || got[0].Earliest != 830 || got[0].Latest != 834 {
		t.Fatalf("unexpected bracket: %+v", got)
	}
	if segments[0] != before[0] || segments[1] != before[1] {
		t.Fatal("diagnostic changed alignment")
	}
	timeline.Rejected = []Gap{{Start: 830, End: 834}}
	if got := FindVisualBoundaryReviews(timeline, segments); len(got) != 0 {
		t.Fatalf("rejected gap accepted: %+v", got)
	}
	timeline.Rejected = nil
	for i := range windows {
		if windows[i].Start >= 794 {
			windows[i].Contextual = true
		}
	}
	if got := FindVisualBoundaryReviews(timeline, segments); len(got) != 0 {
		t.Fatalf("inferred windows accepted as independent: %+v", got)
	}
}

func TestVisualBoundaryReviewRejectsUnknownOrAmbiguousIntervals(t *testing.T) {
	segments := []Segment{{BaseStart: 0, BaseEnd: 20, Lag: 0}, {BaseStart: 20, BaseEnd: 100, Lag: 1}}
	windows := []VisualWindow{{Start: 14, End: 16, Lag: 0}, {Start: 16, End: 18, Lag: 0}, {Start: 18, End: 20, Lag: 0}, {Start: 20, End: 22, Lag: 0}, {Start: 22, End: 24, Lag: 0}, {Start: 24, End: 26, Lag: 0}, {Start: 40, End: 42, Lag: 1}, {Start: 42, End: 44, Lag: 1}, {Start: 44, End: 46, Lag: 1}}
	if got := FindVisualBoundaryReviews(&VisualTimeline{Windows: windows}, segments); len(got) != 0 {
		t.Fatalf("large observation gap accepted: %+v", got)
	}
	windows[6].Start, windows[6].End = 28, 30
	windows[7].Start, windows[7].End = 30, 32
	windows[8].Start, windows[8].End = 32, 34
	windows[5].Lag = .5
	if got := FindVisualBoundaryReviews(&VisualTimeline{Windows: windows}, segments); len(got) != 0 {
		t.Fatalf("ambiguous visual offset accepted: %+v", got)
	}
}

func TestVisualBoundaryReviewRejectsDuplicateWindows(t *testing.T) {
	segments := []Segment{{BaseStart: 0, BaseEnd: 20, Lag: 0}, {BaseStart: 20, BaseEnd: 100, Lag: 1}}
	w := []VisualWindow{{Start: 14, End: 16, Lag: 0}, {Start: 14, End: 16, Lag: 0}, {Start: 18, End: 20, Lag: 0}, {Start: 20, End: 22, Lag: 0}, {Start: 22, End: 24, Lag: 0}, {Start: 24, End: 26, Lag: 0}, {Start: 28, End: 30, Lag: 1}, {Start: 30, End: 32, Lag: 1}, {Start: 32, End: 34, Lag: 1}}
	if got := FindVisualBoundaryReviews(&VisualTimeline{Windows: w}, segments); len(got) != 0 {
		t.Fatalf("duplicate observations accepted: %+v", got)
	}
}

func TestProposeVisualBoundaryPreservesGapAndOriginal(t *testing.T) {
	a := &Alignment{BaseDuration: 100, DubDuration: 120, Segments: []Segment{{BaseStart: 0, BaseEnd: 19.5, Lag: -1}, {BaseStart: 20, BaseEnd: 90, Lag: -.5}}, Gaps: []Gap{{Start: 19.5, End: 20}, {Start: 90, End: 100}}}
	r := VisualBoundaryReview{AudioBoundary: 20, Earliest: 30, Latest: 32, PreviousLag: -1, NextLag: -.5}
	b, err := ProposeVisualBoundaryAlignment(a, r)
	if err != nil {
		t.Fatal(err)
	}
	if len(b.Segments) != 3 || b.Segments[1].BaseStart != 20 || b.Segments[1].BaseEnd != 30 || b.Segments[1].Lag != -1 || b.Segments[2].BaseStart != 30 {
		t.Fatalf("unexpected candidate: %+v", b.Segments)
	}
	if len(b.Gaps) != 2 || b.Gaps[0] != a.Gaps[0] || len(a.Segments) != 2 || a.Segments[1].BaseStart != 20 {
		t.Fatal("gap or original changed")
	}
	r.Earliest = 100
	if _, err = ProposeVisualBoundaryAlignment(a, r); err == nil {
		t.Fatal("out-of-range proposal accepted")
	}
	r.Earliest = 30
	r.PreviousLag = 2
	if _, err = ProposeVisualBoundaryAlignment(a, r); err == nil {
		t.Fatal("stale proposal accepted")
	}
}

func TestBetterVisualBoundaryEvidenceRequiresComparableImprovement(t *testing.T) {
	before := Validation{Eligible: 100, Hits: 80, OffSec: 21, Median: .01, P95: .03}
	after := Validation{Eligible: 100, Hits: 83, OffSec: 0, Median: .01, P95: .03}
	if !BetterVisualBoundaryEvidence(before, after) {
		t.Fatal("expected same-window improvement")
	}
	cases := []Validation{after, after, after, after}
	cases[0].Eligible = 99
	cases[1].Hits = 79
	cases[2].OffSec = 21
	cases[3].P95 = .05
	for _, c := range cases {
		if BetterVisualBoundaryEvidence(before, c) {
			t.Fatalf("accepted incomparable or worse result: %+v", c)
		}
	}
}
