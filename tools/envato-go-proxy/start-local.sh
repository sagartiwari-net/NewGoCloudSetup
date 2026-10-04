#!/bin/bash
set -e
cd "$(dirname "$0")"
export CONFIG_FILE="${CONFIG_FILE:-config.local.json}"
PORT=$(python3 -c "import json; print(json.load(open('$CONFIG_FILE')).get('port','5261'))")

echo "Using config: $CONFIG_FILE"
echo "Killing port $PORT..."
lsof -tiTCP:"$PORT" -sTCP:LISTEN | xargs kill -9 2>/dev/null || true
sleep 1

echo "Building Envato proxy..."
go build -o envato-go-proxy .

echo "=========================================="
echo "  Envato LOCAL (panel security)"
echo "  Direct:  http://127.0.0.1:$PORT  → Access Denied"
echo "  Access:  panel Open → /access?user=&token="
echo "=========================================="
exec ./envato-go-proxy
