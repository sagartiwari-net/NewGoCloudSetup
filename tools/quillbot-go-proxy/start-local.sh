#!/bin/bash
set -e
cd "$(dirname "$0")"
export CANVA_CONFIG="config.local.json"
cp config.local.json config.json

echo "Building Quillbot proxy..."
go build -o quillbot-go-proxy .

PORT=$(python3 -c "import json; print(json.load(open('config.local.json')).get('port','5311'))")
if lsof -tiTCP:"$PORT" -sTCP:LISTEN >/dev/null 2>&1; then
  echo "Killing old process on :$PORT ..."
  lsof -tiTCP:"$PORT" -sTCP:LISTEN | xargs kill -9 2>/dev/null || true
  sleep 1
fi

echo "=========================================="
echo "  Quillbot LOCAL"
echo "  Open:   http://127.0.0.1:$PORT/"
echo "  Target: https://quillbot.com"
echo "  Auth:   IndexedDB (cookie.txt)"
echo "=========================================="
exec ./quillbot-go-proxy
