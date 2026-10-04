#!/bin/bash
set -e
cd "$(dirname "$0")"
export CANVA_CONFIG="config.local.json"
cp config.local.json config.json

echo "Building Jasper proxy..."
go build -o jasper-go-proxy .

PORT=$(python3 -c "import json; print(json.load(open('config.local.json')).get('port','4511'))")
if lsof -tiTCP:"$PORT" -sTCP:LISTEN >/dev/null 2>&1; then
  echo "Killing old process on :$PORT ..."
  lsof -tiTCP:"$PORT" -sTCP:LISTEN | xargs kill -9 2>/dev/null || true
  sleep 1
fi

echo "=========================================="
echo "  Jasper LOCAL"
echo "  Open:   http://127.0.0.1:$PORT/"
echo "  Target: https://app.jasper.ai"
echo "  Auth:   IndexedDB (cookie.txt)"
echo "=========================================="
exec ./jasper-go-proxy
