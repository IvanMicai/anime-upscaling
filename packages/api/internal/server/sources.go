package server

import (
	"encoding/json"
	"net/http"
	"path/filepath"
	"strings"
	"time"

	"os"

	"anime-upscaling/internal/config"
	"anime-upscaling/internal/runner"
	"anime-upscaling/internal/sources"
)

func sourcesFile(cfg config.Config) string {
	return cfg.BaseDir + "/sources.json"
}

// GET /api/sources — list all sources
// POST /api/sources — add a new source
func handleSources(cfg config.Config) http.HandlerFunc {
	r := runner.NewRunner(cfg)

	return func(w http.ResponseWriter, req *http.Request) {
		sf := sourcesFile(cfg)

		switch req.Method {
		case http.MethodGet:
			list, err := sources.Load(sf)
			if err != nil {
				writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
				return
			}
			writeJSON(w, http.StatusOK, list)

		case http.MethodPost:
			var body struct {
				Name string `json:"name"`
				Path string `json:"path"`
			}
			if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON body"})
				return
			}
			if body.Name == "" || body.Path == "" {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": "name and path are required"})
				return
			}

			// Validate path exists
			exists, err := r.PathExists(body.Path)
			if err != nil {
				writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to validate path: " + err.Error()})
				return
			}
			if !exists {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": "path does not exist: " + body.Path})
				return
			}

			s, err := sources.Add(sf, body.Name, body.Path)
			if err != nil {
				writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
				return
			}
			writeJSON(w, http.StatusCreated, s)

		default:
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		}
	}
}

