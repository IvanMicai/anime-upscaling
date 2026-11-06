package dualaudio

import (
	"math/rand"
	"testing"
)

func TestVisualRegionsRequiresMultipleConsistentPatches(t *testing.T) {
	const frames = 120
	random := rand.New(rand.NewSource(42))
	search := make([]byte, frames*regionPixels)
	for i := range search {
		search[i] = byte(random.Intn(180))
	}
	query := make([]byte, 48*regionPixels)
	for i := 0; i < 48; i++ {
		for p := 0; p < regionPixels; p++ {
			query[i*regionPixels+p] = search[(i+40)*regionPixels+p]/2 + 20
		}
	}
	result, err := compareRegionPixels(query, search, 10, 20)
	if err != nil {
		t.Fatal(err)
	}
	if !result.Consensus || result.Agreeing != 16 || result.OffsetFrames != 4 {
		t.Fatalf("expected 16 votes for four-frame shift: %+v", result)
	}
	for f := 0; f < frames; f++ {
		for y := 0; y < regionHeight; y++ {
			for x := 0; x < regionWidth; x++ {
				if x >= 40 || y >= 30 {
					search[f*regionPixels+y*regionWidth+x] = 50
				}
			}
		}
	}
	for f := 0; f < 48; f++ {
		for y := 0; y < regionHeight; y++ {
			for x := 0; x < regionWidth; x++ {
				if x >= 40 || y >= 30 {
					query[f*regionPixels+y*regionWidth+x] = 50
				}
			}
		}
	}
	result, err = compareRegionPixels(query, search, 10, 20)
	if err != nil {
		t.Fatal(err)
	}
	if result.Consensus || result.Agreeing != 1 {
		t.Fatalf("single moving patch must be inconclusive: %+v", result)
	}
}

func TestVisualRegionsRejectsIncompleteFrames(t *testing.T) {
	if _, err := compareRegionPixels(make([]byte, 47*regionPixels), make([]byte, 120*regionPixels), 0, 2); err == nil {
		t.Fatal("expected incomplete query to be rejected")
	}
}
