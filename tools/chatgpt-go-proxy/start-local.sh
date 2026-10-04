#!/bin/bash
set -e
cd "$(dirname "$0")"

# chatgpt proxy always reads config.json — use local for this run
cp config.local.json config.json

echo "Building ChatGPT proxy..."
go build -o chatgpt-proxy .

# free port if leftover process
PORT=$(python3 -c "import json; print(json.load(open('config.local.json')).get('port','6041'))")
if lsof -tiTCP:"$PORT" -sTCP:LISTEN >/dev/null 2>&1; then
  echo "Killing old process on :$PORT ..."
  lsof -tiTCP:"$PORT" -sTCP:LISTEN | xargs kill -9 2>/dev/null || true
  sleep 1
fi

echo "=========================================="
echo "  ChatGPT LOCAL"
echo "  Open:  http://127.0.0.1:$PORT"
echo "  Access: panel token only (cookie from panel database)"
echo "=========================================="
exec ./chatgpt-proxy
