package runner

import (
	"fmt"
	"io"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
)

// formatMicroseconds renders a microsecond count as "HH:MM:SS" to match
// video2x's elapsed field (always zero-padded, always includes hours).
func formatMicroseconds(us int64) string {
	total := us / 1_000_000
	h := total / 3600
	m := (total % 3600) / 60
	s := total % 60
	return fmt.Sprintf("%02d:%02d:%02d", h, m, s)
}

// ExtractFinalFrameCount returns the last `frame=N` value found in an ffmpeg
// stderr/stdout transcript. Used as a fallback when ffprobe can't determine
// total frame count for a container (e.g. some MKVs): the integrity-check
// decode pass already walks every frame, so its final frame= stat is exact.
// Returns 0 when no match is found.
func ExtractFinalFrameCount(s string) int {
	matches := reFrame.FindAllStringSubmatch(s, -1)
	if len(matches) == 0 {
		return 0
	}
	n, _ := strconv.Atoi(matches[len(matches)-1][1])
	return n
}

// Progress holds parsed process progress data.
type Progress struct {
	Source      string  `json:"source,omitempty"`
	Filename    string  `json:"filename,omitempty"`
	Frame       int     `json:"frame"`
	FPS         float64 `json:"fps"`
	TotalFrames int     `json:"total_frames,omitempty"`
	Elapsed     string  `json:"elapsed,omitempty"`
	Speed       string  `json:"speed,omitempty"`
	Percent     float64 `json:"percent,omitempty"`
}

var (
	reFrameSlash  = regexp.MustCompile(`frame=(\d+)/(\d+)`)
	reFrame       = regexp.MustCompile(`frame=\s*(\d+)`)
	reFPS         = regexp.MustCompile(`fps=\s*([\d.]+)`)
	reElapsed     = regexp.MustCompile(`elapsed=(\S+)`)
	reSpeed       = regexp.MustCompile(`speed=\s*(\S+)`)
	reOutTimeUS   = regexp.MustCompile(`out_time_us=\s*(\d+)`)
	reOutTimeMS   = regexp.MustCompile(`out_time_ms=\s*(\d+)`)
	reTotalFrames = regexp.MustCompile(`NUMBER_OF_FRAMES:\s*(\d+)`)
	reStreamVideo = regexp.MustCompile(`Stream\s+#\d+:\d+.*Video:`)
	reStreamAny   = regexp.MustCompile(`Stream\s+#\d+:\d+`)
)

// progressWriter is an io.Writer that tees all writes to an underlying writer
// while parsing FFmpeg progress lines and calling a callback.
type progressWriter struct {
	underlying io.Writer
	onProgress func(Progress)

	mu            sync.Mutex
	buf           []byte
	current       Progress
	lastEmit      time.Time
	minInterval   time.Duration
	inVideoStream bool
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

	// Track which stream's metadata we're in so we only pick up
	// NUMBER_OF_FRAMES from the video stream, not audio/subtitle streams.
	if reStreamVideo.MatchString(line) {
		pw.inVideoStream = true
	} else if reStreamAny.MatchString(line) {
		pw.inVideoStream = false
	}

	// Check for total frames — only from the video stream section
	if pw.inVideoStream {
		if m := reTotalFrames.FindStringSubmatch(line); m != nil {
			if v, err := strconv.Atoi(m[1]); err == nil && v > 0 {
				pw.current.TotalFrames = v
				changed = true
			}
		}
	}

	// Check for progress line — try video2x "frame=N/T" first, then plain "frame=N"
	if m := reFrameSlash.FindStringSubmatch(line); m != nil {
		if v, err := strconv.Atoi(m[1]); err == nil {
			pw.current.Frame = v
			changed = true
		}
		if v, err := strconv.Atoi(m[2]); err == nil && v > 0 {
			pw.current.TotalFrames = v
			changed = true
		}
	} else if m := reFrame.FindStringSubmatch(line); m != nil {
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
		pw.current.Elapsed = strings.TrimRight(strings.TrimSpace(m[1]), ";")
		changed = true
	}

	// ffmpeg -progress emits out_time_us (new) or out_time_ms (older) — both
	// are actually microseconds despite the "ms" name. Format as H:MM:SS so it
	// renders consistently with video2x's elapsed=... output.
	if m := reOutTimeUS.FindStringSubmatch(line); m != nil {
		if v, err := strconv.ParseInt(m[1], 10, 64); err == nil && v >= 0 {
			pw.current.Elapsed = formatMicroseconds(v)
			changed = true
		}
	} else if m := reOutTimeMS.FindStringSubmatch(line); m != nil {
		if v, err := strconv.ParseInt(m[1], 10, 64); err == nil && v >= 0 {
			pw.current.Elapsed = formatMicroseconds(v)
			changed = true
		}
	}

	if m := reSpeed.FindStringSubmatch(line); m != nil {
		s := strings.TrimSpace(m[1])
		if s != "N/A" {
			pw.current.Speed = s
		}
		changed = true
	}

	// ffmpeg -progress blocks end with "progress=continue" or "progress=end".
	// Emit at the block boundary so frame/fps/speed/elapsed from the same tick
	// arrive together, bypassing the rate-limit (blocks already ~500ms apart).
	blockEnd := strings.HasPrefix(line, "progress=")

	if !changed && !blockEnd {
		return
	}

	// Compute percent
	if pw.current.TotalFrames > 0 && pw.current.Frame > 0 {
		pw.current.Percent = float64(pw.current.Frame) / float64(pw.current.TotalFrames) * 100
	}

	// Rate-limit emissions (block boundary always emits)
	now := time.Now()
	if blockEnd || now.Sub(pw.lastEmit) >= pw.minInterval {
		pw.lastEmit = now
		p := pw.current // copy
		pw.onProgress(p)
	}
}
