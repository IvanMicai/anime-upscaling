package server

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"anime-upscaling/internal/cache"
	"anime-upscaling/internal/config"
	"anime-upscaling/internal/files"
	"anime-upscaling/internal/pipeline"
)

func CmdServe(cfg config.Config) error {
	// Clean temp dir from any stale files left by previous crashes
	os.RemoveAll(cfg.TempDir)
	os.MkdirAll(cfg.TempDir, 0755)

	if err := cache.BuildFileStatusCache(cfg); err != nil {
		fmt.Printf("Warning: cache build failed: %v\n", err)
	}

	jm := NewJobManager(cfg)
	ps := pipeline.NewStore(filepath.Join(cfg.BaseDir, "pipelines.json"))

	mux := http.NewServeMux()
	mux.HandleFunc("/api/files/download", corsMiddleware(handleFileDownload(cfg)))
	mux.HandleFunc("/api/files", corsMiddleware(handleFiles(cfg)))
	mux.HandleFunc("/api/jobs", corsMiddleware(handleJobs(jm, cfg)))
	mux.HandleFunc("/api/jobs/", corsMiddleware(handleJobRoutes(jm)))
	mux.HandleFunc("/api/pipelines", corsMiddleware(handlePipelines(ps)))
	mux.HandleFunc("/api/pipelines/", corsMiddleware(handlePipelineRoutes(ps, jm, cfg)))
	mux.HandleFunc("/api/settings", corsMiddleware(handleSettings(jm)))

	addr := ":" + cfg.Port
	fmt.Printf("Server listening on %s\n", addr)
	return http.ListenAndServe(addr, mux)
}

func corsMiddleware(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next(w, r)
	}
}

// GET /api/files/download?dir=input&name=video.mkv
func handleFileDownload(cfg config.Config) http.HandlerFunc {
	allowed := map[string]string{
		"input":        cfg.InputDir,
		"output":       cfg.OutputDir,
		"optimized":    cfg.OptimizedDir,
		"interpolated": cfg.InterpolatedDir,
	}
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		dir := r.URL.Query().Get("dir")
		name := r.URL.Query().Get("name")

		dirPath, ok := allowed[dir]
		if !ok {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid dir"})
			return
		}
		if !files.SafeVideoFilename(name, cfg.VideoExts) {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid filename"})
			return
		}

		fullPath := filepath.Join(dirPath, name)
		f, err := os.Open(fullPath)
		if err != nil {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "file not found"})
			return
		}
		defer f.Close()

		info, err := f.Stat()
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to stat file"})
			return
		}

		w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, name))
		w.Header().Set("Content-Type", "application/octet-stream")
		w.Header().Set("Content-Length", fmt.Sprintf("%d", info.Size()))
		io.Copy(w, f)
	}
}

