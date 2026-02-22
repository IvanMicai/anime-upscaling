#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "$0")"

# Load env
set -a; source .env; set +a

mkdir -p pids logs

# Start API
echo "Starting API on :$API_PORT..."
nohup ./bin/api serve > logs/api.log 2>&1 &
echo $! > pids/api.pid

# Start App
echo "Starting App on :$APP_PORT..."
docker rm -f anime-upscaling-app 2>/dev/null || true
nohup docker run --rm --name anime-upscaling-app \
  --env-file .env \
  -e PORT=$APP_PORT \
  -p $APP_PORT:$APP_PORT \
  anime-upscaling-app > logs/app.log 2>&1 &
sleep 1
docker inspect -f '{{.State.Pid}}' anime-upscaling-app > pids/app.pid 2>/dev/null || echo $! > pids/app.pid

echo "Done. PIDs: api=$(cat pids/api.pid) app=$(cat pids/app.pid)"
echo "Logs: logs/api.log, logs/app.log"
