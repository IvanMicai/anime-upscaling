package dualaudio

import (
	"math"
	"math/rand"
	"testing"
)

func TestRecoverVisualGapsMatchesSequenceWithoutBridgingRejected(t *testing.T) {
	rng := rand.New(rand.NewSource(229))
	base, dub := make([]byte, 40*12*visualBytes), make([]byte, 40*12*visualBytes)
	if _, err := rng.Read(base); err != nil {
		t.Fatal(err)
	}
	if _, err := rng.Read(dub); err != nil {
		t.Fatal(err)
	}
	// The dub is three 12-fps frames later than the base.
	copy(dub[3*visualBytes:], base[:len(base)-3*visualBytes])
	timeline := &VisualTimeline{SampleRate: 12, Windows: []VisualWindow{{Start: 12, End: 14, Lag: -.25}, {Start: 20, End: 22, Lag: -.25}}, Rejected: []Gap{{Start: 16, End: 18}}}
	result, err := recoverVisualGaps(timeline, base, dub, 40, 100)
	if err != nil {
		t.Fatal(err)
	}
	var recovered []VisualWindow
	for _, w := range result.Timeline.Windows {
		if w.GapRecovered && w.Start >= 14 && w.Start < 20 {
			recovered = append(recovered, w)
		}
	}
	if len(recovered) != 2 || recovered[0].Start != 14 || recovered[1].Start != 18 || math.Abs(recovered[0].Lag+.25) > .001 {
		t.Fatalf("unexpected recovered windows: %+v", recovered)
	}
	if len(timeline.Windows) != 2 {
		t.Fatal("input timeline was mutated")
	}
}

func TestRecoverVisualGapsRejectsStaticSequence(t *testing.T) {
	frames := make([]byte, 40*12*visualBytes)
	timeline := &VisualTimeline{SampleRate: 12, Windows: []VisualWindow{{Start: 12, End: 14}, {Start: 20, End: 22}}}
	result, err := recoverVisualGaps(timeline, frames, frames, 40, 100)
	if err != nil {
		t.Fatal(err)
	}
	if result.Recovered != 0 {
		t.Fatal("static frames cannot establish a timing match")
	}
}

func TestRecoverVisualGapsRejectsRepeatedDistantScene(t *testing.T) {
	rng := rand.New(rand.NewSource(230))
	base, dub := make([]byte, 40*12*visualBytes), make([]byte, 40*12*visualBytes)
	if _, err := rng.Read(base); err != nil {
		t.Fatal(err)
	}
	if _, err := rng.Read(dub); err != nil {
		t.Fatal(err)
	}
	copy(dub[3*visualBytes:], base[:len(base)-3*visualBytes])
	queryStart := 14*12 - 6
	matchStart := queryStart + 3
	copy(dub[(matchStart+60)*visualBytes:(matchStart+60+36)*visualBytes], base[queryStart*visualBytes:(queryStart+36)*visualBytes])
	timeline := &VisualTimeline{SampleRate: 12, Windows: []VisualWindow{{Start: 12, End: 14, Lag: -.25}, {Start: 20, End: 22, Lag: -.25}}}
	result, err := recoverVisualGaps(timeline, base, dub, 40, 100)
	if err != nil {
		t.Fatal(err)
	}
	for _, w := range result.Timeline.Windows {
		if w.Start == 14 && w.GapRecovered {
			t.Fatal("repeated distant scene accepted")
		}
	}
}

func TestGapSequenceHashScoreDistinguishesSequences(t *testing.T) {
	rng := rand.New(rand.NewSource(231))
	base, dub := make([]byte, 200*visualBytes), make([]byte, 200*visualBytes)
	if _, err := rng.Read(base); err != nil {
		t.Fatal(err)
	}
	if _, err := rng.Read(dub); err != nil {
		t.Fatal(err)
	}
	copy(dub[80*visualBytes:116*visualBytes], base[40*visualBytes:76*visualBytes])
	good, ok := gapSequenceHashScore(base, dub, 40, 80)
	if !ok || good != 0 {
		t.Fatalf("identical sequence distance %f", good)
	}
	bad, ok := gapSequenceHashScore(base, dub, 40, 5)
	if !ok || bad < .35 {
		t.Fatalf("unrelated sequence distance %f", bad)
	}
	if _, ok := gapSequenceHashScore(base, dub, 190, 0); ok {
		t.Fatal("short sequence accepted")
	}
}

func TestRecoverVisualGapsDoesNotBootstrapDistantWindows(t *testing.T) {
	rng := rand.New(rand.NewSource(232))
	base, dub := make([]byte, 150*12*visualBytes), make([]byte, 150*12*visualBytes)
	if _, err := rng.Read(base); err != nil {
		t.Fatal(err)
	}
	if _, err := rng.Read(dub); err != nil {
		t.Fatal(err)
	}
	copy(dub[3*visualBytes:], base[:len(base)-3*visualBytes])
	timeline := &VisualTimeline{SampleRate: 12, Windows: []VisualWindow{{Start: 12, End: 14, Lag: -.25}}}
	result, err := recoverVisualGaps(timeline, base, dub, 150, 1000)
	if err != nil {
		t.Fatal(err)
	}
	for _, w := range result.Timeline.Windows {
		if w.GapRecovered && w.Start > 102 {
			t.Fatalf("recovered window %.0f exceeded the original anchor range", w.Start)
		}
	}
}

func TestRecoverVisualGapsUsesStrictShortWindowAtEdit(t *testing.T) {
	rng := rand.New(rand.NewSource(701))
	base, dub := make([]byte, 40*12*visualBytes), make([]byte, 40*12*visualBytes)
	if _, err := rng.Read(base); err != nil {
		t.Fatal(err)
	}
	if _, err := rng.Read(dub); err != nil {
		t.Fatal(err)
	}
	// Exactly the target two seconds correspond; the surrounding edit does not.
	copy(dub[(14*12+3)*visualBytes:(16*12+3)*visualBytes], base[14*12*visualBytes:16*12*visualBytes])
	timeline := &VisualTimeline{SampleRate: 12, Windows: []VisualWindow{{Start: 12, End: 14, Lag: -.25}}}
	result, err := recoverVisualGaps(timeline, base, dub, 40, 100)
	if err != nil {
		t.Fatal(err)
	}
	for _, w := range result.Timeline.Windows {
		if w.Start == 14 && w.GapRecovered && math.Abs(w.Lag+.25) < .001 {
			return
		}
	}
	t.Fatal("short matching window was not recovered")
}

func TestShortVisualGapRejectsRepeatedScene(t *testing.T) {
	rng := rand.New(rand.NewSource(702))
	base, dub := make([]byte, 40*12*visualBytes), make([]byte, 40*12*visualBytes)
	if _, err := rng.Read(base); err != nil {
		t.Fatal(err)
	}
	if _, err := rng.Read(dub); err != nil {
		t.Fatal(err)
	}
	query, target := 14*12, 14*12+3
	copy(dub[target*visualBytes:(target+24)*visualBytes], base[query*visualBytes:(query+24)*visualBytes])
	copy(dub[(target+60)*visualBytes:(target+84)*visualBytes], base[query*visualBytes:(query+24)*visualBytes])
	if _, ok := shortVisualGap(base, dub, 14, target); ok {
		t.Fatal("repeated scene accepted as a unique match")
	}
}
