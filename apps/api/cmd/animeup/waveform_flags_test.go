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
