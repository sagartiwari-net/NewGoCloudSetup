#!/bin/bash
set -e
cd "$(dirname "$0")"
export CONFIG_FILE="${CONFIG_FILE:-config.local.json}"
PORT=$(grep -o '"port"[[:space:]]*:[[:space:]]*"[^"]*"' "$CONFIG_FILE" | grep -o '[0-9]\+' | head -1)
PORT=${PORT:-7853}

echo "Using config: $CONFIG_FILE"
echo "Killing port $PORT..."
lsof -ti :"$PORT" | xargs kill -9 2>/dev/null || true
sleep 1

echo "Building..."
go build -o junglescout-go-proxy .

echo ""
echo "=========================================="
echo "  Jungle Scout LOCAL dev server"
echo "  Dashboard:  http://localhost:$PORT"
echo "  Extension:  http://localhost:$PORT/ext-install"
echo "  Login map:  http://localhost:$PORT/login-proxy"
echo "  Cookies:    cookie.txt"
echo "=========================================="
echo ""
exec ./junglescout-go-proxy
