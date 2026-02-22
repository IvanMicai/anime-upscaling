#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "$0")"

# Load env
set -a; source .env; set +a

mkdir -p pids logs

# Build
echo "Building API..."
(cd packages/api && go build -o ../../bin/animeup ./cmd/animeup)

echo "Building App..."
cp .env packages/app/.env.local
(cd packages/app && pnpm install --frozen-lockfile && pnpm build)

# Start API
echo "Starting API on :4751..."
nohup ./bin/animeup serve > logs/api.log 2>&1 &
echo $! > pids/api.pid

# Start App
echo "Starting App on :4750..."
(cd packages/app && nohup pnpm start -p 4750 > ../../logs/app.log 2>&1 &
echo $! > ../../pids/app.pid)

echo "Done. PIDs: api=$(cat pids/api.pid) app=$(cat pids/app.pid)"
echo "Logs: logs/api.log, logs/app.log"
