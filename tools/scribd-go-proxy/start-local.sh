#!/bin/bash
set -e
cd "$(dirname "$0")"
export SCRIBD_CONFIG="config.local.json"
cp config.local.json config.json

echo "Building Scribd proxy..."
go build -o scribd-go-proxy .

PORT=$(python3 -c "import json; print(json.load(open('config.local.json')).get('port','4791'))")
if lsof -tiTCP:"$PORT" -sTCP:LISTEN >/dev/null 2>&1; then
  echo "Killing old process on :$PORT ..."
  lsof -tiTCP:"$PORT" -sTCP:LISTEN | xargs kill -9 2>/dev/null || true
  sleep 1
fi

echo "=========================================="
echo "  Scribd LOCAL"
echo "  Open:   http://localhost:$PORT/home"
echo "  Target: https://www.scribd.com"
echo "  Cookies: cookie.txt (JSON array + GoAuto; referer optional)"
echo "=========================================="
exec ./scribd-go-proxy
