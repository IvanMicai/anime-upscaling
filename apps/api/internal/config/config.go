package config

import (
	"os"
	"runtime"
	"strconv"
	"strings"
)

type Config struct {
	Port            string
	BaseDir         string
	InputDir        string
	OutputDir       string
	OptimizedDir    string
	InterpolatedDir string
	MergedDir       string
	TempDir         string
	LogFile         string
	UserID          int
	GroupID         int
	NumCPUs         int
	HalfCPUs        int
	VideoExts       []string
	Video2xBin      string
	FFmpegBin       string
	FFprobeBin      string

	GPUCount      int
	StreamsPerGPU int
	FFmpegStreams int
	GPUVendor     string // "" | "nvidia" | "amd" | "intel"

	// SeedVR2 runtime (only present in the image built with --target seedvr2).
	SeedVR2Dir      string // venv + CLI checkout (SEEDVR2_DIR)
	SeedVR2ModelDir string // downloaded weights, kept across restarts
	// Memory tuning; 0 / -1 keep the per-model defaults.
	SeedVR2BatchSize    int      // SEEDVR2_BATCH_SIZE, 4n+1
	SeedVR2BlocksToSwap int      // SEEDVR2_BLOCKS_TO_SWAP, -1 = default
	SeedVR2ExtraArgs    []string // SEEDVR2_EXTRA_ARGS, appended to the CLI
}

func NewConfig() Config {
	baseDir := os.Getenv("PROCESS_DIR")
	if baseDir == "" {
		baseDir = "/data"
	}
	numCPUs := runtime.NumCPU()
	halfCPUs := numCPUs / 2
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
		MergedDir:       baseDir + "/merged",
		TempDir:         baseDir + "/temp",
		LogFile:         baseDir + "/process.log",
		UserID:          os.Getuid(),
		GroupID:         os.Getgid(),
		NumCPUs:         numCPUs,
		HalfCPUs:        halfCPUs,
		VideoExts:       []string{".mkv", ".mp4", ".avi"},
		Video2xBin:      "video2x",
		FFmpegBin:       "ffmpeg",
		FFprobeBin:      "ffprobe",
		GPUCount:        envInt("GPU_COUNT", 2),
		StreamsPerGPU:   envInt("STREAMS_PER_GPU", 1),
		FFmpegStreams:   envInt("FFMPEG_STREAMS", 1),
		GPUVendor:       envVendor("GPU_VENDOR", ""),

		SeedVR2Dir:          envString("SEEDVR2_DIR", "/opt/seedvr2"),
		SeedVR2ModelDir:     baseDir + "/models/seedvr2",
		SeedVR2BatchSize:    envSeedVR2BatchSize("SEEDVR2_BATCH_SIZE"),
		SeedVR2BlocksToSwap: envIntMin("SEEDVR2_BLOCKS_TO_SWAP", -1, 0),
		SeedVR2ExtraArgs:    strings.Fields(os.Getenv("SEEDVR2_EXTRA_ARGS")),
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

func envString(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

// envIntMin reads an int that may be as low as minVal (unlike envInt, which
// requires >= 1); anything unset, unparsable or below minVal yields def.
func envIntMin(key string, def, minVal int) int {
	n, err := strconv.Atoi(os.Getenv(key))
	if err != nil || n < minVal {
		return def
	}
	return n
}

// envSeedVR2BatchSize reads SEEDVR2_BATCH_SIZE, which the CLI requires to be
// 4n+1; any other value yields 0 (use the default).
func envSeedVR2BatchSize(key string) int {
	n := envIntMin(key, 0, 1)
	if n%4 != 1 {
		return 0
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

// StageFolders is the ordered list of stage folder names, as used in the API
// (`dir=`, `source=`, delete `folders`).
var StageFolders = []string{"input", "merged", "output", "interpolated", "optimized"}

// StageDirs maps every stage folder name to its directory. It is the single
// place that knows the set of stages: handlers that used to spell the folders
// out one by one had to be found and edited for each new stage.
func (c Config) StageDirs() map[string]string {
	return map[string]string{
		"input":        c.InputDir,
		"merged":       c.MergedDir,
		"output":       c.OutputDir,
		"interpolated": c.InterpolatedDir,
		"optimized":    c.OptimizedDir,
	}
}
