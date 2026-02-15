#!/usr/bin/env bash
set -euo pipefail

# ─── Configuration via environment variables ────────────────────────────────
INPUT_DIR="${INPUT_DIR:-/input}"
OUTPUT_DIR="${OUTPUT_DIR:-/output}"

# Processor: realesrgan, libplacebo, realcugan, rife
PROCESSOR="${PROCESSOR:-realesrgan}"
SCALE="${SCALE:-4}"
CODEC="${CODEC:-libx265}"
OUTPUT_EXT="${OUTPUT_EXT:-mkv}"
PIX_FMT="${PIX_FMT:-}"
BIT_RATE="${BIT_RATE:-0}"
NOISE_LEVEL="${NOISE_LEVEL:-}"
LOG_LEVEL="${LOG_LEVEL:-info}"
NUM_GPUS="${NUM_GPUS:-0}"
HOST_UID="${HOST_UID:-0}"
HOST_GID="${HOST_GID:-0}"

# Processor-specific model/shader (leave empty for video2x defaults)
MODEL="${MODEL:-}"
EXTRA_ENCODER_OPTS="${EXTRA_ENCODER_OPTS:-}"

# ─── Auto-detect Vulkan GPUs ────────────────────────────────────────────────
detect_gpus() {
  echo "[INFO] Dispositivos Vulkan detectados:"
  video2x --list-devices 2>&1 | tee /tmp/vulkan_devices.txt || true
  echo

  if [[ "$NUM_GPUS" -gt 0 ]]; then
    echo "[INFO] NUM_GPUS definido manualmente: $NUM_GPUS"
    return
  fi

  # Count lines matching "Device [index]" pattern from --list-devices output
  local count
  count=$(grep -cE '^\s*(Device|GPU)\s+[0-9]+' /tmp/vulkan_devices.txt 2>/dev/null || true)

  # Fallback: count lines containing "Discrete GPU" or "Integrated GPU"
  if [[ "$count" -lt 1 ]]; then
    count=$(grep -ciE '(Discrete|Integrated|Virtual)\s+GPU' /tmp/vulkan_devices.txt 2>/dev/null || true)
  fi

  # Last resort fallback
  if [[ "$count" -lt 1 ]]; then
    echo "[WARN] Nao foi possivel detectar GPUs automaticamente, usando NUM_GPUS=1"
    count=1
  fi

  NUM_GPUS="$count"
  echo "[INFO] GPUs detectadas: $NUM_GPUS"
}

detect_gpus

