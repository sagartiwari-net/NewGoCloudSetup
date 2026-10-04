#!/bin/bash
set -e

cd "$(dirname "$0")"
export H10_CONFIG="config.local.json"

echo "Using config: $H10_CONFIG"
if command -v lsof >/dev/null 2>&1; then
  PIDS=$(lsof -tiTCP:5201 -sTCP:LISTEN 2>/dev/null || true)
  if [ -n "$PIDS" ]; then
    echo "Killing port 5201..."
    kill $PIDS 2>/dev/null || true
    sleep 0.4
  fi
fi

echo "Building..."
go build -o helium10-go-proxy .
echo "Helium10 local proxy: http://127.0.0.1:5201"
exec ./helium10-go-proxy
