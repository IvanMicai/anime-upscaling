package server

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"anime-upscaling/internal/config"
	"anime-upscaling/internal/dualaudio"
)

func mergeTestConfig(t *testing.T, inputFiles ...string) config.Config {
	t.Helper()
	base := t.TempDir()
	cfg := config.Config{BaseDir: base, InputDir: base + "/input", MergedDir: base + "/merged",
		OutputDir: base + "/output", OptimizedDir: base + "/optimized", InterpolatedDir: base + "/interpolated",
		VideoExts: []string{".mkv", ".mp4"}}
	for _, f := range inputFiles {
		p := filepath.Join(cfg.InputDir, filepath.FromSlash(f))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return cfg
}

func preview(t *testing.T, cfg config.Config, body any) (int, map[string]json.RawMessage) {
	t.Helper()
	raw, _ := json.Marshal(body)
	rec := httptest.NewRecorder()
	handleMergePreview(cfg)(rec, httptest.NewRequest(http.MethodPost, "/api/merge/preview", bytes.NewReader(raw)))
	var out map[string]json.RawMessage
	_ = json.Unmarshal(rec.Body.Bytes(), &out)
	return rec.Code, out
}

func TestMergePreviewPairsTwoFolders(t *testing.T) {
	cfg := mergeTestConfig(t,
		"Show/EN/ep01.en.mkv", "Show/EN/ep02.en.mkv", "Show/EN/extra.en.mkv",
		"Show/PT/ep01.pt-br.mp4", "Show/PT/ep02.pt-br.mp4",
		"Other/ep01.en.mkv") // outside the selected folders: must not be seen
	code, out := preview(t, cfg, map[string]any{"paths": []string{"Show/EN", "Show/PT"}})
	if code != http.StatusOK {
		t.Fatalf("status %d: %s", code, out)
	}
	var pairs []dualaudio.NamePair
	var unpaired []dualaudio.Unpaired
	_ = json.Unmarshal(out["pairs"], &pairs)
	_ = json.Unmarshal(out["unpaired"], &unpaired)
	if len(pairs) != 2 || pairs[0].Output != "Show/ep01.mkv" || pairs[1].Output != "Show/ep02.mkv" {
		t.Errorf("pairs = %+v", pairs)
	}
	if len(unpaired) != 1 || unpaired[0].File != "Show/EN/extra.en.mkv" {
		t.Errorf("unpaired = %+v, want only the orphan inside the selection", unpaired)
	}
}

func TestMergePreviewFlagsExistingOutput(t *testing.T) {
	cfg := mergeTestConfig(t, "a.en.mkv", "a.pt-br.mkv")
	if err := os.MkdirAll(cfg.MergedDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(cfg.MergedDir, "a.mkv"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, out := preview(t, cfg, map[string]any{})
	var pairs []dualaudio.NamePair
	_ = json.Unmarshal(out["pairs"], &pairs)
	if len(pairs) != 1 || !pairs[0].Exists {
		t.Errorf("pairs = %+v, want one pair flagged as already merged", pairs)
	}
}

func TestMergePreviewRejectsEscapingPaths(t *testing.T) {
	cfg := mergeTestConfig(t, "a.en.mkv")
	for _, body := range []map[string]any{
		{"paths": []string{"../etc"}},
		{"files": []string{"../../secret.mkv"}},
		{"source": "merged"},
		{"source": "nope"},
	} {
		if code, _ := preview(t, cfg, body); code != http.StatusBadRequest {
			t.Errorf("%v: status %d, want 400", body, code)
		}
	}
}

func TestNormaliseMerge(t *testing.T) {
	m := mergeRequest{}
	if err := normaliseMerge(&m); err != nil {
		t.Fatal(err)
	}
	if m.MergeVideo != "auto" || m.MergeGapFill != "base" || m.MergeTick != dualaudio.DefaultTickSec {
		t.Errorf("defaults = %+v", m)
	}
	for _, bad := range []mergeRequest{{MergeGapFill: "loud"}, {MergeTick: 0.2}, {MergeTick: 300}, {MergeTick: -1}} {
		if err := normaliseMerge(&bad); err == nil {
			t.Errorf("%+v accepted", bad)
		}
	}
}

func TestCheckLanguageChoice(t *testing.T) {
	pairs, _ := dualaudio.PairByName([]string{"a.en.mkv", "a.pt-br.mkv"})
	if err := checkLanguageChoice("merge_video", "en", pairs); err != nil {
		t.Errorf("en rejected: %v", err)
	}
	if err := checkLanguageChoice("merge_video", "auto", pairs); err != nil {
		t.Errorf("auto rejected: %v", err)
	}
	if err := checkLanguageChoice("merge_video", "ja", pairs); err == nil {
		t.Error("a language no pair has was accepted")
	}
}

// The job detail endpoint builds its response field by field, so a new job
// field is dropped unless it is added there too — which is how `merge` went
// missing from GET /api/jobs/{id} while the list endpoint had it.
func TestJobDetailCarriesMergeBlock(t *testing.T) {
	jm := &JobManager{jobs: map[string]*Job{"j1": {ID: "j1", Type: "merge", Status: "completed",
		Files: []string{"a.mkv"}, Merge: &MergeParams{Video: "en", TickSec: 2, GapFill: "base"}}}}
	rec := httptest.NewRecorder()
	handleJobRoutes(jm)(rec, httptest.NewRequest(http.MethodGet, "/api/jobs/j1", nil))
	var out struct {
		Merge *MergeParams `json:"merge"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	if out.Merge == nil || out.Merge.Video != "en" || out.Merge.TickSec != 2 {
		t.Errorf("job detail = %s, want the merge block", rec.Body.String())
	}
}

func TestMergeFrameAndLocateValidateInput(t *testing.T) {
	cfg := mergeTestConfig(t, "a.en.mkv", "a.pt-br.mkv")
	for _, url := range []string{
		"/api/merge/frame?file=../../etc/passwd&t=1",
		"/api/merge/frame?file=a.en.mkv&t=abc",
		"/api/merge/frame?file=a.en.mkv&t=-3",
		"/api/merge/frame?file=missing.en.mkv&t=1",
		"/api/merge/frame?source=nope&file=a.en.mkv&t=1",
		"/api/merge/frame?file=a.en.txt&t=1",
	} {
		rec := httptest.NewRecorder()
		handleMergeFrame(cfg)(rec, httptest.NewRequest(http.MethodGet, url, nil))
		if rec.Code != http.StatusBadRequest {
			t.Errorf("%s: status %d, want 400", url, rec.Code)
		}
	}
	for _, url := range []string{
		"/api/merge/locate?a=a.en.mkv&b=../x.mkv&t=1",
		"/api/merge/locate?a=a.en.mkv&t=1",
		"/api/merge/locate?a=a.en.mkv&b=a.pt-br.mkv",
	} {
		rec := httptest.NewRecorder()
		handleMergeLocate(cfg)(rec, httptest.NewRequest(http.MethodGet, url, nil))
		if rec.Code != http.StatusBadRequest {
			t.Errorf("%s: status %d, want 400", url, rec.Code)
		}
	}
}
