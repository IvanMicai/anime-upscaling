package config

import (
	"os"
	"runtime"
)

type Config struct {
	Port         string
	BaseDir      string
	InputDir     string
	OutputDir    string
	OptimizedDir string
	LogFile      string
	UserID       int
	GroupID      int
	HalfCPUs     int
	VideoExts    []string
	Video2xImage string
	FFmpegImage  string
	AlpineImage  string
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

	return Config{
		Port:    port,
		BaseDir:      baseDir,
		InputDir:     baseDir + "/input",
		OutputDir:    baseDir + "/output",
		OptimizedDir: baseDir + "/optimized",
		LogFile:      baseDir + "/process.log",
		UserID:       os.Getuid(),
		GroupID:      os.Getgid(),
		HalfCPUs:     halfCPUs,
		VideoExts:    []string{".mkv", ".mp4", ".avi"},
		Video2xImage: "ghcr.io/k4yt3x/video2x:6.4.0",
		FFmpegImage:  "linuxserver/ffmpeg",
		AlpineImage:  "alpine",
	}
}