# ─── Discover video files ───────────────────────────────────────────────────
shopt -s nullglob nocaseglob
FILES=()
for f in "$INPUT_DIR"/*.{mp4,mkv,avi,webm,mov,flv,wmv,m4v}; do
  [[ -f "$f" ]] && FILES+=("$f")
done
shopt -u nocaseglob

TOTAL=${#FILES[@]}
if [[ "$TOTAL" -eq 0 ]]; then
  echo "[INFO] Nenhum video encontrado em $INPUT_DIR"
  exit 0
fi

echo "[INFO] Encontrados $TOTAL video(s) para processar"
echo "[INFO] Processador=$PROCESSOR, Scale=$SCALE, Codec=$CODEC, GPUs=$NUM_GPUS"
echo

# ─── GPU scheduling arrays ──────────────────────────────────────────────────
declare -A GPU_PIDS   # GPU_PIDS[gpu_id] = pid
declare -A PID_FILE   # PID_FILE[pid] = basename
declare -A PID_START  # PID_START[pid] = SECONDS at start
declare -A PID_GPU    # PID_GPU[pid] = gpu_id
SUCCEEDED=0
FAILED=0

# ─── Process a single video ────────────────────────────────────────────────
process_video() {
  local INPUT_FILE="$1"
  local GPU_ID="$2"

  local BASENAME
  BASENAME=$(basename "$INPUT_FILE")
  local FILENAME="${BASENAME%.*}"
  local OUTPUT_FILE="$OUTPUT_DIR/${FILENAME}.${OUTPUT_EXT}"
  local TMP_FILE="$OUTPUT_DIR/${FILENAME}.tmp.${OUTPUT_EXT}"
  local PREFIX="[GPU${GPU_ID}|${BASENAME}]"

  # Skip if output already exists (idempotent for retries)
  if [[ -f "$OUTPUT_FILE" ]]; then
    echo "$PREFIX Ja existe, pulando"
    return 0
  fi

  # Clean up any leftover tmp file from previous failed run
  rm -f "$TMP_FILE"

  # Build video2x command
  local CMD=(
    video2x
    -i "$INPUT_FILE"
    -o "$TMP_FILE"
    -p "$PROCESSOR"
    -d "$GPU_ID"
    -s "$SCALE"
    -c "$CODEC"
    --log-level "$LOG_LEVEL"
    --no-progress
  )

  # Optional: pixel format
  [[ -n "$PIX_FMT" ]] && CMD+=(--pix-fmt "$PIX_FMT")

  # Optional: bit rate (0 = auto/CRF)
  [[ "$BIT_RATE" != "0" ]] && CMD+=(--bit-rate "$BIT_RATE")

  # Optional: noise level
  [[ -n "$NOISE_LEVEL" ]] && CMD+=(-n "$NOISE_LEVEL")

  # Processor-specific model/shader
  case "$PROCESSOR" in
    realesrgan)
      [[ -n "$MODEL" ]] && CMD+=(--realesrgan-model "$MODEL")
      ;;
    realcugan)
      [[ -n "$MODEL" ]] && CMD+=(--realcugan-model "$MODEL")
      ;;
    libplacebo)
      [[ -n "$MODEL" ]] && CMD+=(--libplacebo-shader "$MODEL")
      ;;
  esac

  # Extra encoder options (space-separated key=value pairs)
  if [[ -n "$EXTRA_ENCODER_OPTS" ]]; then
    for opt in $EXTRA_ENCODER_OPTS; do
      CMD+=(-e "$opt")
    done
  fi

  echo "$PREFIX Iniciando upscale: ${SCALE}x"

  # Run video2x, prefix each line with GPU/file tag
  "${CMD[@]}" 2>&1 | sed -u "s/^/$PREFIX /"
  local RC=${PIPESTATUS[0]}

  if [[ $RC -ne 0 || ! -f "$TMP_FILE" ]]; then
    echo "$PREFIX ERRO: upscale falhou (exit code $RC)" >&2
    rm -f "$TMP_FILE"
    return 1
  fi

  # Atomic rename: tmp -> final
  mv "$TMP_FILE" "$OUTPUT_FILE"

  # Fix ownership if running as root with HOST_UID/GID set
  if [[ "$HOST_UID" != "0" || "$HOST_GID" != "0" ]]; then
    chown "${HOST_UID}:${HOST_GID}" "$OUTPUT_FILE" 2>/dev/null || true
  fi

  echo "$PREFIX Finalizado: ${FILENAME}.${OUTPUT_EXT}"
  return 0
}

# ─── GPU scheduling functions ───────────────────────────────────────────────
find_free_gpu() {
  FREE_GPU=-1
  for ((g = 0; g < NUM_GPUS; g++)); do
    local pid="${GPU_PIDS[$g]:-}"
    if [[ -z "$pid" ]]; then
      FREE_GPU=$g
      return
    fi
    if ! kill -0 "$pid" 2>/dev/null; then
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
      unset "GPU_PIDS[$g]"
      unset "PID_FILE[$pid]"
      unset "PID_START[$pid]"
      unset "PID_GPU[$pid]"
      FREE_GPU=$g
      return
    fi
  done
}

wait_for_gpu() {
  while true; do
    find_free_gpu
    if [[ $FREE_GPU -ge 0 ]]; then
      return
    fi
    wait -n 2>/dev/null || true
  done
}

wait_all() {
  for ((g = 0; g < NUM_GPUS; g++)); do
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
    unset "GPU_PIDS[$g]"
  done
}

# ─── Main dispatch loop ────────────────────────────────────────────────────
IDX=0
for FILE in "${FILES[@]}"; do
  ((IDX++))
  BASENAME=$(basename "$FILE")

  # Skip if output already exists
  FILENAME="${BASENAME%.*}"
  if [[ -f "$OUTPUT_DIR/${FILENAME}.${OUTPUT_EXT}" ]]; then
    echo "[SKIP] Ja existe: ${FILENAME}.${OUTPUT_EXT} ($IDX/$TOTAL)"
    ((SUCCEEDED++))
    continue
  fi

  wait_for_gpu

  echo "[GPU${FREE_GPU}] Iniciando: $BASENAME ($IDX/$TOTAL)"

  process_video "$FILE" "$FREE_GPU" &
  local_pid=$!
  GPU_PIDS[$FREE_GPU]=$local_pid
  PID_FILE[$local_pid]="$BASENAME"
  PID_START[$local_pid]=$SECONDS
  PID_GPU[$local_pid]=$FREE_GPU
done

wait_all

echo
echo "[INFO] Processamento concluido: $SUCCEEDED OK, $FAILED falha(s) de $TOTAL video(s)"

if [[ $FAILED -gt 0 ]]; then
  exit 1
fi
