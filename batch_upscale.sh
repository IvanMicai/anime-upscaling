#!/bin/bash
set -e

# Directories
INPUT_DIR="${INPUT_DIR:-/input}"
OUTPUT_DIR="${OUTPUT_DIR:-/output}"
WEIGHTS_DIR="/opt/Real-ESRGAN/weights"

# Model and parameters
MODEL="${MODEL:-realesr-animevideov3}"
SCALE="${SCALE:-4}"
TILE="${TILE:-512}"
DENOISE="${DENOISE:-1.0}"
NUM_PROC="${NUM_PROC:-1}"
SUFFIX="upscaled"
EXT="mp4"

# Ensure weights directory exists
mkdir -p "$WEIGHTS_DIR"

# Download weights if missing
if [ ! -f "$WEIGHTS_DIR/$MODEL.pth" ]; then
  echo "[INFO] Downloading $MODEL.pth..."
  wget -q "https://github.com/xinntao/Real-ESRGAN/releases/download/v0.2.5.0/$MODEL.pth" \
    -O "$WEIGHTS_DIR/$MODEL.pth"
fi

# CUDA check via PyTorch
python3 - <<'PY'
import sys, torch
assert torch.cuda.is_available(), "CUDA not available inside container"
PY

# Batch processing
echo "[INFO] NVIDIA GPU detected. Starting batch upscale."
shopt -s nullglob
echo
for FILE in "$INPUT_DIR"/*.{mp4,mkv,avi,webm,mov}; do
  [ -e "$FILE" ] || continue
  BASENAME=$(basename "$FILE")
  FILE_EXT="${BASENAME##*.}"
  FILENAME="${BASENAME%.*}"

  # Sanitize filename: replace spaces and special chars with underscores
  SAFE_NAME=$(echo "$FILENAME" | sed 's/[][() ]/_/g; s/__*/_/g; s/^_//; s/_$//')

  # If filename has problematic characters, use a symlink with the safe name
  ACTUAL_INPUT="$FILE"
  USED_SYMLINK=""
  if [[ "$SAFE_NAME" != "$FILENAME" ]]; then
    SYMLINK="/tmp/${SAFE_NAME}.${FILE_EXT}"
    ln -sf "$FILE" "$SYMLINK"
    ACTUAL_INPUT="$SYMLINK"
    USED_SYMLINK="$SYMLINK"
    echo "[INFO] Filename has spaces/special chars, using symlink: $BASENAME -> $(basename "$SYMLINK")"
  fi

  ACTUAL_BASENAME=$(basename "$ACTUAL_INPUT")
  ACTUAL_FILENAME="${ACTUAL_BASENAME%.*}"
  OUTPUT_VIDEO="$OUTPUT_DIR/${ACTUAL_FILENAME}_${SUFFIX}.${EXT}"

  echo "[INFO] Upscaling: $BASENAME -> $OUTPUT_VIDEO"
  python3 /opt/Real-ESRGAN/inference_realesrgan_video.py \
    -i "$ACTUAL_INPUT" -o "$OUTPUT_DIR" -n "$MODEL" \
    -s "$SCALE" --tile "$TILE" \
    --denoise_strength "$DENOISE" \
    --num_process_per_gpu "$NUM_PROC" \
    --suffix "$SUFFIX" --ext "$EXT"

  # Clean up Real-ESRGAN temp files
  rm -rf "$OUTPUT_DIR/${ACTUAL_FILENAME}_inp_tmp_videos" \
         "$OUTPUT_DIR/${ACTUAL_FILENAME}_out_tmp_videos" \
         "$OUTPUT_DIR/${ACTUAL_FILENAME}_vidlist.txt"

  if [[ ! -f "$OUTPUT_VIDEO" ]]; then
    echo "[ERROR] Upscaled file not created: $OUTPUT_VIDEO"
    [[ -n "$USED_SYMLINK" ]] && rm -f "$USED_SYMLINK"
    continue
  fi

  # If we used a safe name, rename output back to original name
  FINAL_OUTPUT="$OUTPUT_DIR/${FILENAME}_${SUFFIX}.${EXT}"
  if [[ "$SAFE_NAME" != "$FILENAME" && "$OUTPUT_VIDEO" != "$FINAL_OUTPUT" ]]; then
    mv "$OUTPUT_VIDEO" "$FINAL_OUTPUT"
    OUTPUT_VIDEO="$FINAL_OUTPUT"
  fi

  echo "[REMUX] $OUTPUT_VIDEO -> ${FILENAME}.mkv"
  ffmpeg -y -i "$OUTPUT_VIDEO" -i "$FILE" -map 0:v -map 1:a -map "1:s?" -c copy \
    "$OUTPUT_DIR/${FILENAME}.mkv"
  rm -f "$OUTPUT_VIDEO"

  # Clean up symlink
  [[ -n "$USED_SYMLINK" ]] && rm -f "$USED_SYMLINK"
done

echo "\n[INFO] All videos processed into $OUTPUT_DIR"
