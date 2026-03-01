package server

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"path/filepath"
	"strings"
	"time"

	"anime-upscaling/internal/config"
	"anime-upscaling/internal/files"
	"anime-upscaling/internal/runner"
)

func CmdServe(cfg config.Config) error {
	jm := NewJobManager(cfg)

	mux := http.NewServeMux()
	mux.HandleFunc("/api/files", corsMiddleware(handleFiles(cfg)))
	mux.HandleFunc("/api/jobs", corsMiddleware(handleJobs(jm, cfg)))
	mux.HandleFunc("/api/jobs/", corsMiddleware(handleJobRoutes(jm)))
	mux.HandleFunc("/api/sources", corsMiddleware(handleSources(cfg)))
	mux.HandleFunc("/api/sources/", corsMiddleware(handleSourceRoutes(cfg)))

	addr := ":" + cfg.Port
	fmt.Printf("Server listening on %s\n", addr)
	return http.ListenAndServe(addr, mux)
}

func corsMiddleware(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next(w, r)
	}
}

// GET /api/files?dir=input&refresh=true
func handleFiles(cfg config.Config) http.HandlerFunc {
	r := runner.NewRunner(cfg)

	return func(w http.ResponseWriter, req *http.Request) {
		if req.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		dir := req.URL.Query().Get("dir")
		if dir == "" {
			dir = "input"
		}
		forceRefresh := req.URL.Query().Get("refresh") == "true"

		allowed := map[string]string{
			"input":     cfg.InputDir,
			"output":    cfg.OutputDir,
			"optimized": cfg.OptimizedDir,
		}

		fullPath, ok := allowed[dir]
		if !ok {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid dir: must be input, output, or optimized"})
			return
		}

		var videoFiles []files.VideoFile
		var err error
		if dir == "input" {
			videoFiles, err = files.ListVideosWithStatus(fullPath, cfg.OutputDir, cfg.OptimizedDir, cfg.VideoExts)
		} else if dir == "output" {
			videoFiles, err = files.ListOutputWithStatus(fullPath, cfg.OptimizedDir, cfg.VideoExts)
		} else if dir == "optimized" {
			videoFiles, err = files.ListOptimizedWithStatus(fullPath, cfg.InputDir, cfg.OutputDir, cfg.VideoExts)
		} else {
			videoFiles, err = files.ListVideosWithSize(fullPath, cfg.VideoExts)
		}
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		if videoFiles == nil {
			videoFiles = []files.VideoFile{}
		}

		var cachedAt time.Time

		// Enrich with resolution data
		if len(videoFiles) > 0 {
			names := make([]string, len(videoFiles))
			for i, f := range videoFiles {
				names[i] = f.Name
			}
			ctx, cancel := context.WithTimeout(req.Context(), 30*time.Second)
			defer cancel()

			// Build mounts and filesByLabel
			mounts := []runner.DirMount{{Label: dir, HostDir: fullPath}}
			filesByLabel := map[string][]string{dir: names}

			if dir == "input" {
				var upscaledNames, optNames []string
				for _, f := range videoFiles {
					if f.HasUpscaled {
						upscaledNames = append(upscaledNames, f.Name)
					}
					if f.HasOptimized {
						optNames = append(optNames, f.Name)
					}
				}
				if len(upscaledNames) > 0 {
					mounts = append(mounts, runner.DirMount{Label: "output", HostDir: cfg.OutputDir})
					filesByLabel["output"] = upscaledNames
				}
				if len(optNames) > 0 {
					mounts = append(mounts, runner.DirMount{Label: "optimized", HostDir: cfg.OptimizedDir})
					filesByLabel["optimized"] = optNames
				}
			} else if dir == "output" {
				var optNames []string
				for _, f := range videoFiles {
					if f.HasOptimized {
						optNames = append(optNames, f.Name)
					}
				}
				if len(optNames) > 0 {
					mounts = append(mounts, runner.DirMount{Label: "optimized", HostDir: cfg.OptimizedDir})
					filesByLabel["optimized"] = optNames
				}
			} else if dir == "optimized" {
				var inputNames, upscaledNames []string
				for _, f := range videoFiles {
					if f.HasInput {
						inputNames = append(inputNames, f.Name)
					}
					if f.HasUpscaled {
						upscaledNames = append(upscaledNames, f.Name)
					}
				}
				if len(inputNames) > 0 {
					mounts = append(mounts, runner.DirMount{Label: "input", HostDir: cfg.InputDir})
					filesByLabel["input"] = inputNames
				}
				if len(upscaledNames) > 0 {
					mounts = append(mounts, runner.DirMount{Label: "output", HostDir: cfg.OutputDir})
					filesByLabel["output"] = upscaledNames
				}
			}

			results, ca, _ := r.FFprobeBatchResolutionMultiDirCached(ctx, mounts, filesByLabel, forceRefresh)
			cachedAt = ca

			if results != nil {
				// Primary dir resolution
				if dirRes := results[dir]; dirRes != nil {
					for i, f := range videoFiles {
						if res, ok := dirRes[f.Name]; ok {
							videoFiles[i].Width = res.Width
							videoFiles[i].Height = res.Height
						}
					}
				}
				// Cross-dir resolutions
				if upRes := results["output"]; upRes != nil && dir != "output" {
					for i, f := range videoFiles {
						if res, ok := upRes[f.Name]; ok {
							videoFiles[i].UpscaledWidth = res.Width
							videoFiles[i].UpscaledHeight = res.Height
						}
					}
				}
				if optRes := results["optimized"]; optRes != nil && dir != "optimized" {
					for i, f := range videoFiles {
						if res, ok := optRes[f.Name]; ok {
							videoFiles[i].OptimizedWidth = res.Width
							videoFiles[i].OptimizedHeight = res.Height
						}
					}
				}
				if inRes := results["input"]; inRes != nil && dir != "input" {
					for i, f := range videoFiles {
						if res, ok := inRes[f.Name]; ok {
							videoFiles[i].InputWidth = res.Width
							videoFiles[i].InputHeight = res.Height
						}
					}
				}
			}
		}

		writeJSON(w, http.StatusOK, map[string]interface{}{
			"dir":       dir,
			"files":     videoFiles,
			"cached_at": cachedAt.Format(time.RFC3339),
		})
	}
}

