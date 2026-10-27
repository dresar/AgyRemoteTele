#!/usr/bin/env bash
set -e

DIR="/opt/data/home/agyremote"
cd "$DIR"

PORT=8787
HOST="127.0.0.1"
LOG_FILE="$DIR/server.log"
PID_FILE="$DIR/server.pid"

if [ -f "$PID_FILE" ]; then
  OLD_PID=$(cat "$PID_FILE" 2>/dev/null || true)
  if [ -n "$OLD_PID" ] && kill -0 "$OLD_PID" 2>/dev/null; then
    kill -15 "$OLD_PID" 2>/dev/null || true
    sleep 0.5
    kill -9 "$OLD_PID" 2>/dev/null || true
  fi
fi

OLD_PIDS=$(pgrep -f "server/index.ts" 2>/dev/null || true)
for p in $OLD_PIDS; do
  if [ "$p" != "$$" ]; then
    kill -15 "$p" 2>/dev/null || true
    sleep 0.5
    kill -9 "$p" 2>/dev/null || true
  fi
done

setsid node "$DIR/server/index.ts" >> "$LOG_FILE" 2>&1 < /dev/null &
SERVER_PID=$!
echo "$SERVER_PID" > "$PID_FILE"

READY=0
for i in $(seq 1 30); do
  if curl -s -f "http://$HOST:$PORT/api/health" > /dev/null 2>&1; then
    READY=1
    break
  fi
  sleep 0.2
done

if [ "$READY" -eq 1 ]; then
  echo "Backend daemon successfully started on http://$HOST:$PORT (PID $SERVER_PID)"
  exit 0
else
  echo "Failed to verify backend health on http://$HOST:$PORT"
  cat "$LOG_FILE" | tail -n 20
  exit 1
fi
