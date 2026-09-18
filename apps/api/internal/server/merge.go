package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"path/filepath"
	"strings"

	"anime-upscaling/internal/config"
	"anime-upscaling/internal/dualaudio"
	"anime-upscaling/internal/files"
)

// mergeRequest is the merge-specific part of POST /api/jobs.
//
// What to merge is given one of three ways, in this order of precedence:
// `paths` (every video under each folder — the "two folders" case), `files` (an
// explicit list, which may span folders), or neither (every video under
// source/path). Files are then paired by NAME: "ep01.pt-br.mp4" and
// "ep01.en.mkv" become "ep01.mkv".
type mergeRequest struct {
	Paths             []string `json:"paths"`
	MergeVideo        string   `json:"merge_video"`         // "auto" or a language tag
	MergeTick         float64  `json:"merge_tick"`          // seconds between sync checks
	MergeGapFill      string   `json:"merge_gap_fill"`      // "base" or "silence"
	MergeForce        bool     `json:"merge_force"`         // write even if the sync is not guaranteed
	MergeDefaultAudio string   `json:"merge_default_audio"` // language tag of the default track
}

// resolveMergeFiles turns the request into the list of candidate files,
// relative to sourceDir.
func resolveMergeFiles(cfg config.Config, sourceDir, path string, fileList, paths []string) ([]string, error) {
	if !files.SafeRelDir(path) {
		return nil, fmt.Errorf("invalid path")
	}
	walk := func(rel string) ([]string, error) {
		found, err := files.WalkVideos(filepath.Join(sourceDir, rel), cfg.VideoExts)
		if err != nil {
			return nil, fmt.Errorf("failed to list %q", rel)
		}
		out := make([]string, 0, len(found))
		for f := range found {
			out = append(out, filepath.ToSlash(filepath.Join(rel, f)))
		}
		return out, nil
	}

	switch {
	case len(paths) > 0:
		var all []string
		seen := map[string]bool{}
		for _, p := range paths {
			if !files.SafeRelDir(p) {
				return nil, fmt.Errorf("invalid folder: %s", p)
			}
			got, err := walk(p)
			if err != nil {
				return nil, err
			}
			for _, f := range got {
				if !seen[f] { // nested selections must not list a file twice
					seen[f] = true
					all = append(all, f)
				}
			}
		}
		return all, nil
	case len(fileList) > 0:
		out := make([]string, len(fileList))
		for i, f := range fileList {
			if path != "" && !strings.Contains(f, "/") {
				f = filepath.ToSlash(filepath.Join(path, f))
			}
			if !files.SafeVideoRelPath(f, cfg.VideoExts) {
				return nil, fmt.Errorf("invalid filename: %s", f)
			}
			if !files.FileExists(filepath.Join(sourceDir, f)) {
				return nil, fmt.Errorf("file not found: %s", f)
			}
			out[i] = f
		}
		return out, nil
	}
	return walk(path)
}

func normaliseMerge(m *mergeRequest) error {
	if m.MergeVideo == "" {
		m.MergeVideo = dualaudio.VideoAuto
	}
	m.MergeVideo = strings.ToLower(m.MergeVideo)
	m.MergeDefaultAudio = strings.ToLower(m.MergeDefaultAudio)
	if m.MergeGapFill == "" {
		m.MergeGapFill = string(dualaudio.GapFillBase)
	}
	if m.MergeGapFill != string(dualaudio.GapFillBase) && m.MergeGapFill != string(dualaudio.GapFillSilence) {
		return fmt.Errorf(`merge_gap_fill must be "base" or "silence"`)
	}
	if m.MergeTick < 0 || (m.MergeTick != 0 && (m.MergeTick < dualaudio.MinTickSec || m.MergeTick > dualaudio.MaxTickSec)) {
		return fmt.Errorf("merge_tick must be between %.0f and %.0f seconds", dualaudio.MinTickSec, dualaudio.MaxTickSec)
	}
	m.MergeTick = dualaudio.ClampTick(m.MergeTick)
	return nil
}

