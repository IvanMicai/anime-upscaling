package config

import (
	"os"
	"runtime"
	"strconv"
)

type Config struct {
	Port            string
	BaseDir         string
	InputDir        string
	OutputDir       string
	OptimizedDir    string
	InterpolatedDir string
	TempDir         string
	LogFile         string
	UserID          int
	GroupID         int
	HalfCPUs        int
	VideoExts       []string
	Video2xBin      string
	FFmpegBin       string
	FFprobeBin      string

	GPUCount      int
	StreamsPerGPU int
	FFmpegStreams int
	GPUVendor     string // "" | "nvidia" | "amd" | "intel"
}

func NewConfig() Config {
	baseDir := os.Getenv("PROCESS_DIR")
	if baseDir == "" {
		baseDir = "/mnt/SSD2/process"
	}
	halfCPUs := runtime.NumCPU() / 2
	if halfCPUs < 1 {
		halfCPUs = 1
	}

	port := os.Getenv("API_PORT")
	if port == "" {
		port = "4751"
	}

	cfg := Config{
		Port:            port,
		BaseDir:         baseDir,
		InputDir:        baseDir + "/input",
		OutputDir:       baseDir + "/output",
		OptimizedDir:    baseDir + "/optimized",
		InterpolatedDir: baseDir + "/interpolated",
		TempDir:         baseDir + "/temp",
		LogFile:         baseDir + "/process.log",
		UserID:          os.Getuid(),
		GroupID:         os.Getgid(),
		HalfCPUs:        halfCPUs,
		VideoExts:       []string{".mkv", ".mp4", ".avi"},
		Video2xBin:      "video2x",
		FFmpegBin:       "ffmpeg",
		FFprobeBin:      "ffprobe",
		GPUCount:        envInt("GPU_COUNT", 2),
		StreamsPerGPU:   envInt("STREAMS_PER_GPU", 1),
		FFmpegStreams:   envInt("FFMPEG_STREAMS", 1),
		GPUVendor:       envVendor("GPU_VENDOR", ""),
	}

	// Overlay persisted runtime settings over env/defaults.
	if s, err := LoadSettings(baseDir); err == nil {
		if s.StreamsPerGPU >= 1 {
			cfg.StreamsPerGPU = s.StreamsPerGPU
		}
		if s.FFmpegStreams >= 1 {
			cfg.FFmpegStreams = s.FFmpegStreams
		}
		if ValidGPUVendors[s.GPUVendor] {
			cfg.GPUVendor = s.GPUVendor
		}
	}

	return cfg
}

func envInt(key string, def int) int {
	v := os.Getenv(key)
	if v == "" {
		return def
	}
	n, err := strconv.Atoi(v)
	if err != nil || n < 1 {
		return def
	}
	return n
}

func envVendor(key, def string) string {
	v := os.Getenv(key)
	if v == "" {
		return def
	}
	if !ValidGPUVendors[v] {
		return def
	}
	return v
}
