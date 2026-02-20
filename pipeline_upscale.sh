#!/bin/bash
#
# pipeline_upscale.sh - Upscale + compressão H.265 em paralelo
#
# Pipeline completo: video2x (GPU) → ffmpeg (CPU) rodando simultaneamente.
# GPUs fazem upscale em output/, enquanto ffmpeg comprime para optimized/.
#
# USO:
#   chmod +x pipeline_upscale.sh
#   ./pipeline_upscale.sh              # Executa em foreground
#   nohup ./pipeline_upscale.sh &      # Executa em background
#
# INTERROMPER:
#   ./stop.sh                          # Para tudo (video2x + ffmpeg)
#
# LOGS:
#   tail -f /mnt/SSD2/process/process.log   # Acompanhar em tempo real
#   grep FFMPEG /mnt/SSD2/process/process.log # Ver apenas logs do ffmpeg
#
# NOTAS:
#   - Coloque os .mp4 em /mnt/SSD2/process/input/
#   - Arquivos já existentes em output/ são pulados (video2x)
#   - Arquivos já existentes em optimized/ são pulados (ffmpeg)
#

# --- CONFIGURAÇÃO ---
BASE_DIR="/mnt/SSD2/process"
INPUT_DIR="$BASE_DIR/input"
OUTPUT_DIR="$BASE_DIR/output"
OPTIMIZED_DIR="$BASE_DIR/optimized"
READY_DIR="$BASE_DIR/.ready"
DONE_FLAG="$BASE_DIR/.video2x_done"
HALF_CPUS=$(($(nproc) / 2))

USER_ID=$(id -u)
GROUP_ID=$(id -g)

# --- CORES E LOG ---
GREEN='\033[0;32m'
RED='\033[0;31m'
YELLOW='\033[0;33m'
BLUE='\033[0;34m'
MAGENTA='\033[0;35m'
CYAN='\033[0;36m'
RESET='\033[0m'

LOG_FILE="$BASE_DIR/process.log"

# log <gpu_id> <level> <mensagem> [indice]
log() {
    local gpu_id=$1
    local level=$2
    local msg=$3
    local index=$4
    local timestamp
    timestamp=$(date '+%Y-%m-%d %H:%M:%S')

    # Cor do prefixo da GPU
    local gpu_color=$BLUE
    if [ "$gpu_id" -eq 1 ]; then
        gpu_color=$MAGENTA
    fi

    # Cor do nível
    local level_color=$CYAN
    case $level in
        OK)   level_color=$GREEN ;;
        ERRO) level_color=$RED ;;
        SKIP) level_color=$YELLOW ;;
    esac

    # Progresso
    local progress=""
    if [ -n "$index" ]; then
        progress=" [${index}/${TOTAL_FILES}]"
    fi

    # Linha formatada com cores (terminal)
    local colored_line="${RESET}[${timestamp}] [${gpu_color}GPU ${gpu_id}${RESET}]${progress} [${level_color}${level}${RESET}] ${msg}"
    echo -e "$colored_line"

    # Linha sem cores (arquivo de log)
    local plain_line="[${timestamp}] [GPU ${gpu_id}]${progress} [${level}] ${msg}"
    echo "$plain_line" >> "$LOG_FILE"
}

# log_ffmpeg <level> <mensagem>
log_ffmpeg() {
    local level=$1
    local msg=$2
    local timestamp
    timestamp=$(date '+%Y-%m-%d %H:%M:%S')

    # Cor do nível
    local level_color=$CYAN
    case $level in
        OK)   level_color=$GREEN ;;
        ERRO) level_color=$RED ;;
        SKIP) level_color=$YELLOW ;;
    esac

    # Linha formatada com cores (terminal)
    local colored_line="${RESET}[${timestamp}] [${CYAN}FFMPEG${RESET}] [${level_color}${level}${RESET}] ${msg}"
    echo -e "$colored_line"

    # Linha sem cores (arquivo de log)
    local plain_line="[${timestamp}] [FFMPEG] [${level}] ${msg}"
    echo "$plain_line" >> "$LOG_FILE"
}

