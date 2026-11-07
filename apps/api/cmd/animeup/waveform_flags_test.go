package main

import (
	"anime-upscaling/internal/dualaudio"
	"context"
	"strings"
	"testing"
)

func TestWaveformProbeRequiresStereoAndDisallowsHoldout(t *testing.T) {
	for _, args := range [][]string{
		{"--base", "x", "--dub", "y", "--timeline", "z", "--waveform-at", "3"},
		{"--base", "x", "--dub", "y", "--timeline", "z", "--waveform-at", "3", "--stereo", "--holdout"},
	} {
		err := dualaudioVisualAudio(context.Background(), dualaudio.Tools{}, args)
		if err == nil || !strings.Contains(err.Error(), "--waveform-at requires") {
			t.Fatalf("invalid combination not rejected: %v", err)
		}
	}
}

func TestWaveformScanRejectsInvalidOptions(t *testing.T) {
	for _, extra := range [][]string{
		{"--waveform-every", "5", "--stereo"},
		{"--waveform-every", "NaN", "--stereo"},
		{"--waveform-every", "15"},
		{"--waveform-every", "15", "--stereo", "--holdout"},
		{"--waveform-every", "15", "--stereo", "--waveform-at", "30"},
	} {
		args := append([]string{"--base", "x", "--dub", "y", "--timeline", "z"}, extra...)
		err := dualaudioVisualAudio(context.Background(), dualaudio.Tools{}, args)
		if err == nil || !strings.Contains(err.Error(), "--waveform-every") {
			t.Fatalf("invalid combination not rejected: %v", err)
		}
	}
}
