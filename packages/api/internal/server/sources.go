package server

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"anime-upscaling/internal/config"
	"anime-upscaling/internal/docker"
	"anime-upscaling/internal/sources"
)

func sourcesFile(cfg config.Config) string {
	return cfg.BaseDir + "/sources.json"
}

// GET /api/sources — list all sources
// POST /api/sources — add a new source
func handleSources(cfg config.Config) http.HandlerFunc {
	d := docker.NewDocker(cfg)

	return func(w http.ResponseWriter, r *http.Request) {
		sf := sourcesFile(cfg)

		switch r.Method {
		case http.MethodGet:
			list, err := sources.Load(sf)
			if err != nil {
				writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
				return
			}
			writeJSON(w, http.StatusOK, list)

		case http.MethodPost:
			var req struct {
				Name string `json:"name"`
				Path string `json:"path"`
			}
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON body"})
				return
			}
			if req.Name == "" || req.Path == "" {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": "name and path are required"})
				return
			}

			// Validate path exists on host via Docker
			ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
			defer cancel()
			exists, err := d.PathExists(ctx, req.Path)
			if err != nil {
				writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to validate path: " + err.Error()})
				return
			}
			if !exists {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": "path does not exist on host: " + req.Path})
				return
			}

			s, err := sources.Add(sf, req.Name, req.Path)
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
	d := docker.NewDocker(cfg)

	return func(w http.ResponseWriter, r *http.Request) {
		sf := sourcesFile(cfg)

		// Parse: /api/sources/{id} or /api/sources/{id}/files etc.
		path := strings.TrimPrefix(r.URL.Path, "/api/sources/")
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
			if r.Method != http.MethodDelete {
				http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
				return
			}
			if err := sources.Remove(sf, id); err != nil {
				writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
				return
			}
			writeJSON(w, http.StatusOK, map[string]string{"deleted": id})

		case "files":
			// GET /api/sources/{id}/files
			if r.Method != http.MethodGet {
				http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
				return
			}
			ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
			defer cancel()
			files, err := d.ListFiles(ctx, src.Path, cfg.VideoExts)
			if err != nil {
				writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
				return
			}
			if files == nil {
				files = []docker.FileInfo{}
			}
			writeJSON(w, http.StatusOK, map[string]interface{}{"files": files})

		case "import":
			// POST /api/sources/{id}/import
			if r.Method != http.MethodPost {
				http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
				return
			}
			var req struct {
				Files []string `json:"files"`
			}
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON body"})
				return
			}
			if len(req.Files) == 0 {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": "files list is required"})
				return
			}
			ctx, cancel := context.WithTimeout(r.Context(), 10*time.Minute)
			defer cancel()
			copied, err := d.CopyFiles(ctx, src.Path, cfg.InputDir, req.Files)
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
			if r.Method != http.MethodPost {
				http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
				return
			}
			var req struct {
				Files []string `json:"files"`
				From  string   `json:"from"`
			}
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON body"})
				return
			}
			if len(req.Files) == 0 {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": "files list is required"})
				return
			}
			var fromDir string
			switch req.From {
			case "output":
				fromDir = cfg.OutputDir
			case "optimized":
				fromDir = cfg.OptimizedDir
			default:
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": "from must be 'output' or 'optimized'"})
				return
			}
			ctx, cancel := context.WithTimeout(r.Context(), 10*time.Minute)
			defer cancel()
			copied, err := d.CopyFiles(ctx, fromDir, src.Path, req.Files)
			if err != nil {
				writeJSON(w, http.StatusInternalServerError, map[string]interface{}{
					"error":  err.Error(),
					"copied": copied,
				})
				return
			}
			writeJSON(w, http.StatusOK, map[string]int{"copied": copied})

		default:
			http.NotFound(w, r)
		}
	}
}
