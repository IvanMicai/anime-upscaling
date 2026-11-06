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
