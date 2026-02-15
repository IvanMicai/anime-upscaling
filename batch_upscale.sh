#!/bin/bash

# Directories
INPUT_DIR="${INPUT_DIR:-/input}"
OUTPUT_DIR="${OUTPUT_DIR:-/output}"
WEIGHTS_DIR="/opt/Real-ESRGAN/weights"

# Model and parameters
MODEL="${MODEL:-realesr-animevideov3}"
SCALE="${SCALE:-4}"
TILE="${TILE:-0}"
DENOISE="${DENOISE:-1.0}"
NUM_PROC="${NUM_PROC:-1}"
SUFFIX="upscaled"
EXT="mp4"

# Ensure weights directory exists
mkdir -p "$WEIGHTS_DIR"

# Download weights if missing or corrupt (< 1MB = likely truncated)
MODEL_PATH="$WEIGHTS_DIR/$MODEL.pth"
if [ ! -f "$MODEL_PATH" ] || [ "$(stat -c%s "$MODEL_PATH" 2>/dev/null || echo 0)" -lt 1000000 ]; then
  [ -f "$MODEL_PATH" ] && echo "[WARN] $MODEL.pth appears corrupt, re-downloading..."
  echo "[INFO] Downloading $MODEL.pth..."
  wget -q "https://github.com/xinntao/Real-ESRGAN/releases/download/v0.2.5.0/$MODEL.pth" \
    -O "$MODEL_PATH"
fi

# CUDA check via PyTorch
python3 - <<'PY'
import sys, torch
assert torch.cuda.is_available(), "CUDA not available inside container"
PY

# Detect available GPUs
NUM_GPUS=$(nvidia-smi -L 2>/dev/null | wc -l)
if [ "$NUM_GPUS" -lt 1 ]; then
  echo "[ERROR] No GPUs detected by nvidia-smi"
  exit 1
fi
echo "[INFO] Detected $NUM_GPUS GPU(s)"
echo "[INFO] Parameters: TILE=$TILE, NUM_PROC=$NUM_PROC, SCALE=$SCALE, MODEL=$MODEL, DENOISE=$DENOISE"

