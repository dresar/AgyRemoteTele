#!/bin/bash
set -e

DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
cd "$DIR"

mkdir -p data

while true; do
    echo "[$(date -u '+%Y-%m-%dT%H:%M:%SZ')] Starting AgyRemoteTele..." >> data/bot.log
    ./bin/agyremote >> data/bot.log 2>&1 || true
    echo "[$(date -u '+%Y-%m-%dT%H:%M:%SZ')] AgyRemoteTele exited, restarting in 2 seconds..." >> data/bot.log
    sleep 2
done
