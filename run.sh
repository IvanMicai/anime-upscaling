#!/usr/bin/env bash
# Build and run the batch upscaler
#
# Environment variables (all optional):
#   MODEL     realesr-animevideov3 (default), RealESRGAN_x4plus, realesr-general-x4v3, ...
#   SCALE     2, 3, 4 (default)
#   TILE      0 (default, no tiling), 512, 1024 (for low VRAM)
#   DENOISE   0.0-1.0 (default 1.0)
#   NUM_PROC  parallel segments per GPU (default 1)

docker run --gpus all --rm -it \
  -v "$(realpath ./input)":/input:ro \
  -v "$(realpath ./output)":/output \
  -v "$(realpath ./models)":/opt/Real-ESRGAN/weights \
  -e HOST_UID="$(id -u)" -e HOST_GID="$(id -g)" \
  -e MODEL="${MODEL:-realesr-animevideov3}" \
  -e SCALE="${SCALE:-4}" \
  -e TILE="${TILE:-0}" \
  -e DENOISE="${DENOISE:-1.0}" \
  -e NUM_PROC="${NUM_PROC:-1}" \
  anime-upscaler:latest