// POST /api/jobs (create) and GET /api/jobs (list)
func handleJobs(jm *JobManager, cfg config.Config) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			handleListJobs(jm, w, r)
		case http.MethodPost:
			handleCreateJob(jm, cfg, w, r)
		default:
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		}
	}
}

func handleListJobs(jm *JobManager, w http.ResponseWriter, r *http.Request) {
	jobs := jm.ListJobs()
	writeJSON(w, http.StatusOK, jobs)
}

func handleCreateJob(jm *JobManager, cfg config.Config, w http.ResponseWriter, r *http.Request) {
	var req struct {
		Type       string   `json:"type"`
		Files      []string `json:"files"`
		Source     string   `json:"source"`
		Scale      int      `json:"scale"`
		Resolution int      `json:"resolution"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON body"})
		return
	}

	validTypes := map[string]bool{"upscale": true, "optimize": true, "pipeline": true, "check": true}
	if !validTypes[req.Type] {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "type must be upscale, optimize, pipeline, or check"})
		return
	}

	if req.Scale == 0 {
		req.Scale = 2
	}
	if (req.Type == "upscale" || req.Type == "pipeline") && req.Scale != 2 && req.Scale != 4 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "scale must be 2 or 4"})
		return
	}

	if req.Resolution == 0 {
		req.Resolution = 1
	}
	if req.Type == "optimize" && req.Resolution != 1 && req.Resolution != 2 && req.Resolution != 4 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "resolution must be 1, 2, or 4"})
		return
	}

	if req.Source == "" {
		req.Source = "input"
	}
	validSources := map[string]bool{"input": true, "output": true}
	if req.Type == "check" {
		validSources["optimized"] = true
	}
	if !validSources[req.Source] {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid source"})
		return
	}
	if req.Source == "output" && req.Type != "optimize" && req.Type != "check" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "source=output is only allowed for optimize or check jobs"})
		return
	}

	sourceDir := cfg.InputDir
	switch req.Source {
	case "output":
		sourceDir = cfg.OutputDir
	case "optimized":
		sourceDir = cfg.OptimizedDir
	}

	// If no files specified, use all videos in source dir
	if len(req.Files) == 0 {
		all, err := files.ListVideos(sourceDir, cfg.VideoExts)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to list files"})
			return
		}
		if len(all) == 0 {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": fmt.Sprintf("no video files found in %s/", req.Source)})
			return
		}
		req.Files = all
	} else {
		// Validate each file exists
		for _, f := range req.Files {
			if !files.FileExists(filepath.Join(sourceDir, f)) {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": fmt.Sprintf("file not found in %s/: %s", req.Source, f)})
				return
			}
		}
	}

	job := jm.StartJob(req.Type, req.Files, req.Source, req.Scale, req.Resolution)

	writeJSON(w, http.StatusCreated, map[string]interface{}{
		"id":     job.ID,
		"type":   job.Type,
		"status": job.Status,
		"source": job.Source,
		"scale":  job.Scale,
		"files":  job.Files,
	})
}

// Routes under /api/jobs/{id}...
func handleJobRoutes(jm *JobManager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Parse: /api/jobs/{id} or /api/jobs/{id}/logs or /api/jobs/{id}/cancel
		path := strings.TrimPrefix(r.URL.Path, "/api/jobs/")
		parts := strings.SplitN(path, "/", 2)
		id := parts[0]
		sub := ""
		if len(parts) > 1 {
			sub = parts[1]
		}

		if id == "" {
			http.Error(w, "missing job id", http.StatusBadRequest)
			return
		}

		switch sub {
		case "":
			if r.Method != http.MethodGet {
				http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
				return
			}
			handleGetJob(jm, id, w, r)
		case "logs":
			if r.Method != http.MethodGet {
				http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
				return
			}
			handleJobLogs(jm, id, w, r)
		case "cancel":
			if r.Method != http.MethodPost {
				http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
				return
			}
			handleCancelJob(jm, id, w, r)
		default:
			http.NotFound(w, r)
		}
	}
}

func handleGetJob(jm *JobManager, id string, w http.ResponseWriter, r *http.Request) {
	job := jm.GetJob(id)
	if job == nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "job not found"})
		return
	}
	snap := job.snapshotWithLogs()
	// Include logs in the JSON output for detail endpoint
	type jobDetail struct {
		ID         string      `json:"id"`
		Type       string      `json:"type"`
		Status     string      `json:"status"`
		Source     string      `json:"source"`
		Scale      int         `json:"scale"`
		Resolution int         `json:"resolution"`
		Files      []string    `json:"files"`
		Progress   JobProgress `json:"progress"`
		CreatedAt  time.Time   `json:"created_at"`
		FinishedAt *time.Time  `json:"finished_at,omitempty"`
	}
	writeJSON(w, http.StatusOK, jobDetail{
		ID:         snap.ID,
		Type:       snap.Type,
		Status:     snap.Status,
		Source:     snap.Source,
		Scale:      snap.Scale,
		Resolution: snap.Resolution,
		Files:      snap.Files,
		Progress:   snap.Progress,
		CreatedAt:  snap.CreatedAt,
		FinishedAt: snap.FinishedAt,
	})
}

func handleJobLogs(jm *JobManager, id string, w http.ResponseWriter, r *http.Request) {
	job := jm.GetJob(id)
	if job == nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "job not found"})
		return
	}

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming not supported", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	// Subscribe first, then send history to avoid missing events in between
	ch, running := job.subscribe()
	if running {
		defer job.unsubscribe(ch)
	}

	job.mu.Lock()
	history := make([]logEntry, len(job.Logs))
	copy(history, job.Logs)
	job.mu.Unlock()

	for _, e := range history {
		data, _ := json.Marshal(e)
		fmt.Fprintf(w, "data: %s\n\n", data)
	}
	flusher.Flush()

	if !running {
		return
	}

	ctx := r.Context()
	for {
		select {
		case <-ctx.Done():
			return
		case e, ok := <-ch:
			if !ok {
				// Job finished, channel closed
				return
			}
			data, _ := json.Marshal(e)
			fmt.Fprintf(w, "data: %s\n\n", data)
			flusher.Flush()
		}
	}
}

func handleCancelJob(jm *JobManager, id string, w http.ResponseWriter, r *http.Request) {
	job := jm.CancelJob(id)
	if job == nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "job not found"})
		return
	}

	// Give goroutine a moment to update status
	time.Sleep(50 * time.Millisecond)

	snap := job.snapshot()
	writeJSON(w, http.StatusOK, map[string]string{
		"id":     snap.ID,
		"status": snap.Status,
	})
}

func writeJSON(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}
