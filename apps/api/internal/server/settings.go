package server

import (
	"encoding/json"
	"net/http"

	"anime-upscaling/internal/config"
	"anime-upscaling/internal/runner"
)

type settingsResponse struct {
	StreamsPerGPU int    `json:"streams_per_gpu"`
	FFmpegStreams int    `json:"ffmpeg_streams"`
	GPUCount      int    `json:"gpu_count"`
	GPUVendor     string `json:"gpu_vendor"`
	// SeedVR2Available says whether this image ships the SeedVR2 runtime;
	// the UI only offers the seedvr2 processor when it does.
	SeedVR2Available bool `json:"seedvr2_available"`
}

func handleSettings(jm *JobManager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			cfg := jm.Config()
			writeJSON(w, http.StatusOK, settingsResponse{
				StreamsPerGPU:    cfg.StreamsPerGPU,
				FFmpegStreams:    cfg.FFmpegStreams,
				GPUCount:         cfg.GPUCount,
				GPUVendor:        cfg.GPUVendor,
				SeedVR2Available: runner.SeedVR2Installed(cfg.SeedVR2Dir),
			})
		case http.MethodPut:
			var body struct {
				StreamsPerGPU int    `json:"streams_per_gpu"`
				FFmpegStreams int    `json:"ffmpeg_streams"`
				GPUVendor     string `json:"gpu_vendor"`
			}
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON body"})
				return
			}
			if body.StreamsPerGPU < 1 || body.FFmpegStreams < 1 {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": "streams_per_gpu and ffmpeg_streams must be >= 1"})
				return
			}
			if !config.ValidGPUVendors[body.GPUVendor] {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": "gpu_vendor must be one of: '', nvidia, amd, intel"})
				return
			}
			if jm.HasActiveJobs() {
				writeJSON(w, http.StatusConflict, map[string]string{"error": "há jobs em execução — aguarde terminarem antes de mudar settings"})
				return
			}
			cfg := jm.Config()
			if err := config.SaveSettings(cfg.BaseDir, config.Settings{
				StreamsPerGPU: body.StreamsPerGPU,
				FFmpegStreams: body.FFmpegStreams,
				GPUVendor:     body.GPUVendor,
			}); err != nil {
				writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to save settings: " + err.Error()})
				return
			}
			jm.ApplySettings(body.StreamsPerGPU, body.FFmpegStreams, body.GPUVendor)
			writeJSON(w, http.StatusOK, settingsResponse{
				StreamsPerGPU:    body.StreamsPerGPU,
				FFmpegStreams:    body.FFmpegStreams,
				GPUCount:         cfg.GPUCount,
				GPUVendor:        body.GPUVendor,
				SeedVR2Available: runner.SeedVR2Installed(cfg.SeedVR2Dir),
			})
		default:
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		}
	}
}
