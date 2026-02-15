#!/usr/bin/env bash
# Quick-start: build and run the batch upscaler
#
# Environment variables (all optional):
#   PROCESSOR    realesrgan (default), libplacebo, realcugan, rife
#   SCALE        2, 3, 4 (default)
#   MODEL        processor-specific model name (empty = default)
#   CODEC        libx265 (default), libx264, libsvtav1, hevc_nvenc, ...
#   OUTPUT_EXT   mkv (default), mp4
#   NOISE_LEVEL  denoising level (-1 to disable)
#   NUM_GPUS     override auto-detection

docker run --gpus all --rm -it \
  -v "$(realpath ./input)":/input:ro \
  -v "$(realpath ./output)":/output \
  -e HOST_UID="$(id -u)" -e HOST_GID="$(id -g)" \
  -e PROCESSOR="${PROCESSOR:-realesrgan}" \
  -e SCALE="${SCALE:-4}" \
  -e MODEL="${MODEL:-}" \
  -e CODEC="${CODEC:-libx265}" \
  -e OUTPUT_EXT="${OUTPUT_EXT:-mkv}" \
  -e NOISE_LEVEL="${NOISE_LEVEL:-}" \
  -e NUM_GPUS="${NUM_GPUS:-0}" \
  batch-video2x:latest