// checkLanguageChoice rejects a merge_video / merge_default_audio that names a
// language none of the pairs has — it would fail every file, one by one, later.
func checkLanguageChoice(field, tag string, pairs []dualaudio.NamePair) error {
	if tag == "" || tag == dualaudio.VideoAuto {
		return nil
	}
	for _, p := range pairs {
		if p.LangA.Tag != tag && p.LangB.Tag != tag {
			return fmt.Errorf("%s=%q is not a language of %s", field, tag, p.Output)
		}
	}
	return nil
}

func handleCreateMergeJob(jm *JobManager, cfg config.Config, w http.ResponseWriter, source, path string, fileList []string, m mergeRequest) {
	bad := func(err error) { writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()}) }
	if source == "" {
		source = "input"
	}
	sourceDir, ok := resolveFolder(cfg, source)
	if !ok || source == "merged" {
		bad(fmt.Errorf("invalid source for a merge (must be input, output, interpolated, or optimized)"))
		return
	}
	if err := normaliseMerge(&m); err != nil {
		bad(err)
		return
	}
	candidates, err := resolveMergeFiles(cfg, sourceDir, path, fileList, m.Paths)
	if err != nil {
		bad(err)
		return
	}
	pairs, unpaired := dualaudio.PairByName(candidates)
	if len(pairs) == 0 {
		bad(fmt.Errorf("no pair found: a merge needs two files with the same name and different language tags, e.g. name.pt-br.mp4 + name.en.mp4"))
		return
	}
	if err := checkLanguageChoice("merge_video", m.MergeVideo, pairs); err != nil {
		bad(err)
		return
	}
	if err := checkLanguageChoice("merge_default_audio", m.MergeDefaultAudio, pairs); err != nil {
		bad(err)
		return
	}

	outputs := make([]string, len(pairs))
	for i, p := range pairs {
		outputs[i] = p.Output
	}
	job := jm.StartJob(StartJobParams{
		Type: "merge", Files: outputs, Source: source, SourceDir: sourceDir,
		Merge: &MergeParams{Video: m.MergeVideo, TickSec: m.MergeTick, GapFill: m.MergeGapFill,
			Force: m.MergeForce, DefaultAudio: m.MergeDefaultAudio, Pairs: pairs, Unpaired: unpaired},
	})
	writeJSON(w, http.StatusCreated, map[string]interface{}{
		"id": job.ID, "type": job.Type, "status": job.Status, "files": job.Files,
		"pairs": len(pairs), "unpaired": len(unpaired),
	})
}

// POST /api/merge/preview — the pairing a merge job WOULD do, without starting
// it. Pairing by name is only trustworthy if it can be seen before it runs.
func handleMergePreview(cfg config.Config) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		var req struct {
			Source string   `json:"source"`
			Path   string   `json:"path"`
			Files  []string `json:"files"`
			Paths  []string `json:"paths"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON body"})
			return
		}
		if req.Source == "" {
			req.Source = "input"
		}
		sourceDir, ok := resolveFolder(cfg, req.Source)
		if !ok || req.Source == "merged" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid source"})
			return
		}
		candidates, err := resolveMergeFiles(cfg, sourceDir, req.Path, req.Files, req.Paths)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		pairs, unpaired := dualaudio.PairByName(candidates)
		if pairs == nil {
			pairs = []dualaudio.NamePair{}
		}
		if unpaired == nil {
			unpaired = []dualaudio.Unpaired{}
		}
		for i := range pairs {
			pairs[i].Exists = files.FileExists(filepath.Join(cfg.MergedDir, filepath.FromSlash(pairs[i].Output)))
		}
		writeJSON(w, http.StatusOK, map[string]interface{}{"pairs": pairs, "unpaired": unpaired})
	}
}
