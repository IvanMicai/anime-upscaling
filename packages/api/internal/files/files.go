package files

import (
	"os"
	"path/filepath"
	"strings"
)

type VideoFile struct {
	Name            string `json:"name"`
	Size            int64  `json:"size"`
	Width           int    `json:"width,omitempty"`
	Height          int    `json:"height,omitempty"`
	HasUpscaled     bool   `json:"has_upscaled,omitempty"`
	HasOptimized    bool   `json:"has_optimized,omitempty"`
	HasInput        bool   `json:"has_input,omitempty"`
	UpscaledSize    int64  `json:"upscaled_size,omitempty"`
	OptimizedSize   int64  `json:"optimized_size,omitempty"`
	InputSize       int64  `json:"input_size,omitempty"`
	UpscaledWidth   int    `json:"upscaled_width,omitempty"`
	UpscaledHeight  int    `json:"upscaled_height,omitempty"`
	OptimizedWidth  int    `json:"optimized_width,omitempty"`
	OptimizedHeight int    `json:"optimized_height,omitempty"`
	InputWidth      int    `json:"input_width,omitempty"`
	InputHeight     int    `json:"input_height,omitempty"`
}

func ListVideos(dir string, exts []string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	extSet := make(map[string]bool, len(exts))
	for _, e := range exts {
		extSet[strings.ToLower(e)] = true
	}
	var files []string
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		ext := strings.ToLower(filepath.Ext(entry.Name()))
		if extSet[ext] {
			files = append(files, entry.Name())
		}
	}
	return files, nil
}

func ListVideosWithSize(dir string, exts []string) ([]VideoFile, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	extSet := make(map[string]bool, len(exts))
	for _, e := range exts {
		extSet[strings.ToLower(e)] = true
	}
	var vfiles []VideoFile
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		ext := strings.ToLower(filepath.Ext(entry.Name()))
		if extSet[ext] {
			info, err := entry.Info()
			if err != nil {
				continue
			}
			vfiles = append(vfiles, VideoFile{
				Name: entry.Name(),
				Size: info.Size(),
			})
		}
	}
	return vfiles, nil
}

func ListVideosWithStatus(dir, outputDir, optimizedDir string, exts []string) ([]VideoFile, error) {
	vfiles, err := ListVideosWithSize(dir, exts)
	if err != nil {
		return nil, err
	}
	for i, f := range vfiles {
		if info, err := os.Stat(filepath.Join(outputDir, f.Name)); err == nil {
			vfiles[i].HasUpscaled = true
			vfiles[i].UpscaledSize = info.Size()
		}
		if info, err := os.Stat(filepath.Join(optimizedDir, f.Name)); err == nil {
			vfiles[i].HasOptimized = true
			vfiles[i].OptimizedSize = info.Size()
		}
	}
	return vfiles, nil
}

func ListOutputWithStatus(dir, optimizedDir string, exts []string) ([]VideoFile, error) {
	vfiles, err := ListVideosWithSize(dir, exts)
	if err != nil {
		return nil, err
	}
	for i, f := range vfiles {
		if info, err := os.Stat(filepath.Join(optimizedDir, f.Name)); err == nil {
			vfiles[i].HasOptimized = true
			vfiles[i].OptimizedSize = info.Size()
		}
	}
	return vfiles, nil
}

func ListOptimizedWithStatus(dir, inputDir, outputDir string, exts []string) ([]VideoFile, error) {
	vfiles, err := ListVideosWithSize(dir, exts)
	if err != nil {
		return nil, err
	}
	for i, f := range vfiles {
		if info, err := os.Stat(filepath.Join(inputDir, f.Name)); err == nil {
			vfiles[i].HasInput = true
			vfiles[i].InputSize = info.Size()
		}
		if info, err := os.Stat(filepath.Join(outputDir, f.Name)); err == nil {
			vfiles[i].HasUpscaled = true
			vfiles[i].UpscaledSize = info.Size()
		}
	}
	return vfiles, nil
}

func FileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
