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
	Audio     []runner.AudioTrack    `json:"audio,omitempty"`
	Subtitles []runner.SubtitleTrack `json:"subtitles,omitempty"`
}

const currentCacheVersion = 2

type cacheEnvelope struct {
	Version int       `json:"version"`
	Data    CacheData `json:"data"`
}

type FileStatus struct {
	Input        *SourceEntry `json:"input"`
	Output       *SourceEntry `json:"output"`
	Optimize     *SourceEntry `json:"optimize"`
	Interpolated *SourceEntry `json:"interpolated,omitempty"`
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
	}

	// Scan all directories and index by map for O(1) lookup
	scannedIndex := make(map[string]map[string]int64) // label -> name -> size
	for _, d := range dirs {
		vfiles, err := files.ListVideosWithSize(d.dir, cfg.VideoExts)
		if err != nil {
			vfiles = nil
		}
		idx := make(map[string]int64, len(vfiles))
		for _, f := range vfiles {
			idx[f.Name] = f.Size
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

			// Check if cached entry matches
			var oldEntry *SourceEntry
			switch d.label {
			case "input":
				oldEntry = oldStatus.Input
			case "output":
				oldEntry = oldStatus.Output
			case "optimize":
				oldEntry = oldStatus.Optimize
			case "interpolated":
				oldEntry = oldStatus.Interpolated
			}

			if oldEntry != nil && oldEntry.Size == size {
				// Size matches — reuse cached resolution
				entry := *oldEntry
				switch d.label {
				case "input":
					status.Input = &entry
				case "output":
					status.Output = &entry
				case "optimize":
					status.Optimize = &entry
				case "interpolated":
					status.Interpolated = &entry
				}
			} else {
				// New or changed — need ffprobe
				needProbe[d.label] = append(needProbe[d.label], name)
				entry := SourceEntry{Size: size}
				switch d.label {
				case "input":
					status.Input = &entry
				case "output":
					status.Output = &entry
				case "optimize":
					status.Optimize = &entry
				case "interpolated":
					status.Interpolated = &entry
				}
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
				switch label {
				case "input":
					if status.Input != nil {
						status.Input.Width = res.Width
						status.Input.Height = res.Height
						status.Input.Audio = res.Audio
						status.Input.Subtitles = res.Subtitles
					}
				case "output":
					if status.Output != nil {
						status.Output.Width = res.Width
						status.Output.Height = res.Height
						status.Output.Audio = res.Audio
						status.Output.Subtitles = res.Subtitles
					}
				case "optimize":
					if status.Optimize != nil {
						status.Optimize.Width = res.Width
						status.Optimize.Height = res.Height
						status.Optimize.Audio = res.Audio
						status.Optimize.Subtitles = res.Subtitles
					}
				case "interpolated":
					if status.Interpolated != nil {
						status.Interpolated.Width = res.Width
						status.Interpolated.Height = res.Height
						status.Interpolated.Audio = res.Audio
						status.Interpolated.Subtitles = res.Subtitles
					}
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
