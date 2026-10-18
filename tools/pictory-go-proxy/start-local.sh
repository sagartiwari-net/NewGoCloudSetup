#!/bin/bash
set -e
cd "$(dirname "$0")"
export PICTORY_CONFIG="config.local.json"
if [[ ! -f config.local.json ]]; then
  echo "Missing config.local.json"
  exit 1
fi
cp config.local.json config.json

echo "Building Pictory proxy..."
go build -o pictory-go-proxy .

PORT=$(python3 -c "import json; print(json.load(open('config.local.json')).get('port','5331'))")
if lsof -tiTCP:"$PORT" -sTCP:LISTEN >/dev/null 2>&1; then
  echo "Killing old process on :$PORT ..."
  lsof -tiTCP:"$PORT" -sTCP:LISTEN | xargs kill -9 2>/dev/null || true
  sleep 1
fi

echo "=========================================="
echo "  Pictory LOCAL"
echo "  Open:   http://127.0.0.1:$PORT/"
echo "  Target: https://app.pictory.ai"
echo "  Session: cookie file (flat localStorage or referer export)"
echo "=========================================="
exec ./pictory-go-proxy
