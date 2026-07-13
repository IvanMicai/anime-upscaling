package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"sync"
	"time"

	"anime-upscaling/internal/config"
	"anime-upscaling/internal/logger"
)

type tailedLog struct {
	path   string
	prefix string
	color  string
}

func cmdLogs(ctx context.Context) error {
	cfg := config.NewConfig()

	logs := []tailedLog{
		{cfg.BaseDir + "/process.log", "[PROCESS] ", ""},
		{cfg.BaseDir + "/gpu0.log", "[GPU0]    ", logger.ColorBlue},
		{cfg.BaseDir + "/gpu1.log", "[GPU1]    ", logger.ColorMagenta},
		{cfg.BaseDir + "/ffmpeg.log", "[FFMPEG]  ", logger.ColorCyan},
	}

	// Ensure log files exist
	for _, tl := range logs {
		f, err := os.OpenFile(tl.path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
		if err != nil {
			return fmt.Errorf("touch %s: %w", tl.path, err)
		}
		f.Close()
	}

	var mu sync.Mutex
	var wg sync.WaitGroup

	for _, tl := range logs {
		wg.Add(1)
		go func(tl tailedLog) {
			defer wg.Done()
			tailFile(ctx, tl, &mu)
		}(tl)
	}

	wg.Wait()
	return nil
}

func tailFile(ctx context.Context, tl tailedLog, mu *sync.Mutex) {
	f, err := os.Open(tl.path)
	if err != nil {
		return
	}
	defer f.Close()

	// Seek to end
	f.Seek(0, io.SeekEnd)

	buf := make([]byte, 4096)
	ticker := time.NewTicker(200 * time.Millisecond)
	defer ticker.Stop()

	var partial string

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			for {
				n, err := f.Read(buf)
				if n > 0 {
					data := partial + string(buf[:n])
					partial = ""

					lines := splitKeepPartial(data)
					for i, line := range lines {
						if i == len(lines)-1 && !endsWithNewline(data) {
							partial = line
							break
						}
						if line == "" {
							continue
						}
						mu.Lock()
						if tl.color != "" {
							fmt.Printf("%s%s%s%s\n", tl.color, tl.prefix, logger.ColorReset, line)
						} else {
							fmt.Printf("%s%s\n", tl.prefix, line)
						}
						mu.Unlock()
					}
				}
				if err != nil {
					break
				}
			}
		}
	}
}

func splitKeepPartial(s string) []string {
	var lines []string
	start := 0
	for i := 0; i < len(s); i++ {
		if s[i] == '\n' {
			lines = append(lines, s[start:i])
			start = i + 1
		}
	}
	if start < len(s) {
		lines = append(lines, s[start:])
	}
	return lines
}

func endsWithNewline(s string) bool {
	return len(s) > 0 && s[len(s)-1] == '\n'
}
