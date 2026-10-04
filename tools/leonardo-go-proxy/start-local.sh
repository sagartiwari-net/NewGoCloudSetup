#!/bin/bash
set -e
cd "$(dirname "$0")"
export LEONARDO_CONFIG="config.local.json"
cp config.local.json config.json

echo "Building Leonardo AI proxy..."
go build -o leonardo-go-proxy .

PORT=$(python3 -c "import json; print(json.load(open('config.local.json')).get('port','4601'))")
if lsof -tiTCP:"$PORT" -sTCP:LISTEN >/dev/null 2>&1; then
  echo "Killing old process on :$PORT ..."
  lsof -tiTCP:"$PORT" -sTCP:LISTEN | xargs kill -9 2>/dev/null || true
  sleep 1
fi

echo "=========================================="
echo "  Leonardo AI LOCAL"
echo "  Open:   http://localhost:$PORT/"
echo "  Target: https://app.leonardo.ai"
echo "  Cookies: cookie.txt (JSON array + GoAuto)"
echo "=========================================="
exec ./leonardo-go-proxy
