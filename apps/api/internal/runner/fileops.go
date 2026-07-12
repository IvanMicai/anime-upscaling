package runner

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

type FileInfo struct {
	Name string `json:"name"`
	Size int64  `json:"size"`
}

// ListFiles lists files in a directory, filtering by extension.
func (r *Runner) ListFiles(hostPath string, exts []string) ([]FileInfo, error) {
	entries, err := os.ReadDir(hostPath)
	if err != nil {
		return nil, fmt.Errorf("list files: %w", err)
	}

	extSet := make(map[string]bool, len(exts))
	for _, e := range exts {
		extSet[strings.ToLower(e)] = true
	}

	var files []FileInfo
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		ext := strings.ToLower(filepath.Ext(name))
		if !extSet[ext] {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			continue
		}
		files = append(files, FileInfo{Name: name, Size: info.Size()})
	}
	return files, nil
}

// CopyFile copies a single file between two directories and fixes ownership.
func (r *Runner) CopyFile(srcDir, dstDir, filename string) error {
	srcPath := filepath.Join(srcDir, filename)
	dstPath := filepath.Join(dstDir, filename)

	src, err := os.Open(srcPath)
	if err != nil {
		return fmt.Errorf("copy %s: %w", filename, err)
	}
	defer src.Close()

	dst, err := os.Create(dstPath)
	if err != nil {
		return fmt.Errorf("copy %s: %w", filename, err)
	}
	defer dst.Close()

	if _, err := io.Copy(dst, src); err != nil {
		return fmt.Errorf("copy %s: %w", filename, err)
	}

	return os.Chown(dstPath, r.cfg.UserID, r.cfg.GroupID)
}

// CopyFiles copies multiple files. Individual failures don't stop the rest.
// Returns the count of successes.
func (r *Runner) CopyFiles(srcDir, dstDir string, filenames []string) (int, error) {
	if len(filenames) == 0 {
		return 0, nil
	}

	copied := 0
	for _, f := range filenames {
		if err := r.CopyFile(srcDir, dstDir, f); err != nil {
			continue
		}
		copied++
	}
	return copied, nil
}

// PathExists checks if a path exists and is a directory.
func (r *Runner) PathExists(hostPath string) (bool, error) {
	info, err := os.Stat(hostPath)
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, nil
	}
	return info.IsDir(), nil
}
