package runner

import (
	"strings"
	"sync"
)

// tailWriter is an io.Writer that keeps the last N lines of output in memory.
// Used to capture error details from process stderr/stdout.
type tailWriter struct {
	mu    sync.Mutex
	lines []string
	buf   []byte
	max   int
}

func newTailWriter(maxLines int) *tailWriter {
	return &tailWriter{max: maxLines}
}

func (tw *tailWriter) Write(p []byte) (int, error) {
	tw.mu.Lock()
	defer tw.mu.Unlock()

	tw.buf = append(tw.buf, p...)

	for {
		idx := -1
		for i, b := range tw.buf {
			if b == '\n' || b == '\r' {
				idx = i
				break
			}
		}
		if idx < 0 {
			break
		}

		line := string(tw.buf[:idx])
		tw.buf = tw.buf[idx+1:]

		if line == "" {
			continue
		}
		tw.lines = append(tw.lines, line)
		if len(tw.lines) > tw.max {
			tw.lines = tw.lines[len(tw.lines)-tw.max:]
		}
	}

	return len(p), nil
}

// LastLines returns the captured lines joined by newline, filtering out
// progress/stats lines that aren't useful as error context.
func (tw *tailWriter) LastLines() string {
	tw.mu.Lock()
	defer tw.mu.Unlock()

	// Flush any remaining partial line
	if len(tw.buf) > 0 {
		line := strings.TrimSpace(string(tw.buf))
		if line != "" {
			tw.lines = append(tw.lines, line)
			if len(tw.lines) > tw.max {
				tw.lines = tw.lines[len(tw.lines)-tw.max:]
			}
		}
		tw.buf = nil
	}

	var filtered []string
	for _, line := range tw.lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		// Skip progress/stats lines
		if strings.Contains(line, "frame=") && strings.Contains(line, "fps=") {
			continue
		}
		if strings.HasPrefix(line, "video:") || strings.HasPrefix(line, "audio:") {
			continue
		}
		if strings.Contains(line, "size=") && strings.Contains(line, "time=") {
			continue
		}
		filtered = append(filtered, line)
	}

	return strings.Join(filtered, "\n")
}
