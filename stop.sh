#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "$0")"

stopped=0
for name in api app; do
  pidfile="pids/${name}.pid"
  if [ -f "$pidfile" ]; then
    pid=$(cat "$pidfile")
    if kill -0 "$pid" 2>/dev/null; then
      kill "$pid"
      echo "Stopped $name (pid $pid)"
    else
      echo "$name (pid $pid) not running"
    fi
    rm -f "$pidfile"
    stopped=1
  fi
done

[ "$stopped" -eq 0 ] && echo "No PID files found in pids/"
