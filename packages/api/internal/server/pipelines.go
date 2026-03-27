package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"path/filepath"
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
				Name  string              `json:"name"`
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
					Name  *string              `json:"name"`
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
		Files []string `json:"files"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON body"})
		return
	}

	// Resolve files from input/
	if len(req.Files) == 0 {
		all, err := files.ListVideos(cfg.InputDir, cfg.VideoExts)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to list files"})
			return
		}
		if len(all) == 0 {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "no video files found in input/"})
			return
		}
		req.Files = all
	} else {
		for _, f := range req.Files {
			if !files.FileExists(filepath.Join(cfg.InputDir, f)) {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": fmt.Sprintf("file not found in input/: %s", f)})
				return
			}
		}
	}

	job := jm.StartPipelineJob(p.Name, p.Steps, req.Files)

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
			if s.Scale != 0 && s.Scale != 2 && s.Scale != 4 {
				return fmt.Errorf("step %d: scale must be 2 or 4", i+1)
			}
			if !pipeline.ValidProcessors[s.Processor] {
				return fmt.Errorf("step %d: invalid processor %q", i+1, s.Processor)
			}
			if !pipeline.ValidUpscaleModels[s.Model] {
				return fmt.Errorf("step %d: invalid model %q", i+1, s.Model)
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
		}
	}
	return nil
}
