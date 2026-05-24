package files

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"anime-upscaling/internal/runner"
)

type VideoFile struct {
	Name               string `json:"name"`
	Size               int64  `json:"size"`
	Width              int    `json:"width,omitempty"`
	Height             int    `json:"height,omitempty"`
	HasUpscaled        bool   `json:"has_upscaled,omitempty"`
	HasOptimized       bool   `json:"has_optimized,omitempty"`
	HasInput           bool   `json:"has_input,omitempty"`
	HasInterpolated    bool   `json:"has_interpolated,omitempty"`
	UpscaledSize       int64  `json:"upscaled_size,omitempty"`
	OptimizedSize      int64  `json:"optimized_size,omitempty"`
	InputSize          int64  `json:"input_size,omitempty"`
	InterpolatedSize   int64  `json:"interpolated_size,omitempty"`
	UpscaledWidth      int    `json:"upscaled_width,omitempty"`
	UpscaledHeight     int    `json:"upscaled_height,omitempty"`
	OptimizedWidth     int    `json:"optimized_width,omitempty"`
	OptimizedHeight    int    `json:"optimized_height,omitempty"`
	InputWidth         int    `json:"input_width,omitempty"`
	InputHeight        int    `json:"input_height,omitempty"`
	InterpolatedWidth  int    `json:"interpolated_width,omitempty"`
	InterpolatedHeight int    `json:"interpolated_height,omitempty"`

	FrameRate             float64 `json:"frame_rate,omitempty"`
	InputFrameRate        float64 `json:"input_frame_rate,omitempty"`
	UpscaledFrameRate     float64 `json:"upscaled_frame_rate,omitempty"`
	OptimizedFrameRate    float64 `json:"optimized_frame_rate,omitempty"`
	InterpolatedFrameRate float64 `json:"interpolated_frame_rate,omitempty"`

	Audio                 []runner.AudioTrack    `json:"audio,omitempty"`
	Subtitles             []runner.SubtitleTrack `json:"subtitles,omitempty"`
	InputAudio            []runner.AudioTrack    `json:"input_audio,omitempty"`
	InputSubtitles        []runner.SubtitleTrack `json:"input_subtitles,omitempty"`
	UpscaledAudio         []runner.AudioTrack    `json:"upscaled_audio,omitempty"`
	UpscaledSubtitles     []runner.SubtitleTrack `json:"upscaled_subtitles,omitempty"`
	OptimizedAudio        []runner.AudioTrack    `json:"optimized_audio,omitempty"`
	OptimizedSubtitles    []runner.SubtitleTrack `json:"optimized_subtitles,omitempty"`
	InterpolatedAudio     []runner.AudioTrack    `json:"interpolated_audio,omitempty"`
	InterpolatedSubtitles []runner.SubtitleTrack `json:"interpolated_subtitles,omitempty"`
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

// ListVideosWithSize lists video files in baseDir/subPath and returns the
// subdirectory names found at that level. If the directory does not exist,
// returns empty slices and no error.
func ListVideosWithSize(baseDir, subPath string, exts []string) ([]VideoFile, []string, error) {
	dir := filepath.Join(baseDir, subPath)
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil, nil
		}
		return nil, nil, err
	}
	extSet := make(map[string]bool, len(exts))
	for _, e := range exts {
		extSet[strings.ToLower(e)] = true
	}
	var vfiles []VideoFile
	var dirs []string
	for _, entry := range entries {
		if entry.IsDir() {
			dirs = append(dirs, entry.Name())
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
	return vfiles, dirs, nil
}

// WalkVideos recursively walks baseDir and returns all video files keyed by
// their relative path (e.g., "season1/ep01.mkv") with size info.
func WalkVideos(baseDir string, exts []string) (map[string]int64, error) {
	out := make(map[string]int64)
	extSet := make(map[string]bool, len(exts))
	for _, e := range exts {
		extSet[strings.ToLower(e)] = true
	}
	err := filepath.WalkDir(baseDir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			if os.IsNotExist(err) {
				return nil
			}
			return err
		}
		if d.IsDir() {
			return nil
		}
		ext := strings.ToLower(filepath.Ext(d.Name()))
		if !extSet[ext] {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return nil
		}
		rel, err := filepath.Rel(baseDir, path)
		if err != nil {
			return nil
		}
		// Always use forward slashes for relative paths so JSON/keys are stable.
		rel = filepath.ToSlash(rel)
		out[rel] = info.Size()
		return nil
	})
	if err != nil && !os.IsNotExist(err) {
		return out, err
	}
	return out, nil
}

