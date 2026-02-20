#!/bin/bash
#
# runner_optimize.sh - Otimização de vídeos com ffmpeg H.265
#
# Comprime vídeos de input/ para optimized/ usando libx265 com tune animation.
# Reduz tamanho de arquivo sem necessidade de GPU.
#
# USO:
#   chmod +x runner_optimize.sh
#   ./runner_optimize.sh              # Executa em foreground
#   nohup ./runner_optimize.sh &      # Executa em background (sobrevive a fechar o terminal)
#
# INTERROMPER:
#   ./stop.sh                          # Para tudo
#
# LOGS:
#   tail -f /mnt/SSD2/process/process.log   # Acompanhar em tempo real
#   cat /mnt/SSD2/process/process.log       # Ver log completo
#   grep ERRO /mnt/SSD2/process/process.log # Ver apenas erros
#   grep SKIP /mnt/SSD2/process/process.log # Ver arquivos pulados
#
# NOTAS:
#   - Coloque os vídeos (.mkv, .mp4, .avi) em /mnt/SSD2/process/input/
#   - Arquivos já existentes em optimized/ são pulados automaticamente
#

# --- CONFIGURAÇÃO ---
BASE_DIR="/mnt/SSD2/process"
INPUT_DIR="$BASE_DIR/input"
OPTIMIZED_DIR="$BASE_DIR/optimized"
HALF_CPUS=$(($(nproc) / 2))

USER_ID=$(id -u)
GROUP_ID=$(id -g)

# --- CORES E LOG ---
GREEN='\033[0;32m'
RED='\033[0;31m'
YELLOW='\033[0;33m'
CYAN='\033[0;36m'
RESET='\033[0m'

LOG_FILE="$BASE_DIR/process.log"

log() {
    local level=$1
    local msg=$2
    local index=$3
    local timestamp
    timestamp=$(date '+%Y-%m-%d %H:%M:%S')

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
    local colored_line="${RESET}[${timestamp}] [${CYAN}FFMPEG${RESET}]${progress} [${level_color}${level}${RESET}] ${msg}"
    echo -e "$colored_line"

    # Linha sem cores (arquivo de log)
    local plain_line="[${timestamp}] [FFMPEG]${progress} [${level}] ${msg}"
    echo "$plain_line" >> "$LOG_FILE"
}

# Cria as pastas necessárias
mkdir -p "$OPTIMIZED_DIR"

# Carrega todos os vídeos para uma lista
shopt -s nullglob
FILES=("$INPUT_DIR"/*.mkv "$INPUT_DIR"/*.mp4 "$INPUT_DIR"/*.avi)
TOTAL_FILES=${#FILES[@]}

if [ $TOTAL_FILES -eq 0 ]; then
    echo -e "${YELLOW}Nenhum vídeo encontrado em $INPUT_DIR${RESET}"
    exit 0
fi

echo -e "${CYAN}[$(date '+%Y-%m-%d %H:%M:%S')] Iniciando otimização de $TOTAL_FILES arquivos (${HALF_CPUS} CPUs)...${RESET}"
echo "[$(date '+%Y-%m-%d %H:%M:%S')] Iniciando otimização de $TOTAL_FILES arquivos (${HALF_CPUS} CPUs)..." >> "$LOG_FILE"

INDEX=0
for video_path in "${FILES[@]}"; do
    ((INDEX++))
    filename=$(basename "$video_path")

    # Verifica se já existe na saída
    if [ -f "$OPTIMIZED_DIR/$filename" ]; then
        log SKIP "Pulando $filename (já existe)" "$INDEX"
        continue
    fi

    log INFO "Iniciando: $filename" "$INDEX"

    docker run --rm \
      --name ffmpeg-optimize \
      --cpus=$HALF_CPUS \
      -e PUID=$USER_ID \
      -e PGID=$GROUP_ID \
      -v "$BASE_DIR":/work \
      linuxserver/ffmpeg \
      -i "/work/input/$filename" \
      -c:v libx265 \
      -preset fast \
      -crf 22 \
      -tune animation \
      -pix_fmt yuv420p10le \
      -c:a copy \
      "/work/optimized/$filename" >> "$BASE_DIR/docker_ffmpeg.log" 2>&1

    local_exit_code=$?

    if [ $local_exit_code -ne 0 ]; then
        log ERRO "Falha ao processar: $filename (exit code: $local_exit_code)" "$INDEX"
        continue
    fi

    # Verifica se o arquivo de saída foi realmente criado
    if [ ! -f "$OPTIMIZED_DIR/$filename" ]; then
        log ERRO "ffmpeg retornou 0 mas output não existe: $filename" "$INDEX"
        continue
    fi

    # Corrige permissão
    docker run --rm -v "$OPTIMIZED_DIR":/work alpine chown $USER_ID:$GROUP_ID "/work/$filename"

    log OK "Concluído: $filename" "$INDEX"
done

echo -e "${GREEN}[$(date '+%Y-%m-%d %H:%M:%S')] Tudo pronto!${RESET}"
echo "[$(date '+%Y-%m-%d %H:%M:%S')] Tudo pronto!" >> "$LOG_FILE"
