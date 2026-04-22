package server

import (
	"encoding/json"
	"net/http"

	"anime-upscaling/internal/config"
)

type settingsResponse struct {
	StreamsPerGPU int `json:"streams_per_gpu"`
	FFmpegStreams int `json:"ffmpeg_streams"`
	GPUCount      int `json:"gpu_count"`
}

func handleSettings(jm *JobManager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			cfg := jm.Config()
			writeJSON(w, http.StatusOK, settingsResponse{
				StreamsPerGPU: cfg.StreamsPerGPU,
				FFmpegStreams: cfg.FFmpegStreams,
				GPUCount:      cfg.GPUCount,
			})
		case http.MethodPut:
			var body struct {
				StreamsPerGPU int `json:"streams_per_gpu"`
				FFmpegStreams int `json:"ffmpeg_streams"`
			}
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON body"})
				return
			}
			if body.StreamsPerGPU < 1 || body.FFmpegStreams < 1 {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": "streams_per_gpu and ffmpeg_streams must be >= 1"})
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
			}); err != nil {
				writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to save settings: " + err.Error()})
				return
			}
			jm.ApplySettings(body.StreamsPerGPU, body.FFmpegStreams)
			writeJSON(w, http.StatusOK, settingsResponse{
				StreamsPerGPU: body.StreamsPerGPU,
				FFmpegStreams: body.FFmpegStreams,
				GPUCount:      cfg.GPUCount,
			})
		default:
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		}
	}
}
