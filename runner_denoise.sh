#!/bin/bash
#
# runner_denoise.sh - Denoise de vídeos com video2x usando 2 GPUs em paralelo
#
# Usa libplacebo com shader Anime4K_Denoise_Bilateral_Mode.glsl para
# aplicar denoise sem alterar a resolução do vídeo.
#
# REQUISITO:
#   Shader em /mnt/SSD2/process/shaders/Anime4K_Denoise_Bilateral_Mode.glsl
#
# USO:
#   chmod +x runner_denoise.sh
#   ./runner_denoise.sh              # Executa em foreground
#   nohup ./runner_denoise.sh &      # Executa em background (sobrevive a fechar o terminal)
#
# INTERROMPER:
#   Ctrl+C                   # Para o script (containers Docker em execução continuam)
#   docker ps                # Lista containers ainda rodando
#   docker stop $(docker ps -q --filter ancestor=ghcr.io/k4yt3x/video2x:6.4.0)  # Para os containers do video2x
#
# LOGS:
#   tail -f /mnt/SSD2/process/process.log   # Acompanhar em tempo real
#   cat /mnt/SSD2/process/process.log       # Ver log completo
#   grep ERRO /mnt/SSD2/process/process.log # Ver apenas erros
#   grep SKIP /mnt/SSD2/process/process.log # Ver arquivos pulados
#
# NOTAS:
#   - Coloque os .mp4 em /mnt/SSD2/process/input/
#   - Arquivos já existentes em output/ são pulados automaticamente
#

# --- CONFIGURAÇÃO ---
BASE_DIR="/mnt/SSD2/process"
INPUT_DIR="$BASE_DIR/input"
OUTPUT_DIR="$BASE_DIR/output"

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

# Cria as pastas necessárias
mkdir -p "$OUTPUT_DIR"

# Carrega todos os vídeos para uma lista (Array)
shopt -s nullglob
FILES=("$INPUT_DIR"/*)
TOTAL_FILES=${#FILES[@]}
CURRENT_INDEX=0

# Inicializa PIDs como vazio (GPU livre)
PID_GPU0=""
PID_GPU1=""

# --- FUNÇÃO DE PROCESSAMENTO (O que a GPU faz) ---
run_task() {
    local video_path=$1
    local gpu_id=$2
    local index=$3
    local filename=$(basename "$video_path")

    # Verifica se já existe na saída
    if [ -f "$OUTPUT_DIR/$filename" ]; then
        log "$gpu_id" SKIP "Pulando $filename (já existe)" "$index"
        return
    fi

    log "$gpu_id" INFO "Iniciando: $filename" "$index"

    local docker_log="$BASE_DIR/docker_gpu${gpu_id}.log"

    # Extract input resolution using ffprobe from inside the video2x image
    local vid_w vid_h
    vid_w=$(docker run --rm -v "$BASE_DIR":/host --entrypoint ffprobe \
        ghcr.io/k4yt3x/video2x:6.4.0 \
        -v error -select_streams v:0 -show_entries stream=width -of csv=p=0 \
        "/host/input/$filename")
    vid_h=$(docker run --rm -v "$BASE_DIR":/host --entrypoint ffprobe \
        ghcr.io/k4yt3x/video2x:6.4.0 \
        -v error -select_streams v:0 -show_entries stream=height -of csv=p=0 \
        "/host/input/$filename")

    if [ -z "$vid_w" ] || [ -z "$vid_h" ]; then
        log "$gpu_id" ERRO "Falha ao obter resolução de: $filename (w=$vid_w, h=$vid_h)" "$index"
        return
    fi

    log "$gpu_id" INFO "Resolução detectada: ${vid_w}x${vid_h}" "$index"

    # Denoise at original resolution using libplacebo + Anime4K denoise shader
    docker run --rm \
      -u $(id -u):$(id -g) \
      --gpus "device=$gpu_id" \
      -v "$BASE_DIR":/host \
      ghcr.io/k4yt3x/video2x:6.4.0 \
      -i "/host/input/$filename" \
      -o "/host/output/$filename" \
      -p libplacebo \
      -w "$vid_w" \
      -h "$vid_h" \
      --libplacebo-shader "/host/shaders/Anime4K_Denoise_Bilateral_Mode.glsl" > "$docker_log" 2>&1

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

    log "$gpu_id" OK "Concluído: $filename" "$index"
}

# --- LOOP GERENCIADOR (O "Chefe") ---
echo -e "${CYAN}[$(date '+%Y-%m-%d %H:%M:%S')] Iniciando denoise de $TOTAL_FILES arquivos com fila dinâmica...${RESET}"
echo "[$(date '+%Y-%m-%d %H:%M:%S')] Iniciando denoise de $TOTAL_FILES arquivos com fila dinâmica..." >> "$LOG_FILE"

while [ $CURRENT_INDEX -lt $TOTAL_FILES ]; do

    # Verifica se a GPU 0 está livre (Se o processo PID_GPU0 não existe mais)
    if [ -z "$PID_GPU0" ] || ! kill -0 "$PID_GPU0" 2>/dev/null; then
        # Pega o próximo arquivo
        FILE="${FILES[$CURRENT_INDEX]}"
        INDEX=$((CURRENT_INDEX + 1))

        # Lança o trabalho em background (&) e salva o PID
        ( run_task "$FILE" 0 "$INDEX" ) &
        PID_GPU0=$!

        # Incrementa o índice para o próximo vídeo
        ((CURRENT_INDEX++))

        # Pequena pausa para evitar race condition nos logs
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

    # Se as duas GPUs estão ocupadas, espera 5 segundos antes de checar de novo
    sleep 5
done

# --- FINALIZAÇÃO ---
echo -e "${YELLOW}[$(date '+%Y-%m-%d %H:%M:%S')] Todos os trabalhos foram distribuídos. Aguardando as últimas tarefas terminarem...${RESET}"
echo "[$(date '+%Y-%m-%d %H:%M:%S')] Todos os trabalhos foram distribuídos. Aguardando as últimas tarefas terminarem..." >> "$LOG_FILE"
wait $PID_GPU0
wait $PID_GPU1
echo -e "${GREEN}[$(date '+%Y-%m-%d %H:%M:%S')] Tudo pronto!${RESET}"
echo "[$(date '+%Y-%m-%d %H:%M:%S')] Tudo pronto!" >> "$LOG_FILE"
