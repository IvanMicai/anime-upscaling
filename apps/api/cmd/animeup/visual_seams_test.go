package main

import (
	"anime-upscaling/internal/dualaudio"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestVisualSeamsAcceptsAnalyzeResultAndReportsBracket(t *testing.T) {
	dir := t.TempDir()
	timeline := filepath.Join(dir, "timeline.json")
	alignment := filepath.Join(dir, "alignment.json")
	visual := `{"windows":[{"start":14,"end":16,"lag":0},{"start":16,"end":18,"lag":0},{"start":18,"end":20,"lag":0},{"start":20,"end":22,"lag":0},{"start":22,"end":24,"lag":0},{"start":24,"end":26,"lag":0},{"start":28,"end":30,"lag":1},{"start":30,"end":32,"lag":1},{"start":32,"end":34,"lag":1}]}`
	result := `{"alignment":{"segments":[{"base_start":0,"base_end":20,"lag":0},{"base_start":20,"base_end":100,"lag":1}]}}`
	if err := os.WriteFile(timeline, []byte(visual), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(alignment, []byte(result), 0600); err != nil {
		t.Fatal(err)
	}
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	old := os.Stdout
	os.Stdout = w
	defer func() { os.Stdout = old }()
	err = dualaudioVisualSeams([]string{"--timeline", timeline, "--alignment", alignment})
	_ = w.Close()
	if err != nil {
		t.Fatal(err)
	}
	var out struct {
		Boundaries []struct{ Earliest, Latest float64 }
	}
	if err = json.NewDecoder(r).Decode(&out); err != nil {
		t.Fatal(err)
	}
	if len(out.Boundaries) != 1 || out.Boundaries[0].Earliest != 26 || out.Boundaries[0].Latest != 28 {
		t.Fatalf("unexpected response: %+v", out)
	}
}

func TestVisualSeamsEvaluateRejectsInvalidTickBeforeDecoding(t *testing.T) {
	err := dualaudioVisualSeamsEvaluate(context.Background(), dualaudio.Tools{}, []string{"--base", "base", "--dub", "dub", "--timeline", "timeline", "--alignment", "alignment", "--tick", "0"})
	if err == nil {
		t.Fatal("invalid tick must be rejected")
	}
}

func TestVisualSeamsEvaluateWithoutCandidateDoesNotDecode(t *testing.T) {
	dir := t.TempDir()
	timeline := filepath.Join(dir, "timeline.json")
	alignment := filepath.Join(dir, "alignment.json")
	if err := os.WriteFile(timeline, []byte(`{"windows":[]}`), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(alignment, []byte(`{"base_duration":100,"dub_duration":100,"segments":[{"base_start":0,"base_end":100,"lag":0}]}`), 0600); err != nil {
		t.Fatal(err)
	}
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	old := os.Stdout
	os.Stdout = w
	defer func() { os.Stdout = old }()
	err = dualaudioVisualSeamsEvaluate(context.Background(), dualaudio.Tools{}, []string{"--base", "missing-base", "--dub", "missing-dub", "--timeline", timeline, "--alignment", alignment})
	_ = w.Close()
	if err != nil {
		t.Fatal(err)
	}
	var out struct{ Status, Reason string }
	if err = json.NewDecoder(r).Decode(&out); err != nil {
		t.Fatal(err)
	}
	if out.Status != "review" || out.Reason == "" {
		t.Fatalf("unexpected response: %+v", out)
	}
}

func TestVisualSeamsEvaluateRejectsMismatchedDurations(t *testing.T) {
	dir := t.TempDir()
	timeline := filepath.Join(dir, "timeline.json")
	alignment := filepath.Join(dir, "alignment.json")
	probe := filepath.Join(dir, "probe")
	visual := `{"windows":[{"start":14,"end":16,"lag":0},{"start":16,"end":18,"lag":0},{"start":18,"end":20,"lag":0},{"start":20,"end":22,"lag":0},{"start":22,"end":24,"lag":0},{"start":24,"end":26,"lag":0},{"start":28,"end":30,"lag":1},{"start":30,"end":32,"lag":1},{"start":32,"end":34,"lag":1}]}`
	result := `{"base_duration":100,"dub_duration":100,"segments":[{"base_start":0,"base_end":20,"lag":0},{"base_start":20,"base_end":100,"lag":1}]}`
	for path, data := range map[string]string{timeline: visual, alignment: result, probe: "#!/bin/sh\nprintf '999\\n'\n"} {
		if err := os.WriteFile(path, []byte(data), 0700); err != nil {
			t.Fatal(err)
		}
	}
	err := dualaudioVisualSeamsEvaluate(context.Background(), dualaudio.Tools{FFprobe: probe}, []string{"--base", "base", "--dub", "dub", "--timeline", timeline, "--alignment", alignment})
	if err == nil || !strings.Contains(err.Error(), "durations do not match") {
		t.Fatalf("expected duration mismatch, got %v", err)
	}
}
