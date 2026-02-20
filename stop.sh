#!/bin/bash
#
# stop.sh - Para o runner/pipeline e todos os containers video2x e ffmpeg
#

echo "Parando runner.sh..."
pkill -f runner.sh 2>/dev/null && echo "runner.sh parado." || echo "runner.sh não estava rodando."

echo "Parando pipeline/runner scripts..."
pkill -f pipeline_upscale.sh 2>/dev/null && echo "pipeline_upscale.sh parado." || echo "pipeline_upscale.sh não estava rodando."
pkill -f runner_optimize.sh 2>/dev/null && echo "runner_optimize.sh parado." || echo "runner_optimize.sh não estava rodando."

echo "Parando containers video2x..."
CONTAINERS=$(docker ps -q --filter ancestor=ghcr.io/k4yt3x/video2x:6.4.0)
if [ -n "$CONTAINERS" ]; then
    docker stop $CONTAINERS
    echo "Containers video2x parados."
else
    echo "Nenhum container video2x rodando."
fi

echo "Parando containers ffmpeg..."
CONTAINERS=$(docker ps -q --filter ancestor=linuxserver/ffmpeg)
if [ -n "$CONTAINERS" ]; then
    docker stop $CONTAINERS
    echo "Containers ffmpeg parados."
else
    echo "Nenhum container ffmpeg rodando."
fi
