#!/bin/bash
#
# checker.sh - Verificação de integridade de vídeos
#
# Escaneia input/, output/ e optimized/ verificando se os vídeos estão íntegros.
# Usa ffprobe (verificação rápida) + ffmpeg decode completo (verificação profunda).
#
# USO:
#   chmod +x checker.sh
#   ./checker.sh                        # Verifica todas as pastas
#   ./checker.sh input                  # Verifica apenas input/
#   ./checker.sh output optimized       # Verifica output/ e optimized/
#
# NOTAS:
#   - Suporta .mkv, .mp4, .avi
#   - Usa Docker linuxserver/ffmpeg (mesmo container dos outros scripts)
#   - Dois níveis de verificação: ffprobe (abre?) + ffmpeg decode (frames ok?)
#

# --- CONFIGURAÇÃO ---
BASE_DIR="/mnt/SSD2/process"
ALL_DIRS=("input" "output" "optimized")

USER_ID=$(id -u)
GROUP_ID=$(id -g)

# --- CORES ---
GREEN='\033[0;32m'
RED='\033[0;31m'
YELLOW='\033[0;33m'
CYAN='\033[0;36m'
BOLD='\033[1m'
RESET='\033[0m'

# --- FUNÇÕES ---

log_header() {
    echo ""
    echo -e "${BOLD}${CYAN}=== $1 ===${RESET}"
    echo ""
}

log_ok() {
    echo -e "  ${GREEN}[OK]${RESET}    $1"
}

log_erro() {
    echo -e "  ${RED}[ERRO]${RESET}  $1"
}

log_warn() {
    echo -e "  ${YELLOW}[WARN]${RESET}  $1"
}

log_detail() {
    echo -e "          ${RED}↳ $1${RESET}"
}

# Verifica um único arquivo de vídeo
# Retorna: 0 = ok, 1 = erro no probe, 2 = erro no decode
check_file() {
    local filepath=$1
    local filename
    filename=$(basename "$filepath")
    local rel_path=${filepath#"$BASE_DIR/"}

    # Etapa 1: ffprobe — verifica se o arquivo abre e tem streams
    probe_output=$(docker run --rm \
        -e PUID=$USER_ID \
        -e PGID=$GROUP_ID \
        -v "$BASE_DIR":/work \
        --entrypoint ffprobe \
        linuxserver/ffmpeg \
        -v error \
        -show_entries stream=codec_type \
        -of csv=p=0 \
        "/work/$rel_path" 2>&1)

    probe_exit=$?

    if [ $probe_exit -ne 0 ]; then
        log_erro "$rel_path (ffprobe falhou)"
        if [ -n "$probe_output" ]; then
            log_detail "$probe_output"
        fi
        return 1
    fi

    # Verifica se tem pelo menos uma stream de vídeo
    if ! echo "$probe_output" | grep -q "video"; then
        log_erro "$rel_path (nenhuma stream de vídeo encontrada)"
        return 1
    fi

    # Etapa 2: ffmpeg decode — decodifica tudo, reporta erros
    decode_output=$(docker run --rm \
        -e PUID=$USER_ID \
        -e PGID=$GROUP_ID \
        -v "$BASE_DIR":/work \
        linuxserver/ffmpeg \
        -v error \
        -i "/work/$rel_path" \
        -f null \
        - 2>&1)

    decode_exit=$?

    if [ $decode_exit -ne 0 ] || [ -n "$decode_output" ]; then
        if [ $decode_exit -ne 0 ]; then
            log_erro "$rel_path (decode falhou, exit code: $decode_exit)"
        else
            log_warn "$rel_path (decode com avisos)"
        fi
        # Mostra as primeiras linhas de erro
        while IFS= read -r line; do
            [ -n "$line" ] && log_detail "$line"
        done <<< "$(echo "$decode_output" | head -5)"
        return 2
    fi

    log_ok "$rel_path"
    return 0
}

# --- SELECIONA PASTAS ---

if [ $# -gt 0 ]; then
    DIRS=("$@")
else
    DIRS=("${ALL_DIRS[@]}")
fi

# --- CONTADORES GLOBAIS ---
GLOBAL_OK=0
GLOBAL_ERRO=0
GLOBAL_WARN=0
GLOBAL_TOTAL=0

echo -e "${BOLD}${CYAN}Verificação de integridade de vídeos${RESET}"
echo -e "Base: ${CYAN}$BASE_DIR${RESET}"
echo -e "Pastas: ${CYAN}${DIRS[*]}${RESET}"

# --- LOOP PRINCIPAL ---
shopt -s nullglob

for dir_name in "${DIRS[@]}"; do
    dir_path="$BASE_DIR/$dir_name"

    if [ ! -d "$dir_path" ]; then
        log_header "$dir_name/"
        echo -e "  ${YELLOW}Pasta não encontrada: $dir_path${RESET}"
        continue
    fi

    FILES=("$dir_path"/*.mkv "$dir_path"/*.mp4 "$dir_path"/*.avi)
    file_count=${#FILES[@]}

    log_header "$dir_name/ ($file_count arquivos)"

    if [ $file_count -eq 0 ]; then
        echo -e "  ${YELLOW}Nenhum vídeo encontrado${RESET}"
        continue
    fi

    dir_ok=0
    dir_erro=0
    dir_warn=0
    index=0

    for video_path in "${FILES[@]}"; do
        ((index++))
        ((GLOBAL_TOTAL++))

        check_file "$video_path"
        result=$?

        case $result in
            0) ((dir_ok++)); ((GLOBAL_OK++)) ;;
            1) ((dir_erro++)); ((GLOBAL_ERRO++)) ;;
            2) ((dir_warn++)); ((GLOBAL_WARN++)) ;;
        esac
    done

    # Resumo da pasta
    echo ""
    echo -e "  Resumo: ${GREEN}${dir_ok} ok${RESET}, ${RED}${dir_erro} erro${RESET}, ${YELLOW}${dir_warn} warn${RESET} / ${file_count} total"
done

# --- RESUMO FINAL ---
echo ""
echo -e "${BOLD}${CYAN}=== Resumo Final ===${RESET}"
echo ""
echo -e "  Total verificado: ${BOLD}$GLOBAL_TOTAL${RESET}"
echo -e "  ${GREEN}OK:   $GLOBAL_OK${RESET}"
echo -e "  ${RED}ERRO: $GLOBAL_ERRO${RESET}"
echo -e "  ${YELLOW}WARN: $GLOBAL_WARN${RESET}"
echo ""

if [ $GLOBAL_ERRO -gt 0 ]; then
    echo -e "  ${RED}${BOLD}⚠ Arquivos com erro detectados!${RESET}"
    exit 1
elif [ $GLOBAL_WARN -gt 0 ]; then
    echo -e "  ${YELLOW}Alguns arquivos tiveram avisos durante decode.${RESET}"
    exit 0
else
    echo -e "  ${GREEN}Todos os arquivos estão íntegros.${RESET}"
    exit 0
fi