# Collect all video files
shopt -s nullglob nocaseglob
FILES=()
for FILE in "$INPUT_DIR"/*.{mp4,mkv,avi,webm,mov,flv,wmv,m4v}; do
  [ -f "$FILE" ] && FILES+=("$FILE")
done
shopt -u nocaseglob

TOTAL=${#FILES[@]}
if [ "$TOTAL" -eq 0 ]; then
  echo "[INFO] No video files found in $INPUT_DIR"
  exit 0
fi
echo "[INFO] Found $TOTAL video(s) to process with $NUM_GPUS GPU(s)"
echo

# Tracking
declare -A GPU_PIDS    # GPU_PIDS[gpu_id] = pid
declare -A PID_FILE    # PID_FILE[pid] = original filename
declare -A PID_START   # PID_START[pid] = start time (seconds)
declare -A PID_GPU     # PID_GPU[pid] = gpu_id
SUCCEEDED=0
FAILED=0

process_video() {
  local FILE="$1"
  local GPU_ID="$2"
  local LOG_FILE="/tmp/upscale_gpu${GPU_ID}.log"

  local BASENAME=$(basename "$FILE")
  local FILE_EXT="${BASENAME##*.}"
  local FILENAME="${BASENAME%.*}"

  # Skip if final output already exists (idempotent for retries)
  if [[ -f "$OUTPUT_DIR/${FILENAME}.mkv" ]]; then
    echo "[GPU${GPU_ID}|${BASENAME}] Ja existe, pulando"
    return 0
  fi

  # Sanitize filename: replace spaces and special chars with underscores
  local SAFE_NAME=$(echo "$FILENAME" | sed 's/[][() ]/_/g; s/__*/_/g; s/^_//; s/_$//')

  # If filename has problematic characters, use a symlink with the safe name
  local ACTUAL_INPUT="$FILE"
  local USED_SYMLINK=""
  if [[ "$SAFE_NAME" != "$FILENAME" ]]; then
    local SYMLINK="/tmp/${SAFE_NAME}.${FILE_EXT}"
    ln -sf "$FILE" "$SYMLINK"
    ACTUAL_INPUT="$SYMLINK"
    USED_SYMLINK="$SYMLINK"
  fi

  local ACTUAL_BASENAME=$(basename "$ACTUAL_INPUT")
  local ACTUAL_FILENAME="${ACTUAL_BASENAME%.*}"
  local OUTPUT_VIDEO="$OUTPUT_DIR/${ACTUAL_FILENAME}_${SUFFIX}.${EXT}"

  echo "[GPU${GPU_ID}|${BASENAME}] Iniciando upscale: ${SCALE}x, tile=${TILE}, denoise=${DENOISE}"

  if [[ "$NUM_PROC" -gt 1 ]]; then
    echo "[GPU${GPU_ID}|${BASENAME}] Dividindo video em ${NUM_PROC} segmentos..."
  fi

  # Run upscale
  CUDA_VISIBLE_DEVICES="$GPU_ID" python3 /opt/Real-ESRGAN/inference_realesrgan_video.py \
    -i "$ACTUAL_INPUT" -o "$OUTPUT_DIR" -n "$MODEL" \
    -s "$SCALE" --tile "$TILE" \
    --denoise_strength "$DENOISE" \
    --num_process_per_gpu "$NUM_PROC" \
    --suffix "$SUFFIX" --ext "$EXT" \
    2>&1 | stdbuf -oL grep -v -E "^\s*(ffmpeg version|built with|configuration:|lib(av|sw|post)|Input #|Output #|Stream #|Stream mapping|Metadata:|Chapter #|Press \[|Side data|handler_name|vendor_id|compatible_brands|major_brand|minor_version|encoder|cpb:|\[lib)" \
    | stdbuf -oL sed "s/^/[GPU${GPU_ID}|${BASENAME}] /" | tee -a "$LOG_FILE"

  local RC=${PIPESTATUS[0]}

  # Clean up Real-ESRGAN temp files
  rm -rf "$OUTPUT_DIR/${ACTUAL_FILENAME}_inp_tmp_videos" \
         "$OUTPUT_DIR/${ACTUAL_FILENAME}_out_tmp_videos" \
         "$OUTPUT_DIR/${ACTUAL_FILENAME}_vidlist.txt"

  if [[ $RC -ne 0 || ! -f "$OUTPUT_VIDEO" ]]; then
    echo "[ERROR] Upscale failed for: $BASENAME (see $LOG_FILE)" >&2
    [[ -n "$USED_SYMLINK" ]] && rm -f "$USED_SYMLINK"
    return 1
  fi

  # If we used a safe name, rename output back to original name
  local FINAL_OUTPUT="$OUTPUT_DIR/${FILENAME}_${SUFFIX}.${EXT}"
  if [[ "$SAFE_NAME" != "$FILENAME" && "$OUTPUT_VIDEO" != "$FINAL_OUTPUT" ]]; then
    mv "$OUTPUT_VIDEO" "$FINAL_OUTPUT"
    OUTPUT_VIDEO="$FINAL_OUTPUT"
  fi

  # Remux: add audio and subtitles from original
  echo "[GPU${GPU_ID}|${BASENAME}] Remuxando audio/legendas..."
  ffmpeg -y -i "$OUTPUT_VIDEO" -i "$FILE" -map 0:v -map 1:a -map "1:s?" -c copy \
    "$OUTPUT_DIR/${FILENAME}.mkv" >> "$LOG_FILE" 2>&1
  rm -f "$OUTPUT_VIDEO"
  echo "[GPU${GPU_ID}|${BASENAME}] Finalizado: ${FILENAME}.mkv"

  # Clean up symlink
  [[ -n "$USED_SYMLINK" ]] && rm -f "$USED_SYMLINK"
  return 0
}

