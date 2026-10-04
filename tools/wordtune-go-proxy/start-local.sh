#!/bin/bash
set -e
cd "$(dirname "$0")"
export WORDTUNE_CONFIG="config.local.json"
cp config.local.json config.json

echo "Building Wordtune proxy..."
go build -o wordtune-go-proxy .

PORT=$(python3 -c "import json; print(json.load(open('config.local.json')).get('port','4631'))")
if lsof -tiTCP:"$PORT" -sTCP:LISTEN >/dev/null 2>&1; then
  echo "Killing old process on :$PORT ..."
  lsof -tiTCP:"$PORT" -sTCP:LISTEN | xargs kill -9 2>/dev/null || true
  sleep 1
fi

echo "=========================================="
echo "  Wordtune LOCAL"
echo "  Open:   http://localhost:$PORT/editor/"
echo "  Target: https://app.wordtune.com"
echo "  Cookies: cookie.txt (JSON array + GoAuto)"
echo "=========================================="
exec ./wordtune-go-proxy
