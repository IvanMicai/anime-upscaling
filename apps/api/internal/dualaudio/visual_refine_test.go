package dualaudio

import (
	"context"
	"math"
	"math/rand"
	"testing"
)

func TestRefineVisualRejectionsRequiresCandidateDetails(t *testing.T) {
	input := &VisualTimeline{SampleRate: 12, Rejected: []Gap{{Start: 10, End: 12}}}
	if _, err := RefineVisualRejections(context.Background(), Tools{}, "a", "b", input, 1); err == nil {
		t.Fatal("old timeline without candidate details must not be silently accepted")
	}
	if _, err := RefineVisualRejections(context.Background(), Tools{}, "a", "b", nil, 1); err == nil {
		t.Fatal("nil timeline must fail")
	}
}

func TestFineCandidateMustCoverExactlyTwoSeconds(t *testing.T) {
	if !validVisualWindow(VisualWindow{Start: 10, End: 12}, 30) {
		t.Fatal("valid window rejected")
	}
	if validVisualWindow(VisualWindow{Start: 10, End: 25}, 30) {
		t.Fatal("probe must not validate a longer window")
	}
}

func TestRefineVisualSequence24RequiresDistinctPeak(t *testing.T) {
	rng := rand.New(rand.NewSource(88))
	query, search := make([]byte, 72*visualBytes), make([]byte, 336*visualBytes)
	if _, err := rng.Read(query); err != nil {
		t.Fatal(err)
	}
	if _, err := rng.Read(search); err != nil {
		t.Fatal(err)
	}
	copy(search[(132+2)*visualBytes:], query)
	shift, _, _, ok := refineVisualSequence24(query, search)
	if !ok || shift != 2 {
		t.Fatalf("shift=%d matched=%v", shift, ok)
	}
	// One mismatching sampled frame can hide in a high average; its half-second
	// block must still fail so a short cut is not silently bridged.
	if _, err := rng.Read(search[(132+2+68)*visualBytes : (132+2+69)*visualBytes]); err != nil {
		t.Fatal(err)
	}
	if _, _, _, ok := refineVisualSequence24(query, search); ok {
		t.Fatal("local visual mismatch must be inconclusive")
	}
	// A five-frame repeating shot gives equally good peaks five frames apart.
	pattern := make([]byte, 5*visualBytes)
	if _, err := rng.Read(pattern); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 72; i++ {
		copy(query[i*visualBytes:(i+1)*visualBytes], pattern[(i%5)*visualBytes:(i%5+1)*visualBytes])
	}
	for i := -10; i < 90; i++ {
		j := (i%5 + 5) % 5
		copy(search[(132+i)*visualBytes:(133+i)*visualBytes], pattern[j*visualBytes:(j+1)*visualBytes])
	}
	if _, _, _, ok := refineVisualSequence24(query, search); ok {
		t.Fatal("repeated sequence must be inconclusive")
	}
}

func TestRefineVisualSequence24RejectsStaticAndShortInput(t *testing.T) {
	query, search := make([]byte, 72*visualBytes), make([]byte, 336*visualBytes)
	if _, _, _, ok := refineVisualSequence24(query, search); ok {
		t.Fatal("constant sequence accepted")
	}
	if _, _, _, ok := refineVisualSequence24(query[:100], search); ok {
		t.Fatal("short sequence accepted")
	}
}

func TestSelectAcceptedVisualWindows(t *testing.T) {
	tl := &VisualTimeline{SampleRate: 12, Windows: []VisualWindow{{Start: 2, End: 4}, {Start: 4, End: 6}, {Start: 10, End: 12}}}
	ids, err := selectAcceptedVisualWindows(tl, []float64{10, 2})
	if err != nil || len(ids) != 2 || ids[0] != 2 || ids[1] != 0 {
		t.Fatalf("selection: %v %v", ids, err)
	}
	for _, starts := range [][]float64{nil, {2, 2}, {3}, {math.NaN()}, {math.Inf(1)}} {
		if _, err := selectAcceptedVisualWindows(tl, starts); err == nil {
			t.Fatalf("invalid selection accepted: %v", starts)
		}
	}
	if _, err := selectAcceptedVisualWindows(&VisualTimeline{SampleRate: 24}, []float64{2}); err == nil {
		t.Fatal("wrong sample rate accepted")
	}
}
