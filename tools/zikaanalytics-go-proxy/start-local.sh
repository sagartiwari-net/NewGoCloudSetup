#!/bin/bash
set -e
cd "$(dirname "$0")"
export CONFIG_FILE="${CONFIG_FILE:-config.local.json}"
PORT=$(grep -o '"port"[[:space:]]*:[[:space:]]*"[^"]*"' "$CONFIG_FILE" | grep -o '[0-9]\+' | head -1)
PORT=${PORT:-5251}

echo "Using config: $CONFIG_FILE"
echo "Killing port $PORT..."
lsof -ti :"$PORT" | xargs kill -9 2>/dev/null || true
sleep 1

echo "Building..."
go build -o zikaanalytics-go-proxy .

echo ""
echo "=========================================="
echo "  Zik Analytics LOCAL dev server"
echo "  URL: http://127.0.0.1:$PORT"
echo "  Cookies: cookie.txt / panel DB"
echo "  (no /etc/hosts setup needed)"
echo "=========================================="
echo ""
exec ./zikaanalytics-go-proxy
