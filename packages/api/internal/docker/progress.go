package docker

import (
	"io"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Progress holds parsed container progress data.
type Progress struct {
	Frame       int     `json:"frame"`
	FPS         float64 `json:"fps"`
	TotalFrames int     `json:"total_frames,omitempty"`
	Elapsed     string  `json:"elapsed,omitempty"`
	Speed       string  `json:"speed,omitempty"`
	Percent     float64 `json:"percent,omitempty"`
}

var (
	reFrame       = regexp.MustCompile(`frame=\s*(\d+)`)
	reFPS         = regexp.MustCompile(`fps=\s*([\d.]+)`)
	reElapsed     = regexp.MustCompile(`elapsed=(\S+)`)
	reSpeed       = regexp.MustCompile(`speed=\s*(\S+)`)
	reTotalFrames = regexp.MustCompile(`NUMBER_OF_FRAMES:\s*(\d+)`)
)

// progressWriter is an io.Writer that tees all writes to an underlying writer
// while parsing FFmpeg progress lines and calling a callback.
type progressWriter struct {
	underlying io.Writer
	onProgress func(Progress)

	mu          sync.Mutex
	buf         []byte
	current     Progress
	lastEmit    time.Time
	minInterval time.Duration
}

func newProgressWriter(w io.Writer, onProgress func(Progress)) *progressWriter {
	return &progressWriter{
		underlying:  w,
		onProgress:  onProgress,
		minInterval: time.Second,
	}
}

func (pw *progressWriter) Write(p []byte) (int, error) {
	// Always write to the underlying writer first
	n, err := pw.underlying.Write(p)

	pw.mu.Lock()
	defer pw.mu.Unlock()

	// Append to internal buffer
	pw.buf = append(pw.buf, p[:n]...)

	// Process complete lines (split on \n or \r)
	for {
		idx := -1
		for i, b := range pw.buf {
			if b == '\n' || b == '\r' {
				idx = i
				break
			}
		}
		if idx < 0 {
			break
		}

		line := string(pw.buf[:idx])
		pw.buf = pw.buf[idx+1:]

		if line == "" {
			continue
		}

		pw.parseLine(line)
	}

	return n, err
}

func (pw *progressWriter) parseLine(line string) {
	changed := false

	// Check for total frames (from ffprobe-style header in stats)
	if m := reTotalFrames.FindStringSubmatch(line); m != nil {
		if v, err := strconv.Atoi(m[1]); err == nil && v > 0 {
			pw.current.TotalFrames = v
			changed = true
		}
	}

	// Check for progress line (contains "frame=")
	if m := reFrame.FindStringSubmatch(line); m != nil {
		if v, err := strconv.Atoi(m[1]); err == nil {
			pw.current.Frame = v
			changed = true
		}
	}

	if m := reFPS.FindStringSubmatch(line); m != nil {
		if v, err := strconv.ParseFloat(m[1], 64); err == nil {
			pw.current.FPS = v
			changed = true
		}
	}

	if m := reElapsed.FindStringSubmatch(line); m != nil {
		pw.current.Elapsed = strings.TrimSpace(m[1])
		changed = true
	}

	if m := reSpeed.FindStringSubmatch(line); m != nil {
		s := strings.TrimSpace(m[1])
		if s != "N/A" {
			pw.current.Speed = s
		}
		changed = true
	}

	if !changed {
		return
	}

	// Compute percent
	if pw.current.TotalFrames > 0 && pw.current.Frame > 0 {
		pw.current.Percent = float64(pw.current.Frame) / float64(pw.current.TotalFrames) * 100
	}

	// Rate-limit emissions
	now := time.Now()
	if now.Sub(pw.lastEmit) >= pw.minInterval {
		pw.lastEmit = now
		p := pw.current // copy
		pw.onProgress(p)
	}
}
