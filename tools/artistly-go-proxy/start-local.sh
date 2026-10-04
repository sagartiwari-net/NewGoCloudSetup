#!/bin/bash
set -e
cd "$(dirname "$0")"
export ARTISTLY_CONFIG="config.local.json"
cp config.local.json config.json

echo "Building Artistly proxy..."
go build -o artistly-go-proxy .

PORT=$(python3 -c "import json; print(json.load(open('config.local.json')).get('port','4821'))")
if lsof -tiTCP:"$PORT" -sTCP:LISTEN >/dev/null 2>&1; then
  echo "Killing old process on :$PORT ..."
  lsof -tiTCP:"$PORT" -sTCP:LISTEN | xargs kill -9 2>/dev/null || true
  sleep 1
fi

echo "=========================================="
echo "  Artistly LOCAL"
echo "  Open:   http://localhost:$PORT/ai/ai-image-designer"
echo "  Target: https://app.artistly.ai"
echo "  Cookies: cookie.txt (JSON array + GoAuto; referer optional)"
echo "=========================================="
exec ./artistly-go-proxy
