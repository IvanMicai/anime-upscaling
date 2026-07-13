package logger

import (
	"fmt"
	"os"
	"sync"
	"time"
)

// JobLog represents a single log event from a running job.
type JobLog struct {
	Source  string    `json:"source"`
	Level   string    `json:"level"`
	Index   int       `json:"index"`
	Message string    `json:"message"`
	Time    time.Time `json:"time"`
}

const (
	ColorReset   = "\033[0m"
	ColorGreen   = "\033[0;32m"
	ColorRed     = "\033[0;31m"
	ColorYellow  = "\033[0;33m"
	ColorBlue    = "\033[0;34m"
	ColorMagenta = "\033[0;35m"
	ColorCyan    = "\033[0;36m"
	ColorBold    = "\033[1m"
)

type Logger struct {
	mu      sync.Mutex
	logFile *os.File
	total   int
}

func NewLogger(logPath string) (*Logger, error) {
	f, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return nil, fmt.Errorf("open log file: %w", err)
	}
	return &Logger{logFile: f}, nil
}

func (l *Logger) SetTotal(n int) {
	l.total = n
}

func (l *Logger) Close() {
	l.logFile.Close()
}

func (l *Logger) Log(source, level string, index int, msg string) {
	ts := time.Now().Format("2006-01-02 15:04:05")

	sourceColor := ColorCyan
	switch source {
	case "GPU 0":
		sourceColor = ColorBlue
	case "GPU 1":
		sourceColor = ColorMagenta
	case "FFMPEG":
		sourceColor = ColorCyan
	}

	levelColor := ColorCyan
	switch level {
	case "OK":
		levelColor = ColorGreen
	case "ERRO":
		levelColor = ColorRed
	case "SKIP":
		levelColor = ColorYellow
	case "WARN":
		levelColor = ColorYellow
	}

	progress := ""
	if index > 0 && l.total > 0 {
		progress = fmt.Sprintf(" [%d/%d]", index, l.total)
	}

	colored := fmt.Sprintf("%s[%s] [%s%s%s]%s [%s%s%s] %s",
		ColorReset, ts, sourceColor, source, ColorReset, progress, levelColor, level, ColorReset, msg)
	plain := fmt.Sprintf("[%s] [%s]%s [%s] %s", ts, source, progress, level, msg)

	l.mu.Lock()
	defer l.mu.Unlock()
	fmt.Println(colored)
	fmt.Fprintln(l.logFile, plain)
}

func (l *Logger) Info(source string, index int, msg string) {
	l.Log(source, "INFO", index, msg)
}

func (l *Logger) Ok(source string, index int, msg string) {
	l.Log(source, "OK", index, msg)
}

func (l *Logger) Erro(source string, index int, msg string) {
	l.Log(source, "ERRO", index, msg)
}

func (l *Logger) Skip(source string, index int, msg string) {
	l.Log(source, "SKIP", index, msg)
}

func (l *Logger) Banner(msg string) {
	ts := time.Now().Format("2006-01-02 15:04:05")
	colored := fmt.Sprintf("%s[%s] %s", ColorCyan, ts, msg+ColorReset)
	plain := fmt.Sprintf("[%s] %s", ts, msg)
	l.mu.Lock()
	defer l.mu.Unlock()
	fmt.Println(colored)
	fmt.Fprintln(l.logFile, plain)
}