// Routes under /api/sources/{id}...
func handleSourceRoutes(cfg config.Config) http.HandlerFunc {
	r := runner.NewRunner(cfg)

	return func(w http.ResponseWriter, req *http.Request) {
		sf := sourcesFile(cfg)

		// Parse: /api/sources/{id} or /api/sources/{id}/files etc.
		path := strings.TrimPrefix(req.URL.Path, "/api/sources/")
		parts := strings.SplitN(path, "/", 2)
		id := parts[0]
		sub := ""
		if len(parts) > 1 {
			sub = parts[1]
		}

		if id == "" {
			http.Error(w, "missing source id", http.StatusBadRequest)
			return
		}

		// Look up source
		src, err := sources.Get(sf, id)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		if src == nil {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "source not found"})
			return
		}

		switch sub {
		case "":
			// DELETE /api/sources/{id}
			if req.Method != http.MethodDelete {
				http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
				return
			}
			if err := sources.Remove(sf, id); err != nil {
				writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
				return
			}
			writeJSON(w, http.StatusOK, map[string]string{"deleted": id})

		case "files":
			// GET /api/sources/{id}/files?refresh=true
			if req.Method != http.MethodGet {
				http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
				return
			}
			forceRefresh := req.URL.Query().Get("refresh") == "true"
			fileList, err := r.ListFiles(src.Path, cfg.VideoExts)
			if err != nil {
				writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
				return
			}

			type sourceFileWithStatus struct {
				Name            string `json:"name"`
				Size            int64  `json:"size"`
				Width           int    `json:"width,omitempty"`
				Height          int    `json:"height,omitempty"`
				InInput         bool   `json:"in_input,omitempty"`
				InOutput        bool   `json:"in_output,omitempty"`
				InOptimized     bool   `json:"in_optimized,omitempty"`
				InputSize       int64  `json:"input_size,omitempty"`
				OutputSize      int64  `json:"output_size,omitempty"`
				OptimizedSize   int64  `json:"optimized_size,omitempty"`
				InputWidth      int    `json:"input_width,omitempty"`
				InputHeight     int    `json:"input_height,omitempty"`
				UpscaledWidth   int    `json:"upscaled_width,omitempty"`
				UpscaledHeight  int    `json:"upscaled_height,omitempty"`
				OptimizedWidth  int    `json:"optimized_width,omitempty"`
				OptimizedHeight int    `json:"optimized_height,omitempty"`
			}

			// Build mounts and filesByLabel for batch ffprobe
			names := make([]string, len(fileList))
			for i, f := range fileList {
				names[i] = f.Name
			}
			mounts := []runner.DirMount{{Label: "source", HostDir: src.Path}}
			filesByLabel := map[string][]string{"source": names}

			var inputNames, outputNames, optNames []string
			for _, f := range fileList {
				if _, err := os.Stat(filepath.Join(cfg.InputDir, f.Name)); err == nil {
					inputNames = append(inputNames, f.Name)
				}
				if _, err := os.Stat(filepath.Join(cfg.OutputDir, f.Name)); err == nil {
					outputNames = append(outputNames, f.Name)
				}
				if _, err := os.Stat(filepath.Join(cfg.OptimizedDir, f.Name)); err == nil {
					optNames = append(optNames, f.Name)
				}
			}
			if len(inputNames) > 0 {
				mounts = append(mounts, runner.DirMount{Label: "input", HostDir: cfg.InputDir})
				filesByLabel["input"] = inputNames
			}
			if len(outputNames) > 0 {
				mounts = append(mounts, runner.DirMount{Label: "output", HostDir: cfg.OutputDir})
				filesByLabel["output"] = outputNames
			}
			if len(optNames) > 0 {
				mounts = append(mounts, runner.DirMount{Label: "optimized", HostDir: cfg.OptimizedDir})
				filesByLabel["optimized"] = optNames
			}

			ctx := req.Context()
			allRes, cachedAt, _ := r.FFprobeBatchResolutionMultiDirCached(ctx, mounts, filesByLabel, forceRefresh)

			result := make([]sourceFileWithStatus, 0, len(fileList))
			for _, f := range fileList {
				sf := sourceFileWithStatus{Name: f.Name, Size: f.Size}
				if allRes != nil {
					if res, ok := allRes["source"][f.Name]; ok {
						sf.Width = res.Width
						sf.Height = res.Height
					}
				}
				if info, err := os.Stat(filepath.Join(cfg.InputDir, f.Name)); err == nil {
					sf.InInput = true
					sf.InputSize = info.Size()
					if allRes != nil {
						if res, ok := allRes["input"][f.Name]; ok {
							sf.InputWidth = res.Width
							sf.InputHeight = res.Height
						}
					}
				}
				if info, err := os.Stat(filepath.Join(cfg.OutputDir, f.Name)); err == nil {
					sf.InOutput = true
					sf.OutputSize = info.Size()
					if allRes != nil {
						if res, ok := allRes["output"][f.Name]; ok {
							sf.UpscaledWidth = res.Width
							sf.UpscaledHeight = res.Height
						}
					}
				}
				if info, err := os.Stat(filepath.Join(cfg.OptimizedDir, f.Name)); err == nil {
					sf.InOptimized = true
					sf.OptimizedSize = info.Size()
					if allRes != nil {
						if res, ok := allRes["optimized"][f.Name]; ok {
							sf.OptimizedWidth = res.Width
							sf.OptimizedHeight = res.Height
						}
					}
				}
				result = append(result, sf)
			}
			writeJSON(w, http.StatusOK, map[string]interface{}{
				"files":     result,
				"cached_at": cachedAt.Format(time.RFC3339),
			})

		case "import":
			// POST /api/sources/{id}/import
			if req.Method != http.MethodPost {
				http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
				return
			}
			var body struct {
				Files []string `json:"files"`
			}
			if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON body"})
				return
			}
			if len(body.Files) == 0 {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": "files list is required"})
				return
			}
			copied, err := r.CopyFiles(src.Path, cfg.InputDir, body.Files)
			if err != nil {
				writeJSON(w, http.StatusInternalServerError, map[string]interface{}{
					"error":  err.Error(),
					"copied": copied,
				})
				return
			}
			writeJSON(w, http.StatusOK, map[string]int{"copied": copied})

		case "export":
			// POST /api/sources/{id}/export
			if req.Method != http.MethodPost {
				http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
				return
			}
			var body struct {
				Files []string `json:"files"`
				From  string   `json:"from"`
			}
			if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON body"})
				return
			}
			if len(body.Files) == 0 {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": "files list is required"})
				return
			}
			var fromDir string
			switch body.From {
			case "output":
				fromDir = cfg.OutputDir
			case "optimized":
				fromDir = cfg.OptimizedDir
			default:
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": "from must be 'output' or 'optimized'"})
				return
			}
			copied, err := r.CopyFiles(fromDir, src.Path, body.Files)
			if err != nil {
				writeJSON(w, http.StatusInternalServerError, map[string]interface{}{
					"error":  err.Error(),
					"copied": copied,
				})
				return
			}
			writeJSON(w, http.StatusOK, map[string]int{"copied": copied})

		default:
			http.NotFound(w, req)
		}
	}
}
