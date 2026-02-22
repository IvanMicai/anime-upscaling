package files

import (
	"os"
	"path/filepath"
	"strings"
)

type VideoFile struct {
	Name         string `json:"name"`
	Size         int64  `json:"size"`
	HasUpscaled  bool   `json:"has_upscaled,omitempty"`
	HasOptimized bool   `json:"has_optimized,omitempty"`
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
		if FileExists(filepath.Join(outputDir, f.Name)) {
			vfiles[i].HasUpscaled = true
		}
		if FileExists(filepath.Join(optimizedDir, f.Name)) {
			vfiles[i].HasOptimized = true
		}
	}
	return vfiles, nil
}

func FileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
