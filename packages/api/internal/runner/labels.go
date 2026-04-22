package runner

import "fmt"

// GPUSource returns the log/progress source label for a GPU stream.
// When streamsPerGPU<=1 it returns "GPU N" to preserve the legacy format;
// otherwise "GPU G·S" where S is the 1-indexed stream number on that GPU.
func GPUSource(gpuID, streamIdx, streamsPerGPU int) string {
	if streamsPerGPU <= 1 {
		return fmt.Sprintf("GPU %d", gpuID)
	}
	return fmt.Sprintf("GPU %d·%d", gpuID, streamIdx+1)
}

// FFmpegSource returns the log/progress source label for an FFmpeg worker.
// When ffmpegStreams<=1 it returns "FFMPEG"; otherwise "FFMPEG S" (1-indexed).
func FFmpegSource(slotIdx, ffmpegStreams int) string {
	if ffmpegStreams <= 1 {
		return "FFMPEG"
	}
	return fmt.Sprintf("FFMPEG %d", slotIdx+1)
}
