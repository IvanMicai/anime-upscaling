package dualaudio

import (
	"math"
	"testing"
)

func TestProposeVisualAudioRepairsSplitsWithoutMutatingInput(t *testing.T) {
	a := &Alignment{BaseDuration: 100, DubDuration: 120, Coverage: 1, Segments: []Segment{{BaseStart: 0, BaseEnd: 100, Lag: -10}}}
	run := OffRun{Start: 30, End: 50, Lag: -.28, Windows: 3}
	got, applied := ProposeVisualAudioRepairs(a, []OffRun{run})
	if len(applied) != 1 || len(got.Segments) != 3 || math.Abs(got.Segments[1].Lag+10.28) > .0001 || got.Segments[1].BaseStart != 30 || got.Segments[1].BaseEnd != 50 {
		t.Fatalf("bad repair: %+v", got.Segments)
	}
	if len(a.Segments) != 1 || a.Segments[0].Lag != -10 || got.Coverage != a.Coverage {
		t.Fatal("input or coverage changed")
	}
}

func TestProposeVisualAudioRepairsRefusesGapsAndLargeMoves(t *testing.T) {
	a := &Alignment{BaseDuration: 100, DubDuration: 120, Segments: []Segment{{BaseStart: 0, BaseEnd: 40, Lag: -10}, {BaseStart: 42, BaseEnd: 100, Lag: -10}}}
	runs := []OffRun{{Start: 30, End: 50, Lag: -.28, Windows: 3}, {Start: 60, End: 80, Lag: 1, Windows: 3}}
	got, applied := ProposeVisualAudioRepairs(a, runs)
	if len(applied) != 0 || len(got.Segments) != 2 {
		t.Fatal("gap or excessive correction accepted")
	}
}

func TestShouldKeepVisualAudioRepairRejectsNewVisualConflict(t *testing.T) {
	old := Validation{Hits: 190, Eligible: 222, Median: .002, P95: .06, OffSec: 20}
	next := Validation{Hits: 194, Eligible: 222, Median: .002, P95: .053, OffSec: 0}
	if ShouldKeepVisualAudioRepair(old, next, 86, 106) {
		t.Fatal("audio gain must not add conflicting visual intervals")
	}
	if !ShouldKeepVisualAudioRepair(old, next, 86, 86) {
		t.Fatal("non-regressing repair was rejected")
	}
	next.Eligible--
	if ShouldKeepVisualAudioRepair(old, next, 86, 86) {
		t.Fatal("changed evaluation coverage accepted")
	}
}
