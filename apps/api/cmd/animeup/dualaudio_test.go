package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"anime-upscaling/internal/dualaudio"
)

func TestMergeReportKeepsEarlierEpisodes(t *testing.T) {
	path := filepath.Join(t.TempDir(), "report.json")
	first := []*dualaudio.Result{
		{Base: "e01.mkv", Status: dualaudio.StatusOK},
		{Base: "e02.mkv", Status: dualaudio.StatusReview},
	}
	if err := mergeReport(path, first); err != nil {
		t.Fatal(err)
	}
	// A partial re-run: one episode rebuilt, one skipped (nil).
	if err := mergeReport(path, []*dualaudio.Result{{Base: "e02.mkv", Status: dualaudio.StatusOK}, nil}); err != nil {
		t.Fatal(err)
	}

	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var got []*dualaudio.Result
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("report has %d entries, want 2 — a partial run erased the rest", len(got))
	}
	if got[0].Base != "e01.mkv" || got[1].Status != dualaudio.StatusOK {
		t.Errorf("unexpected merge result: %+v %+v", got[0], got[1])
	}
}
