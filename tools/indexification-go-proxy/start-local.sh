#!/bin/bash
set -e
cd "$(dirname "$0")"
PORT=$(grep -o '"port"[[:space:]]*:[[:space:]]*"[^"]*"' config.json | grep -o '[0-9]\+' | head -1)
PORT=${PORT:-7867}

echo "Killing port $PORT..."
lsof -ti :"$PORT" | xargs kill -9 2>/dev/null || true
sleep 1

echo "Building..."
go build -o indexification-go-proxy .

echo "Starting http://localhost:$PORT → http://www.indexification.com"
exec ./indexification-go-proxy
