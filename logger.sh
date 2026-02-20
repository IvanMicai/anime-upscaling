#!/bin/bash
# Unified container log viewer — color-coded tail of all pipeline logs

BASE_DIR="${1:-${BASE_DIR:-/mnt/SSD2/process}}"

# ANSI colors
BLUE='\033[0;34m'
MAGENTA='\033[0;35m'
CYAN='\033[0;36m'
RESET='\033[0m'

LOG_PROCESS="$BASE_DIR/process.log"
LOG_GPU0="$BASE_DIR/docker_gpu0.log"
LOG_GPU1="$BASE_DIR/docker_gpu1.log"
LOG_FFMPEG="$BASE_DIR/docker_ffmpeg.log"

# Ensure log files exist so tail -f doesn't fail
touch "$LOG_PROCESS" "$LOG_GPU0" "$LOG_GPU1" "$LOG_FFMPEG"

cleanup() {
    kill 0 2>/dev/null
    wait 2>/dev/null
    exit 0
}
trap cleanup SIGINT SIGTERM

tail -f --pid=$$ "$LOG_PROCESS" | sed -u "s/^/[PROCESS] /" &
tail -f --pid=$$ "$LOG_GPU0"    | sed -u "s/^/$(printf "${BLUE}")[GPU0]    $(printf "${RESET}")/" &
tail -f --pid=$$ "$LOG_GPU1"    | sed -u "s/^/$(printf "${MAGENTA}")[GPU1]    $(printf "${RESET}")/" &
tail -f --pid=$$ "$LOG_FFMPEG"  | sed -u "s/^/$(printf "${CYAN}")[FFMPEG]  $(printf "${RESET}")/" &

wait
