#!/bin/bash
set -e
cd "$(dirname "$0")"

# Server: config.server.json → config.json (cookie.txt mode, port 8786)
if [ ! -f config.json ] && [ -f config.server.json ]; then
  cp config.server.json config.json
fi

export CONFIG_FILE="${CONFIG_FILE:-config.json}"
PORT=$(grep -o '"port"[[:space:]]*:[[:space:]]*"[^"]*"' "$CONFIG_FILE" | grep -o '[0-9]\+' | head -1)
PORT=${PORT:-8786}

echo "Using config: $CONFIG_FILE"
echo "Killing port $PORT..."
lsof -ti :"$PORT" | xargs kill -9 2>/dev/null || fuser -k "$PORT/tcp" 2>/dev/null || true
sleep 1

echo "Building..."
go build -o erank-go-proxy .

echo ""
echo "=========================================="
echo "  eRank SERVER"
echo "  Domain: erank.1clkaccess.store"
echo "  Port:   $PORT (nginx → 127.0.0.1:$PORT)"
echo "  Cookies: cookie.txt"
echo "=========================================="
echo ""
exec ./erank-go-proxy
