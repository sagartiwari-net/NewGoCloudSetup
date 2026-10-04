#!/bin/bash
set -e
cd "$(dirname "$0")"
export CANVA_CONFIG="config.local.json"
cp config.local.json config.json

echo "Building Mojo proxy..."
go build -o mojo-go-proxy .

PORT=$(python3 -c "import json; print(json.load(open('config.local.json')).get('port','4881'))")
if lsof -tiTCP:"$PORT" -sTCP:LISTEN >/dev/null 2>&1; then
  echo "Killing old process on :$PORT ..."
  lsof -tiTCP:"$PORT" -sTCP:LISTEN | xargs kill -9 2>/dev/null || true
  sleep 1
fi

echo "=========================================="
echo "  Mojo LOCAL"
echo "  Open:   http://localhost:$PORT/"
echo "  Target: https://app.mojo.video"
echo "  Auth:   IndexedDB Firebase (cookie.txt; referer optional)"
echo "=========================================="
exec ./mojo-go-proxy
