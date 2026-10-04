#!/bin/bash
set -e
cd "$(dirname "$0")"
PORT=$(grep -o '"port"[[:space:]]*:[[:space:]]*"[^"]*"' config.json | grep -o '[0-9]\+' | head -1)
PORT=${PORT:-7872}

echo "Killing port $PORT..."
lsof -ti :"$PORT" | xargs kill -9 2>/dev/null || true
sleep 1

echo "Building..."
go mod tidy
go build -o claude-ai-go-proxy .

echo ""
echo "Starting http://localhost:$PORT → https://claude.ai"
echo "Open:  http://localhost:$PORT/new"
echo ""
exec ./claude-ai-go-proxy
