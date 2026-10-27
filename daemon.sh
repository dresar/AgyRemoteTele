#!/usr/bin/env bash
cd /opt/data/home/agyremote || exit 1
while true; do
  node server/index.ts >> /opt/data/home/agyremote/server.log 2>&1
  sleep 1
done
