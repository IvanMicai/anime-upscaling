package runner

import (
	"io"
	"testing"
	"time"
)

func TestFFmpegProgressUsesWallClockElapsed(t *testing.T) {
	var got Progress
	pw := newFFmpegProgressWriter(io.Discard, func(p Progress) {
		got = p
	}, "Encode")
	pw.startedAt = time.Now().Add(-65 * time.Second)

	pw.parseLine("out_time_us=5000000")
	pw.parseLine("progress=continue")

	if got.Elapsed != "00:01:05" {
		t.Fatalf("expected wall-clock elapsed, got %q", got.Elapsed)
	}
	if got.Phase != "Encode" {
		t.Fatalf("expected phase Encode, got %q", got.Phase)
	}
}

func TestFormatDuration(t *testing.T) {
	got := FormatDuration(2*time.Hour + 3*time.Minute + 4*time.Second)
	if got != "02:03:04" {
		t.Fatalf("FormatDuration mismatch: got %q", got)
	}
}
