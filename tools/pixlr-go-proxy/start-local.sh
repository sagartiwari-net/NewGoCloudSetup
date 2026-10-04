#!/bin/bash
set -e
cd "$(dirname "$0")"
export PIXLR_CONFIG="config.local.json"
cp config.local.json config.json

echo "Building Pixlr proxy..."
go build -o pixlr-go-proxy .

PORT=$(python3 -c "import json; print(json.load(open('config.local.json')).get('port','4901'))")
if lsof -tiTCP:"$PORT" -sTCP:LISTEN >/dev/null 2>&1; then
  echo "Killing old process on :$PORT ..."
  lsof -tiTCP:"$PORT" -sTCP:LISTEN | xargs kill -9 2>/dev/null || true
  sleep 1
fi

echo "=========================================="
echo "  Pixlr LOCAL"
echo "  Open:   http://localhost:$PORT/"
echo "  Target: https://pixlr.com"
echo "  Cookies: cookie.txt (JSON array + GoAuto; referer optional)"
echo "=========================================="
exec ./pixlr-go-proxy
