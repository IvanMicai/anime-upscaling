package process

import (
	"bytes"
	"os"

	"anime-upscaling/internal/runner"
)

// video2xSuccessMarker is the line video2x prints to its log immediately
// before the Vulkan teardown. When glslang's PoolAlloc assertion fires on
// shutdown (a known upstream bug), this marker is already in the log and
// the temp output file is fully written, even though the process exits by
// signal.
var video2xSuccessMarker = []byte("Video processed successfully")

// salvageSignaledRun reports whether a video2x run that died by signal
// actually finished writing its output. Returns true only when all three
// hold: (1) err is a signal-terminated exec error, (2) tempOutPath exists
// with non-zero size, (3) the per-GPU log contains the success marker.
// Callers can treat such runs as successful and move the temp output.
func salvageSignaledRun(err error, logFile, tempOutPath string) bool {
	if _, signaled := runner.SignalFromError(err); !signaled {
		return false
	}
	info, statErr := os.Stat(tempOutPath)
	if statErr != nil || info.Size() == 0 {
		return false
	}
	data, readErr := os.ReadFile(logFile)
	if readErr != nil {
		return false
	}
	return bytes.Contains(data, video2xSuccessMarker)
}
