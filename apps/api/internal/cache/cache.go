package cache

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"time"

	"anime-upscaling/internal/config"
	"anime-upscaling/internal/files"
	"anime-upscaling/internal/runner"
)

type SourceEntry struct {
	Size      int64                  `json:"size"`
	Width     int                    `json:"width"`
	Height    int                    `json:"height"`
	FrameRate float64                `json:"frame_rate,omitempty"`
	Audio     []runner.AudioTrack    `json:"audio,omitempty"`
	Subtitles []runner.SubtitleTrack `json:"subtitles,omitempty"`
	Sync      *runner.SyncInfo       `json:"sync,omitempty"`
}

// Bumped to 5: entries gained Sync and the merged stage.
const currentCacheVersion = 5

type cacheEnvelope struct {
	Version int       `json:"version"`
	Data    CacheData `json:"data"`
}

type FileStatus struct {
	Input        *SourceEntry `json:"input"`
	Output       *SourceEntry `json:"output"`
	Optimize     *SourceEntry `json:"optimize"`
	Interpolated *SourceEntry `json:"interpolated,omitempty"`
	Merged       *SourceEntry `json:"merged,omitempty"`
}

// Slot returns the entry for a cache label ("input", "output", "optimize",
// "interpolated", "merged"), or nil for an unknown label. Going through it keeps
// the set of stages in ONE place: the builder used to repeat a four-way switch
// in four spots, and every new stage meant finding them all.
func (s *FileStatus) Slot(label string) **SourceEntry {
	switch label {
	case "input":
		return &s.Input
	case "output":
		return &s.Output
	case "optimize":
		return &s.Optimize
	case "interpolated":
		return &s.Interpolated
	case "merged":
		return &s.Merged
	}
	return nil
}

type CacheData map[string]FileStatus

func CachePath(cfg config.Config) string {
	return cfg.BaseDir + "/cache-file-status.json"
}

func LoadCache(path string) CacheData {
	data, err := os.ReadFile(path)
	if err != nil {
		return make(CacheData)
	}
	// Try versioned envelope first
	var env cacheEnvelope
	if err := json.Unmarshal(data, &env); err == nil && env.Version == currentCacheVersion && env.Data != nil {
		return env.Data
	}
	// Version mismatch or legacy format — return empty to force rebuild
	return make(CacheData)
}

func saveCache(path string, cache CacheData) error {
	env := cacheEnvelope{Version: currentCacheVersion, Data: cache}
	data, err := json.MarshalIndent(env, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal cache: %w", err)
	}
	return os.WriteFile(path, data, 0644)
}

func BuildFileStatusCache(cfg config.Config) error {
	fmt.Println("Building file status cache...")
	start := time.Now()

	path := CachePath(cfg)
	old := LoadCache(path)

	type dirInfo struct {
		label string
		dir   string
	}
	dirs := []dirInfo{
		{"input", cfg.InputDir},
		{"output", cfg.OutputDir},
		{"optimize", cfg.OptimizedDir},
		{"interpolated", cfg.InterpolatedDir},
		{"merged", cfg.MergedDir},
	}

	// Scan all directories recursively and index by map for O(1) lookup.
	// Keys are relative paths from the base dir (forward-slash separated).
	scannedIndex := make(map[string]map[string]int64) // label -> relPath -> size
	for _, d := range dirs {
		idx, err := files.WalkVideos(d.dir, cfg.VideoExts)
		if err != nil {
			idx = make(map[string]int64)
		}
		scannedIndex[d.label] = idx
	}

	// Collect all unique filenames
	allNames := make(map[string]bool)
	for _, idx := range scannedIndex {
		for name := range idx {
			allNames[name] = true
		}
	}

	// Build new cache and find files needing ffprobe
	newCache := make(CacheData)
	needProbe := make(map[string][]string) // label -> filenames

	for name := range allNames {
		var status FileStatus
		oldStatus := old[name]

		for _, d := range dirs {
			size, found := scannedIndex[d.label][name]
			if !found {
				continue
			}

			slot := status.Slot(d.label)
			if old := oldStatus.Slot(d.label); old != nil && *old != nil && (*old).Size == size {
				// Size matches — reuse the cached metadata.
				entry := **old
				*slot = &entry
			} else {
				// New or changed — needs ffprobe.
				needProbe[d.label] = append(needProbe[d.label], name)
				*slot = &SourceEntry{Size: size}
			}
		}

		newCache[name] = status
	}

	// Count files to probe
	totalProbe := 0
	for _, names := range needProbe {
		totalProbe += len(names)
	}

	if totalProbe == 0 {
		fmt.Println("All files cached, no probing needed")
	} else {
		fmt.Printf("Probing %d file(s) with ffprobe...\n", totalProbe)

		r := runner.NewRunner(cfg)
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
		defer cancel()

		mounts := make([]runner.DirMount, 0, len(dirs))
		for _, d := range dirs {
			if len(needProbe[d.label]) > 0 {
				mounts = append(mounts, runner.DirMount{Label: d.label, HostDir: d.dir})
			}
		}

		results, err := r.FFprobeBatchFullMetadataParallel(ctx, mounts, needProbe)
		if err != nil {
			return fmt.Errorf("ffprobe batch: %w", err)
		}

		// Merge probe results into cache
		for label, resMap := range results {
			for name, res := range resMap {
				status := newCache[name]
				if slot := status.Slot(label); slot != nil && *slot != nil {
					e := *slot
					e.Width, e.Height, e.FrameRate = res.Width, res.Height, res.FrameRate
					e.Audio, e.Subtitles, e.Sync = res.Audio, res.Subtitles, res.Sync
				}
				newCache[name] = status
			}
		}
	}

	if err := saveCache(path, newCache); err != nil {
		return fmt.Errorf("save cache: %w", err)
	}

	fmt.Printf("Cache ready (%d files, %s)\n", len(newCache), time.Since(start).Round(time.Millisecond))
	return nil
}
