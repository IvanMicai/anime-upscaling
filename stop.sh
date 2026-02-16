#!/bin/bash
#
# stop.sh - Para o runner.sh e todos os containers video2x
#

echo "Parando runner.sh..."
pkill -f runner.sh 2>/dev/null && echo "runner.sh parado." || echo "runner.sh não estava rodando."

echo "Parando containers video2x..."
CONTAINERS=$(docker ps -q --filter ancestor=k4yt3x/video2x:latest)
if [ -n "$CONTAINERS" ]; then
    docker stop $CONTAINERS
    echo "Containers parados."
else
    echo "Nenhum container video2x rodando."
fi
