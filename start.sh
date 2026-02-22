#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "$0")"

# Load env
set -a; source .env; set +a

mkdir -p logs

# Start API
echo "Starting API on :$API_PORT..."
docker rm -f anime-upscaling-api 2>/dev/null || true
nohup docker run --rm --name anime-upscaling-api \
  --env-file .env \
  -e API_PORT=$API_PORT \
  -p $API_PORT:$API_PORT \
  -v /var/run/docker.sock:/var/run/docker.sock \
  -v $PROCESS_DIR:$PROCESS_DIR \
  anime-upscaling-api > logs/api.log 2>&1 &

# Start App
echo "Starting App on :$APP_PORT..."
docker rm -f anime-upscaling-app 2>/dev/null || true
nohup docker run --rm --name anime-upscaling-app \
  --env-file .env \
  -e PORT=$APP_PORT \
  -p $APP_PORT:$APP_PORT \
  anime-upscaling-app > logs/app.log 2>&1 &

sleep 1
echo "Done. Containers: anime-upscaling-api, anime-upscaling-app."
echo "Logs: logs/api.log, logs/app.log"