// ListAllWithStatus reads every base dir at subPath and returns the union of
// video files and subdirectories. For each file, has_* and *_size are set for
// every base where the file exists. Name and the primary Size mirror the
// `primary` base when the file is present there; otherwise Size stays 0 and
// the active tab's column renders as missing.
func ListAllWithStatus(primary string, baseDirs map[string]string, subPath string, exts []string) ([]VideoFile, []string, error) {
	extSet := make(map[string]bool, len(exts))
	for _, e := range exts {
		extSet[strings.ToLower(e)] = true
	}

	dirSet := make(map[string]struct{})
	fileMap := make(map[string]*VideoFile)

	for label, base := range baseDirs {
		entries, err := os.ReadDir(filepath.Join(base, subPath))
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return nil, nil, err
		}
		for _, entry := range entries {
			if entry.IsDir() {
				dirSet[entry.Name()] = struct{}{}
				continue
			}
			ext := strings.ToLower(filepath.Ext(entry.Name()))
			if !extSet[ext] {
				continue
			}
			info, err := entry.Info()
			if err != nil {
				continue
			}
			vf, ok := fileMap[entry.Name()]
			if !ok {
				vf = &VideoFile{Name: entry.Name()}
				fileMap[entry.Name()] = vf
			}
			size := info.Size()
			switch label {
			case "input":
				vf.HasInput = true
				vf.InputSize = size
			case "output":
				vf.HasUpscaled = true
				vf.UpscaledSize = size
			case "optimized":
				vf.HasOptimized = true
				vf.OptimizedSize = size
			case "interpolated":
				vf.HasInterpolated = true
				vf.InterpolatedSize = size
			}
			if label == primary {
				vf.Size = size
			}
		}
	}

	dirs := make([]string, 0, len(dirSet))
	for d := range dirSet {
		dirs = append(dirs, d)
	}
	SortNatural(dirs)

	vfiles := make([]VideoFile, 0, len(fileMap))
	for _, vf := range fileMap {
		vfiles = append(vfiles, *vf)
	}
	sort.Slice(vfiles, func(i, j int) bool { return NaturalLess(vfiles[i].Name, vfiles[j].Name) })

	return vfiles, dirs, nil
}

type DeleteItem struct {
	Name    string   `json:"name"`
	Path    string   `json:"path,omitempty"`
	Folders []string `json:"folders"`
}

func DeleteFiles(items []DeleteItem, inputDir, outputDir, optimizedDir, interpolatedDir string, exts []string) (int, []string) {
	folderDirs := map[string]string{
		"input":        inputDir,
		"output":       outputDir,
		"optimized":    optimizedDir,
		"interpolated": interpolatedDir,
	}

	deleted := 0
	var errors []string

	for _, item := range items {
		if !SafeVideoFilename(item.Name, exts) {
			errors = append(errors, fmt.Sprintf("invalid filename %q", item.Name))
			continue
		}
		if item.Path != "" && !SafeRelDir(item.Path) {
			errors = append(errors, fmt.Sprintf("invalid path %q for %s", item.Path, item.Name))
			continue
		}
		for _, folder := range item.Folders {
			dir, ok := folderDirs[folder]
			if !ok {
				errors = append(errors, fmt.Sprintf("invalid folder %q for %s", folder, item.Name))
				continue
			}
			path := filepath.Join(dir, item.Path, item.Name)
			if err := os.Remove(path); err != nil {
				errors = append(errors, fmt.Sprintf("failed to delete %s/%s: %v", folder, filepath.Join(item.Path, item.Name), err))
			} else {
				deleted++
			}
		}
	}

	return deleted, errors
}

func FileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func SafeVideoFilename(name string, exts []string) bool {
	if name == "" || strings.Contains(name, "/") || strings.Contains(name, "\\") || strings.Contains(name, "..") {
		return false
	}
	if filepath.Base(name) != name {
		return false
	}
	if len(exts) == 0 {
		return true
	}
	ext := strings.ToLower(filepath.Ext(name))
	for _, allowed := range exts {
		if ext == strings.ToLower(allowed) {
			return true
		}
	}
	return false
}

// SafeRelDir validates a relative directory path used for navigation.
// Allows empty (root), single segment "season1", multi-segment "season1/specials".
// Rejects absolute paths, "..", backslashes, empty segments.
func SafeRelDir(rel string) bool {
	if rel == "" {
		return true
	}
	if strings.Contains(rel, "\\") {
		return false
	}
	if strings.HasPrefix(rel, "/") {
		return false
	}
	for _, seg := range strings.Split(rel, "/") {
		if seg == "" || seg == "." || seg == ".." {
			return false
		}
	}
	cleaned := filepath.ToSlash(filepath.Clean(rel))
	return cleaned == rel
}

// SafeVideoRelPath validates a relative video path. The last segment must be
// a valid filename matching one of the allowed extensions; preceding segments
// must form a valid relative directory.
func SafeVideoRelPath(rel string, exts []string) bool {
	if rel == "" {
		return false
	}
	if strings.Contains(rel, "\\") || strings.HasPrefix(rel, "/") {
		return false
	}
	segs := strings.Split(rel, "/")
	for _, seg := range segs {
		if seg == "" || seg == "." || seg == ".." {
			return false
		}
	}
	cleaned := filepath.ToSlash(filepath.Clean(rel))
	if cleaned != rel {
		return false
	}
	name := segs[len(segs)-1]
	return SafeVideoFilename(name, exts)
}
