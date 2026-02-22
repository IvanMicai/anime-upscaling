package files

import (
	"os"
	"path/filepath"
	"strings"
)

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

func FileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