// GET /api/files?dir=input&refresh=true
func handleFiles(cfg config.Config) http.HandlerFunc {
	cachePath := cache.CachePath(cfg)

	// Map dir query param to cache label
	dirToLabel := map[string]string{
		"input":        "input",
		"output":       "output",
		"optimized":    "optimize",
		"interpolated": "interpolated",
	}

	return func(w http.ResponseWriter, req *http.Request) {
		if req.Method == http.MethodDelete {
			var body struct {
				Items []files.DeleteItem `json:"items"`
			}
			if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON body"})
				return
			}
			if len(body.Items) == 0 {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": "no items to delete"})
				return
			}
			deleted, errs := files.DeleteFiles(body.Items, cfg.InputDir, cfg.OutputDir, cfg.OptimizedDir, cfg.InterpolatedDir, cfg.VideoExts)
			// Invalidate cache after deletion
			if deleted > 0 {
				if err := cache.BuildFileStatusCache(cfg); err != nil {
					fmt.Printf("Warning: cache rebuild after delete failed: %v\n", err)
				}
			}
			if errs == nil {
				errs = []string{}
			}
			writeJSON(w, http.StatusOK, map[string]interface{}{
				"deleted": deleted,
				"errors":  errs,
			})
			return
		}

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
			"input":        cfg.InputDir,
			"output":       cfg.OutputDir,
			"optimized":    cfg.OptimizedDir,
			"interpolated": cfg.InterpolatedDir,
		}

		fullPath, ok := allowed[dir]
		if !ok {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid dir: must be input, output, optimized, or interpolated"})
			return
		}

		// Rebuild cache if refresh requested
		if forceRefresh {
			if err := cache.BuildFileStatusCache(cfg); err != nil {
				writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "cache rebuild failed: " + err.Error()})
				return
			}
		}

		// Auto-refresh cache if older than 10 minutes
		if !forceRefresh {
			info, err := os.Stat(cachePath)
			if err != nil || time.Since(info.ModTime()) > 10*time.Minute {
				if err := cache.BuildFileStatusCache(cfg); err != nil {
					fmt.Printf("Warning: auto cache refresh failed: %v\n", err)
				}
			}
		}

		var videoFiles []files.VideoFile
		var err error
		switch dir {
		case "input":
			videoFiles, err = files.ListVideosWithStatus(fullPath, cfg.OutputDir, cfg.OptimizedDir, cfg.InterpolatedDir, cfg.VideoExts)
		case "output":
			videoFiles, err = files.ListOutputWithStatus(fullPath, cfg.InputDir, cfg.OptimizedDir, cfg.VideoExts)
		case "optimized":
			videoFiles, err = files.ListOptimizedWithStatus(fullPath, cfg.InputDir, cfg.OutputDir, cfg.VideoExts)
		case "interpolated":
			videoFiles, err = files.ListInterpolatedWithStatus(fullPath, cfg.InputDir, cfg.VideoExts)
		default:
			videoFiles, err = files.ListVideosWithSize(fullPath, cfg.VideoExts)
		}
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		if videoFiles == nil {
			videoFiles = []files.VideoFile{}
		}

		// Enrich with resolution data from cache JSON
		if len(videoFiles) > 0 {
			cached := cache.LoadCache(cachePath)
			primaryLabel := dirToLabel[dir]

			for i, f := range videoFiles {
				status, exists := cached[f.Name]
				if !exists {
					continue
				}

				// Primary dir resolution
				var primary *cache.SourceEntry
				switch primaryLabel {
				case "input":
					primary = status.Input
				case "output":
					primary = status.Output
				case "optimize":
					primary = status.Optimize
				case "interpolated":
					primary = status.Interpolated
				}
				if primary != nil {
					videoFiles[i].Width = primary.Width
					videoFiles[i].Height = primary.Height
					videoFiles[i].Audio = primary.Audio
					videoFiles[i].Subtitles = primary.Subtitles
				}

				// Cross-dir resolutions + tracks
				if dir != "output" && status.Output != nil {
					videoFiles[i].UpscaledWidth = status.Output.Width
					videoFiles[i].UpscaledHeight = status.Output.Height
					videoFiles[i].UpscaledAudio = status.Output.Audio
					videoFiles[i].UpscaledSubtitles = status.Output.Subtitles
				}
				if dir != "optimized" && status.Optimize != nil {
					videoFiles[i].OptimizedWidth = status.Optimize.Width
					videoFiles[i].OptimizedHeight = status.Optimize.Height
					videoFiles[i].OptimizedAudio = status.Optimize.Audio
					videoFiles[i].OptimizedSubtitles = status.Optimize.Subtitles
				}
				if dir != "input" && status.Input != nil {
					videoFiles[i].InputWidth = status.Input.Width
					videoFiles[i].InputHeight = status.Input.Height
					videoFiles[i].InputAudio = status.Input.Audio
					videoFiles[i].InputSubtitles = status.Input.Subtitles
				}
				if dir != "interpolated" && status.Interpolated != nil {
					videoFiles[i].InterpolatedWidth = status.Interpolated.Width
					videoFiles[i].InterpolatedHeight = status.Interpolated.Height
					videoFiles[i].InterpolatedAudio = status.Interpolated.Audio
					videoFiles[i].InterpolatedSubtitles = status.Interpolated.Subtitles
				}
			}
		}

		resp := map[string]interface{}{
			"dir":   dir,
			"files": videoFiles,
		}
		if info, err := os.Stat(cachePath); err == nil {
			resp["cached_at"] = info.ModTime().UTC()
		}
		writeJSON(w, http.StatusOK, resp)
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
		Type        string   `json:"type"`
		Files       []string `json:"files"`
		Source      string   `json:"source"`
		Scale       int      `json:"scale"`
		Resolution  int      `json:"resolution"`
		Multiplier  int      `json:"multiplier"`
		Threads     int      `json:"threads"`
		RifeModel   string   `json:"rife_model"`
		SceneThresh float64  `json:"scene_thresh"`
		Processor   string   `json:"processor"`
		Model       string   `json:"model"`
		NoiseLevel  int      `json:"noise_level"`
		Quality     string   `json:"quality"`
		Codec       string   `json:"codec"`
		Preset      string   `json:"preset"`
		Tune        string   `json:"tune"`
		PixFmt      string   `json:"pix_fmt"`
		AudioCodec  string   `json:"audio_codec"`
		UseGPU      bool     `json:"use_gpu"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON body"})
		return
	}

	validTypes := map[string]bool{"upscale": true, "optimize": true, "check": true, "interpolate": true}
	if !validTypes[req.Type] {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "type must be upscale, optimize, check, or interpolate"})
		return
	}

	// Upscale validation
	if req.Scale == 0 {
		req.Scale = 2
	}
	if req.Type == "upscale" && req.Scale != 2 && req.Scale != 3 && req.Scale != 4 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "scale must be 2, 3, or 4"})
		return
	}
	if req.Type == "upscale" {
		if req.Processor != "" && !pipeline.ValidProcessors[req.Processor] {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid processor"})
			return
		}
		if req.Model != "" && !pipeline.ValidUpscaleModels[req.Model] {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid model"})
			return
		}
		if req.Model != "" && !pipeline.ValidModelScale(req.Model, req.Scale) {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": fmt.Sprintf("model %q does not support scale %d", req.Model, req.Scale)})
			return
		}
		if req.NoiseLevel < 0 || req.NoiseLevel > 3 {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "noise_level must be between 0 and 3"})
			return
		}
	}

	// Interpolate validation
	if req.Multiplier == 0 {
		req.Multiplier = 2
	}
	if req.Type == "interpolate" && req.Multiplier != 2 && req.Multiplier != 3 && req.Multiplier != 4 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "multiplier must be 2, 3, or 4"})
		return
	}
	if req.RifeModel == "" {
		req.RifeModel = "rife-v4.6"
	}
	if req.Type == "interpolate" && !pipeline.ValidRifeModels[req.RifeModel] {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid rife_model"})
		return
	}
	if req.SceneThresh == 0 {
		req.SceneThresh = 10.0
	}
	if req.Type == "interpolate" && (req.SceneThresh < 0 || req.SceneThresh > 100) {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "scene_thresh must be between 0 and 100"})
		return
	}

	// Optimize validation
	if req.Type == "optimize" {
		if req.Quality != "" {
			if _, ok := pipeline.QualityToCRF[req.Quality]; !ok {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": "quality must be ultra, alta, media, or baixa"})
				return
			}
		}
		if req.Codec != "" && !pipeline.ValidCodecs[req.Codec] {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid codec"})
			return
		}
		if req.Preset != "" && !pipeline.ValidPresets[req.Preset] {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid preset"})
			return
		}
		if req.Tune != "" && !pipeline.ValidTunes[req.Tune] {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid tune"})
			return
		}
		if req.PixFmt != "" && !pipeline.ValidPixFmts[req.PixFmt] {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid pix_fmt"})
			return
		}
		if req.AudioCodec != "" && !pipeline.ValidAudioCodecs[req.AudioCodec] {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid audio_codec"})
			return
		}
		if req.UseGPU {
			if jm.Config().GPUVendor == "" {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": "use_gpu requires gpu_vendor to be set in settings"})
				return
			}
			if req.Codec == "copy" || req.Codec == "libvpx-vp9" {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": "use_gpu is incompatible with codec=copy or libvpx-vp9"})
				return
			}
		}
	}

	// threads: 0 means auto (will use HalfCPUs at process level)
	if req.Threads < 0 {
		req.Threads = 0
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
	sourceDir, ok := resolveFolder(cfg, req.Source)
	if !ok {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid source (must be input, output, interpolated, or optimized)"})
		return
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
			if !files.SafeVideoFilename(f, cfg.VideoExts) {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": fmt.Sprintf("invalid filename: %s", f)})
				return
			}
			if !files.FileExists(filepath.Join(sourceDir, f)) {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": fmt.Sprintf("file not found in %s/: %s", req.Source, f)})
				return
			}
		}
	}

	job := jm.StartJob(StartJobParams{
		Type:        req.Type,
		Files:       req.Files,
		Source:      req.Source,
		SourceDir:   sourceDir,
		Scale:       req.Scale,
		Resolution:  req.Resolution,
		Multiplier:  req.Multiplier,
		Threads:     req.Threads,
		RifeModel:   req.RifeModel,
		SceneThresh: req.SceneThresh,
		Processor:   req.Processor,
		Model:       req.Model,
		NoiseLevel:  req.NoiseLevel,
		Quality:     req.Quality,
		Codec:       req.Codec,
		Preset:      req.Preset,
		Tune:        req.Tune,
		PixFmt:      req.PixFmt,
		AudioCodec:  req.AudioCodec,
		UseGPU:      req.UseGPU,
	})

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
		ID            string                  `json:"id"`
		Type          string                  `json:"type"`
		Status        string                  `json:"status"`
		Source        string                  `json:"source"`
		Scale         int                     `json:"scale"`
		Resolution    int                     `json:"resolution"`
		Multiplier    int                     `json:"multiplier,omitempty"`
		RifeModel     string                  `json:"rife_model,omitempty"`
		SceneThresh   float64                 `json:"scene_thresh,omitempty"`
		Threads       int                     `json:"threads,omitempty"`
		PipelineName  string                  `json:"pipeline_name,omitempty"`
		PipelineSteps []pipeline.PipelineStep `json:"pipeline_steps,omitempty"`
		Files         []string                `json:"files"`
		Progress      JobProgress             `json:"progress"`
		CreatedAt     time.Time               `json:"created_at"`
		FinishedAt    *time.Time              `json:"finished_at,omitempty"`
	}
	writeJSON(w, http.StatusOK, jobDetail{
		ID:            snap.ID,
		Type:          snap.Type,
		Status:        snap.Status,
		Source:        snap.Source,
		Scale:         snap.Scale,
		Resolution:    snap.Resolution,
		Multiplier:    snap.Multiplier,
		RifeModel:     snap.RifeModel,
		SceneThresh:   snap.SceneThresh,
		Threads:       snap.Threads,
		PipelineName:  snap.PipelineName,
		PipelineSteps: snap.PipelineSteps,
		Files:         snap.Files,
		Progress:      snap.Progress,
		CreatedAt:     snap.CreatedAt,
		FinishedAt:    snap.FinishedAt,
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

// resolveFolder maps a logical folder name (input/output/interpolated/optimized)
// to its absolute directory path from config. Returns ok=false for unknown names.
func resolveFolder(cfg config.Config, name string) (string, bool) {
	switch name {
	case "input":
		return cfg.InputDir, true
	case "output":
		return cfg.OutputDir, true
	case "optimized":
		return cfg.OptimizedDir, true
	case "interpolated":
		return cfg.InterpolatedDir, true
	}
	return "", false
}

// dirToSourceName is the inverse of resolveFolder: maps an absolute directory
// path back to its logical name. Returns "" for unknown paths.
func dirToSourceName(cfg config.Config, dir string) string {
	switch filepath.Clean(dir) {
	case filepath.Clean(cfg.InputDir):
		return "input"
	case filepath.Clean(cfg.OutputDir):
		return "output"
	case filepath.Clean(cfg.OptimizedDir):
		return "optimized"
	case filepath.Clean(cfg.InterpolatedDir):
		return "interpolated"
	}
	return ""
}