# Cria as pastas necessárias e limpa flags anteriores
mkdir -p "$OUTPUT_DIR" "$OPTIMIZED_DIR" "$READY_DIR"
rm -f "$DONE_FLAG"

# Carrega todos os vídeos para uma lista (Array)
shopt -s nullglob
FILES=("$INPUT_DIR"/*)
TOTAL_FILES=${#FILES[@]}
CURRENT_INDEX=0

# Inicializa PIDs como vazio (GPU livre)
PID_GPU0=""
PID_GPU1=""

# --- FUNÇÃO DE PROCESSAMENTO VIDEO2X ---
run_task() {
    local video_path=$1
    local gpu_id=$2
    local index=$3
    local filename=$(basename "$video_path")

    # Verifica se já existe na saída
    if [ -f "$OUTPUT_DIR/$filename" ]; then
        log "$gpu_id" SKIP "Pulando $filename (já existe)" "$index"
        # Marca como pronto para ffmpeg (pode já existir em output de run anterior)
        touch "$READY_DIR/$filename"
        return
    fi

    log "$gpu_id" INFO "Iniciando: $filename" "$index"

    # Executa o Video2X v6 - Upscaling 2x
    local docker_log="$BASE_DIR/docker_gpu${gpu_id}.log"
    docker run --rm \
      -u $(id -u):$(id -g) \
      --gpus "device=$gpu_id" \
      -v "$BASE_DIR":/host \
      ghcr.io/k4yt3x/video2x:6.4.0 \
      -i "/host/input/$filename" \
      -o "/host/output/$filename" \
      -p realesrgan \
      -s 2 \
      --realesrgan-model realesr-animevideov3 > "$docker_log" 2>&1

    local exit_code=$?

    log "$gpu_id" INFO "video2x terminou com exit code: $exit_code" "$index"

    if [ $exit_code -ne 0 ]; then
        log "$gpu_id" ERRO "Falha ao processar: $filename (exit code: $exit_code)" "$index"
        log "$gpu_id" ERRO "Últimas linhas do docker: $(tail -5 "$docker_log")" "$index"
        return
    fi

    # Verifica se o arquivo de saída foi realmente criado
    if [ ! -f "$OUTPUT_DIR/$filename" ]; then
        log "$gpu_id" ERRO "video2x retornou 0 mas output não existe: $filename" "$index"
        log "$gpu_id" ERRO "Últimas linhas do docker: $(tail -5 "$docker_log")" "$index"
        return
    fi

    # Corrige permissão
    docker run --rm -v "$OUTPUT_DIR":/work alpine chown $USER_ID:$GROUP_ID "/work/$filename"

    # Sinaliza para o ffmpeg worker que este arquivo está pronto
    touch "$READY_DIR/$filename"

    log "$gpu_id" OK "Concluído: $filename" "$index"
}

# --- FUNÇÕES FFMPEG ---
ffmpeg_task() {
    local filename=$1

    # Skip se já foi comprimido
    if [ -f "$OPTIMIZED_DIR/$filename" ]; then
        log_ffmpeg SKIP "Pulando $filename (já existe)"
        rm -f "$READY_DIR/$filename"
        return
    fi

    log_ffmpeg INFO "Comprimindo: $filename"

    docker run --rm \
      --cpus=$HALF_CPUS \
      -e PUID=$USER_ID -e PGID=$GROUP_ID \
      -v "$BASE_DIR":/work \
      linuxserver/ffmpeg \
      -i "/work/output/$filename" \
      -c:v libx265 -preset fast -crf 22 \
      -tune animation -pix_fmt yuv420p10le \
      -c:a copy \
      "/work/optimized/$filename" >> "$BASE_DIR/docker_ffmpeg.log" 2>&1

    local exit_code=$?

    if [ $exit_code -ne 0 ]; then
        log_ffmpeg ERRO "Falha: $filename (exit code: $exit_code)"
        return
    fi

    rm -f "$READY_DIR/$filename"
    log_ffmpeg OK "Concluído: $filename"
}

ffmpeg_worker() {
    while true; do
        local found=false
        for marker in "$READY_DIR"/*; do
            [ -f "$marker" ] || continue
            ffmpeg_task "$(basename "$marker")"
            found=true
            break  # Re-scan após cada arquivo
        done

        if ! $found; then
            if [ -f "$DONE_FLAG" ]; then
                break
            fi
            sleep 10
        fi
    done
    log_ffmpeg INFO "Worker ffmpeg finalizado."
}

# --- LOOP PRINCIPAL ---
echo -e "${CYAN}[$(date '+%Y-%m-%d %H:%M:%S')] Iniciando pipeline upscale+compressão de $TOTAL_FILES arquivos...${RESET}"
echo "[$(date '+%Y-%m-%d %H:%M:%S')] Iniciando pipeline upscale+compressão de $TOTAL_FILES arquivos..." >> "$LOG_FILE"

# Inicia o worker ffmpeg em background
ffmpeg_worker &
PID_FFMPEG=$!

while [ $CURRENT_INDEX -lt $TOTAL_FILES ]; do

    # Verifica se a GPU 0 está livre
    if [ -z "$PID_GPU0" ] || ! kill -0 "$PID_GPU0" 2>/dev/null; then
        FILE="${FILES[$CURRENT_INDEX]}"
        INDEX=$((CURRENT_INDEX + 1))

        ( run_task "$FILE" 0 "$INDEX" ) &
        PID_GPU0=$!

        ((CURRENT_INDEX++))
        sleep 1
        continue
    fi

    # Verifica se ainda tem arquivos antes de tentar a GPU 1
    if [ $CURRENT_INDEX -ge $TOTAL_FILES ]; then
        break
    fi

    # Verifica se a GPU 1 está livre
    if [ -z "$PID_GPU1" ] || ! kill -0 "$PID_GPU1" 2>/dev/null; then
        FILE="${FILES[$CURRENT_INDEX]}"
        INDEX=$((CURRENT_INDEX + 1))

        ( run_task "$FILE" 1 "$INDEX" ) &
        PID_GPU1=$!

        ((CURRENT_INDEX++))
        sleep 1
        continue
    fi

    # Se as duas GPUs estão ocupadas, espera 5 segundos
    sleep 5
done

# --- FINALIZAÇÃO ---
echo -e "${YELLOW}[$(date '+%Y-%m-%d %H:%M:%S')] Todos os trabalhos video2x distribuídos. Aguardando GPUs terminarem...${RESET}"
echo "[$(date '+%Y-%m-%d %H:%M:%S')] Todos os trabalhos video2x distribuídos. Aguardando GPUs terminarem..." >> "$LOG_FILE"
[ -n "$PID_GPU0" ] && wait $PID_GPU0
[ -n "$PID_GPU1" ] && wait $PID_GPU1

# Sinaliza para o ffmpeg worker que não haverá mais arquivos
touch "$DONE_FLAG"

echo -e "${YELLOW}[$(date '+%Y-%m-%d %H:%M:%S')] GPUs finalizadas. Aguardando ffmpeg terminar compressão restante...${RESET}"
echo "[$(date '+%Y-%m-%d %H:%M:%S')] GPUs finalizadas. Aguardando ffmpeg terminar compressão restante..." >> "$LOG_FILE"
wait $PID_FFMPEG

# Limpa flags
rm -f "$DONE_FLAG"
rm -rf "$READY_DIR"

echo -e "${GREEN}[$(date '+%Y-%m-%d %H:%M:%S')] Pipeline completo! Tudo pronto.${RESET}"
echo "[$(date '+%Y-%m-%d %H:%M:%S')] Pipeline completo! Tudo pronto." >> "$LOG_FILE"