# Find a free GPU. Sets FREE_GPU to the gpu id, or -1 if none free.
find_free_gpu() {
  FREE_GPU=-1
  for ((g=0; g<NUM_GPUS; g++)); do
    local pid="${GPU_PIDS[$g]:-}"
    if [[ -z "$pid" ]]; then
      FREE_GPU=$g
      return
    fi
    # Check if the process is still running
    if ! kill -0 "$pid" 2>/dev/null; then
      # Process finished, harvest it
      wait "$pid" 2>/dev/null
      local rc=$?
      local fname="${PID_FILE[$pid]}"
      local gpu="${PID_GPU[$pid]}"
      local start="${PID_START[$pid]}"
      local elapsed=$(( SECONDS - start ))
      local mins=$(( elapsed / 60 ))
      local secs=$(( elapsed % 60 ))
      if [[ $rc -eq 0 ]]; then
        echo "[GPU${gpu}] Concluido: $fname (${mins}m${secs}s)"
        ((SUCCEEDED++))
      else
        echo "[GPU${gpu}] FALHOU: $fname (${mins}m${secs}s)"
        ((FAILED++))
      fi
      unset GPU_PIDS[$g]
      unset PID_FILE[$pid]
      unset PID_START[$pid]
      unset PID_GPU[$pid]
      FREE_GPU=$g
      return
    fi
  done
}

# Wait for any one GPU to become free
wait_for_gpu() {
  while true; do
    find_free_gpu
    if [[ $FREE_GPU -ge 0 ]]; then
      return
    fi
    # Wait for any child to finish
    wait -n 2>/dev/null || true
  done
}

# Harvest all remaining jobs
wait_all() {
  for ((g=0; g<NUM_GPUS; g++)); do
    local pid="${GPU_PIDS[$g]:-}"
    [[ -z "$pid" ]] && continue
    wait "$pid" 2>/dev/null
    local rc=$?
    local fname="${PID_FILE[$pid]}"
    local gpu="${PID_GPU[$pid]}"
    local start="${PID_START[$pid]}"
    local elapsed=$(( SECONDS - start ))
    local mins=$(( elapsed / 60 ))
    local secs=$(( elapsed % 60 ))
    if [[ $rc -eq 0 ]]; then
      echo "[GPU${gpu}] Concluido: $fname (${mins}m${secs}s)"
      ((SUCCEEDED++))
    else
      echo "[GPU${gpu}] FALHOU: $fname (${mins}m${secs}s)"
      ((FAILED++))
    fi
    unset GPU_PIDS[$g]
  done
}

# Main dispatch loop
IDX=0
for FILE in "${FILES[@]}"; do
  IDX=$((IDX + 1))
  BASENAME=$(basename "$FILE")
  FILENAME="${BASENAME%.*}"

  # Skip if output already exists
  if [[ -f "$OUTPUT_DIR/${FILENAME}.mkv" ]]; then
    echo "[SKIP] Ja existe: ${FILENAME}.mkv ($IDX/$TOTAL)"
    continue
  fi

  wait_for_gpu

  echo "[GPU${FREE_GPU}] Iniciando: $BASENAME ($IDX/$TOTAL)"

  # Clear log for this GPU slot
  > "/tmp/upscale_gpu${FREE_GPU}.log"

  process_video "$FILE" "$FREE_GPU" &
  local_pid=$!
  GPU_PIDS[$FREE_GPU]=$local_pid
  PID_FILE[$local_pid]="$BASENAME"
  PID_START[$local_pid]=$SECONDS
  PID_GPU[$local_pid]=$FREE_GPU
done

# Wait for remaining jobs
wait_all

echo
echo "[INFO] Processamento concluido: $SUCCEEDED OK, $FAILED falha(s) de $TOTAL video(s)"
