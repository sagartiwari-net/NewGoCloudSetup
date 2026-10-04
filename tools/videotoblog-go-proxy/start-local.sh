#!/bin/bash
set -e
cd "$(dirname "$0")"
export VIDEOTOBLOG_CONFIG="config.local.json"
cp config.local.json config.json

echo "Building Video to Blog proxy..."
go build -o videotoblog-go-proxy .

PORT=$(python3 -c "import json; print(json.load(open('config.local.json')).get('port','5041'))")
if lsof -tiTCP:"$PORT" -sTCP:LISTEN >/dev/null 2>&1; then
  echo "Killing old process on :$PORT ..."
  lsof -tiTCP:"$PORT" -sTCP:LISTEN | xargs kill -9 2>/dev/null || true
  sleep 1
fi

echo "=========================================="
echo "  Video to Blog LOCAL"
echo "  Open:   http://localhost:$PORT/articles"
echo "  Target: https://videotoblog.ai"
echo "  Session: cookie.txt (GoAuto IndexedDB Firebase; referer optional)"
echo "=========================================="
exec ./videotoblog-go-proxy
