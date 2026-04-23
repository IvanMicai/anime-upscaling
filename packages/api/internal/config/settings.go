package config

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sync"
)

type Settings struct {
	StreamsPerGPU int    `json:"streams_per_gpu"`
	FFmpegStreams int    `json:"ffmpeg_streams"`
	GPUVendor     string `json:"gpu_vendor,omitempty"`
}

// ValidGPUVendors lists allowed vendor values for GPU-accelerated FFmpeg.
// Empty string means "no GPU encoder available" — optimize jobs must run on CPU.
var ValidGPUVendors = map[string]bool{
	"":       true,
	"nvidia": true,
	"amd":    true,
	"intel":  true,
}

var settingsMu sync.Mutex

func settingsPath(baseDir string) string {
	return filepath.Join(baseDir, "settings.json")
}

func LoadSettings(baseDir string) (Settings, error) {
	settingsMu.Lock()
	defer settingsMu.Unlock()

	var s Settings
	b, err := os.ReadFile(settingsPath(baseDir))
	if err != nil {
		return s, err
	}
	if err := json.Unmarshal(b, &s); err != nil {
		return s, err
	}
	return s, nil
}

func SaveSettings(baseDir string, s Settings) error {
	if s.StreamsPerGPU < 1 || s.FFmpegStreams < 1 {
		return errors.New("streams_per_gpu and ffmpeg_streams must be >= 1")
	}
	if !ValidGPUVendors[s.GPUVendor] {
		return errors.New("gpu_vendor must be one of: '', nvidia, amd, intel")
	}

	settingsMu.Lock()
	defer settingsMu.Unlock()

	if err := os.MkdirAll(baseDir, 0755); err != nil {
		return err
	}
	b, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	tmp := settingsPath(baseDir) + ".tmp"
	if err := os.WriteFile(tmp, b, 0644); err != nil {
		return err
	}
	return os.Rename(tmp, settingsPath(baseDir))
}
