package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"path/filepath"
	"sort"
	"strings"

	"anime-upscaling/internal/config"
	"anime-upscaling/internal/files"
	"anime-upscaling/internal/pipeline"
)

// handlePipelines handles GET /api/pipelines (list) and POST /api/pipelines (create).
func handlePipelines(ps *pipeline.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			list := ps.List()
			if list == nil {
				list = []pipeline.Pipeline{}
			}
			writeJSON(w, http.StatusOK, list)

		case http.MethodPost:
			var req struct {
				Name  string                  `json:"name"`
				Steps []pipeline.PipelineStep `json:"steps"`
			}
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON body"})
				return
			}
			if req.Name == "" {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": "name is required"})
				return
			}
			if len(req.Steps) == 0 {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": "at least one step is required"})
				return
			}
			if err := validateSteps(req.Steps); err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
				return
			}

			p, err := ps.Create(req.Name, req.Steps)
			if err != nil {
				writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
				return
			}
			writeJSON(w, http.StatusCreated, p)

		default:
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		}
	}
}

// handlePipelineRoutes handles /api/pipelines/{id}, /api/pipelines/{id}/run.
func handlePipelineRoutes(ps *pipeline.Store, jm *JobManager, cfg config.Config) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimPrefix(r.URL.Path, "/api/pipelines/")
		parts := strings.SplitN(path, "/", 2)
		id := parts[0]
		sub := ""
		if len(parts) > 1 {
			sub = parts[1]
		}

		if id == "" {
			http.Error(w, "missing pipeline id", http.StatusBadRequest)
			return
		}

		switch sub {
		case "":
			switch r.Method {
			case http.MethodGet:
				p := ps.Get(id)
				if p == nil {
					writeJSON(w, http.StatusNotFound, map[string]string{"error": "pipeline not found"})
					return
				}
				writeJSON(w, http.StatusOK, p)

			case http.MethodPut:
				var req struct {
					Name  *string                 `json:"name"`
					Steps []pipeline.PipelineStep `json:"steps"`
				}
				if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
					writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON body"})
					return
				}
				if req.Steps != nil {
					if len(req.Steps) == 0 {
						writeJSON(w, http.StatusBadRequest, map[string]string{"error": "at least one step is required"})
						return
					}
					if err := validateSteps(req.Steps); err != nil {
						writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
						return
					}
				}

				p, err := ps.Update(id, req.Name, req.Steps)
				if err != nil {
					writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
					return
				}
				if p == nil {
					writeJSON(w, http.StatusNotFound, map[string]string{"error": "pipeline not found"})
					return
				}
				writeJSON(w, http.StatusOK, p)

			case http.MethodDelete:
				if !ps.Delete(id) {
					writeJSON(w, http.StatusNotFound, map[string]string{"error": "pipeline not found"})
					return
				}
				writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})

			default:
				http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			}

		case "run":
			if r.Method != http.MethodPost {
				http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
				return
			}
			handleRunPipeline(ps, jm, cfg, id, w, r)

		default:
			http.NotFound(w, r)
		}
	}
}

