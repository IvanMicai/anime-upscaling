package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"path/filepath"
	"strings"
	"time"

	"anime-upscaling/internal/config"
	"anime-upscaling/internal/files"
)

func CmdServe(cfg config.Config) error {
	jm := NewJobManager(cfg)
	auth := apiKeyMiddleware(cfg.ApiKey)

	mux := http.NewServeMux()
	mux.HandleFunc("/api/files", corsMiddleware(auth(handleFiles(cfg))))
	mux.HandleFunc("/api/jobs", corsMiddleware(auth(handleJobs(jm, cfg))))
	mux.HandleFunc("/api/jobs/", corsMiddleware(auth(handleJobRoutes(jm))))

	addr := ":" + cfg.Port
	fmt.Printf("Server listening on %s\n", addr)
	return http.ListenAndServe(addr, mux)
}

func corsMiddleware(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, X-API-Key")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next(w, r)
	}
}

func apiKeyMiddleware(apiKey string) func(http.HandlerFunc) http.HandlerFunc {
	return func(next http.HandlerFunc) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			if apiKey == "" {
				next(w, r)
				return
			}
			if r.Header.Get("X-API-Key") != apiKey {
				writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "invalid or missing API key"})
				return
			}
			next(w, r)
		}
	}
}

// GET /api/files?dir=input
func handleFiles(cfg config.Config) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		dir := r.URL.Query().Get("dir")
		if dir == "" {
			dir = "input"
		}

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

		videoFiles, err := files.ListVideos(fullPath, cfg.VideoExts)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		if videoFiles == nil {
			videoFiles = []string{}
		}

		writeJSON(w, http.StatusOK, map[string]interface{}{
			"dir":   dir,
			"files": videoFiles,
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
		Type  string   `json:"type"`
		Files []string `json:"files"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON body"})
		return
	}

	validTypes := map[string]bool{"upscale": true, "optimize": true, "pipeline": true}
	if !validTypes[req.Type] {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "type must be upscale, optimize, or pipeline"})
		return
	}

	// If no files specified, use all videos in input/
	if len(req.Files) == 0 {
		all, err := files.ListVideos(cfg.InputDir, cfg.VideoExts)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to list input files"})
			return
		}
		if len(all) == 0 {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "no video files found in input/"})
			return
		}
		req.Files = all
	} else {
		// Validate each file exists
		for _, f := range req.Files {
			if !files.FileExists(filepath.Join(cfg.InputDir, f)) {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": fmt.Sprintf("file not found in input/: %s", f)})
				return
			}
		}
	}

	job := jm.StartJob(req.Type, req.Files)

	writeJSON(w, http.StatusCreated, map[string]interface{}{
		"id":     job.ID,
		"type":   job.Type,
		"status": job.Status,
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
		Files      []string    `json:"files"`
		Progress   JobProgress `json:"progress"`
		CreatedAt  time.Time   `json:"created_at"`
		FinishedAt *time.Time  `json:"finished_at,omitempty"`
	}
	writeJSON(w, http.StatusOK, jobDetail{
		ID:         snap.ID,
		Type:       snap.Type,
		Status:     snap.Status,
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