func handleRunPipeline(ps *pipeline.Store, jm *JobManager, cfg config.Config, id string, w http.ResponseWriter, r *http.Request) {
	p := ps.Get(id)
	if p == nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "pipeline not found"})
		return
	}

	var req struct {
		Files  []string `json:"files"`
		Source string   `json:"source"`
		Path   string   `json:"path"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON body"})
		return
	}

	if req.Source == "" {
		req.Source = "input"
	}
	sourceDir, ok := resolveFolder(cfg, req.Source)
	if !ok {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid source (must be input, output, interpolated, or optimized)"})
		return
	}
	if !files.SafeRelDir(req.Path) {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid path"})
		return
	}

	// Resolve files recursively from sourceDir/path
	if len(req.Files) == 0 {
		all, err := files.WalkVideos(filepath.Join(sourceDir, req.Path), cfg.VideoExts)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to list files"})
			return
		}
		if len(all) == 0 {
			label := req.Source
			if req.Path != "" {
				label = req.Source + "/" + req.Path
			}
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": fmt.Sprintf("no video files found in %s/", label)})
			return
		}
		req.Files = make([]string, 0, len(all))
		for rel := range all {
			if req.Path != "" {
				rel = filepath.ToSlash(filepath.Join(req.Path, rel))
			}
			req.Files = append(req.Files, rel)
		}
		sort.Strings(req.Files)
	} else {
		for i, f := range req.Files {
			if req.Path != "" && !strings.Contains(f, "/") {
				f = filepath.ToSlash(filepath.Join(req.Path, f))
				req.Files[i] = f
			}
			if !files.SafeVideoRelPath(f, cfg.VideoExts) {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": fmt.Sprintf("invalid filename: %s", f)})
				return
			}
			if !files.FileExists(filepath.Join(sourceDir, f)) {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": fmt.Sprintf("file not found in %s/: %s", req.Source, f)})
				return
			}
		}
	}

	job := jm.StartPipelineJob(p.Name, p.Steps, req.Files, sourceDir)

	writeJSON(w, http.StatusCreated, map[string]interface{}{
		"id":            job.ID,
		"type":          job.Type,
		"status":        job.Status,
		"pipeline_name": p.Name,
		"files":         job.Files,
	})
}

func validateSteps(steps []pipeline.PipelineStep) error {
	for i, s := range steps {
		if !pipeline.ValidOperations[s.Operation] {
			return fmt.Errorf("step %d: invalid operation %q", i+1, s.Operation)
		}

		switch s.Operation {
		case "upscale":
			if s.Scale != 0 && s.Scale != 2 && s.Scale != 3 && s.Scale != 4 {
				return fmt.Errorf("step %d: scale must be 2, 3, or 4", i+1)
			}
			if !pipeline.ValidProcessors[s.Processor] {
				return fmt.Errorf("step %d: invalid processor %q", i+1, s.Processor)
			}
			if !pipeline.ValidUpscaleModels[s.Model] {
				return fmt.Errorf("step %d: invalid model %q", i+1, s.Model)
			}
			if s.Model != "" {
				effectiveScale := s.Scale
				if effectiveScale == 0 {
					effectiveScale = 2
				}
				if !pipeline.ValidModelScale(s.Model, effectiveScale) {
					return fmt.Errorf("step %d: model %q does not support scale %d", i+1, s.Model, effectiveScale)
				}
			}
			if s.NoiseLevel < 0 || s.NoiseLevel > 3 {
				return fmt.Errorf("step %d: noise_level must be between 0 and 3", i+1)
			}
		case "interpolate":
			if s.Multiplier != 0 && s.Multiplier != 2 && s.Multiplier != 3 && s.Multiplier != 4 {
				return fmt.Errorf("step %d: multiplier must be 2, 3, or 4", i+1)
			}
			if !pipeline.ValidRifeModels[s.RifeModel] {
				return fmt.Errorf("step %d: invalid rife_model %q", i+1, s.RifeModel)
			}
			if s.SceneThresh < 0 || s.SceneThresh > 100 {
				return fmt.Errorf("step %d: scene_thresh must be between 0 and 100", i+1)
			}
		case "optimize":
			if s.Quality != "" {
				if _, ok := pipeline.QualityToCRF[s.Quality]; !ok {
					return fmt.Errorf("step %d: quality must be ultra, alta, media, or baixa", i+1)
				}
			}
			if s.Resolution != 0 && s.Resolution != 1 && s.Resolution != 2 && s.Resolution != 4 {
				return fmt.Errorf("step %d: resolution must be 1, 2, or 4", i+1)
			}
			if s.FrameRate != 0 && s.FrameRate != 1 && s.FrameRate != 2 && s.FrameRate != 4 {
				return fmt.Errorf("step %d: frame_rate must be 1, 2, or 4", i+1)
			}
			if s.FrameRateMode != "" && s.FrameRateMode != "relative" && s.FrameRateMode != "absolute" {
				return fmt.Errorf("step %d: frame_rate_mode must be \"relative\" or \"absolute\"", i+1)
			}
			if s.FrameRateMode == "absolute" && s.FrameRateAbsolute <= 0 {
				return fmt.Errorf("step %d: frame_rate_absolute must be > 0 when frame_rate_mode is \"absolute\"", i+1)
			}
			if s.FrameRateAbsolute < 0 {
				return fmt.Errorf("step %d: frame_rate_absolute must be >= 0", i+1)
			}
			if s.Threads < 0 {
				return fmt.Errorf("step %d: threads must be >= 0", i+1)
			}
			if !pipeline.ValidCodecs[s.Codec] {
				return fmt.Errorf("step %d: invalid codec %q", i+1, s.Codec)
			}
			if !pipeline.ValidPresets[s.Preset] {
				return fmt.Errorf("step %d: invalid preset %q", i+1, s.Preset)
			}
			if !pipeline.ValidTunes[s.Tune] {
				return fmt.Errorf("step %d: invalid tune %q", i+1, s.Tune)
			}
			if !pipeline.ValidPixFmts[s.PixFmt] {
				return fmt.Errorf("step %d: invalid pix_fmt %q", i+1, s.PixFmt)
			}
			if !pipeline.ValidAudioCodecs[s.AudioCodec] {
				return fmt.Errorf("step %d: invalid audio_codec %q", i+1, s.AudioCodec)
			}
			if s.UseGPU && (s.Codec == "copy" || s.Codec == "libvpx-vp9") {
				return fmt.Errorf("step %d: use_gpu is incompatible with codec %q", i+1, s.Codec)
			}
		}
	}
	return nil
}
