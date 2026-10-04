#!/bin/bash
set -e
cd "$(dirname "$0")"
export FISH_CONFIG="config.local.json"
cp config.local.json config.json

echo "Building Fish Audio proxy..."
go build -o fishaudio-go-proxy .

PORT=$(python3 -c "import json; print(json.load(open('config.local.json')).get('port','4991'))")
if lsof -tiTCP:"$PORT" -sTCP:LISTEN >/dev/null 2>&1; then
  echo "Killing old process on :$PORT ..."
  lsof -tiTCP:"$PORT" -sTCP:LISTEN | xargs kill -9 2>/dev/null || true
  sleep 1
fi

echo "=========================================="
echo "  Fish Audio LOCAL"
echo "  Open:   http://localhost:$PORT/app/"
echo "  Target: https://fish.audio"
echo "  Session: cookie.txt (GoAuto localStorage; referer optional)"
echo "=========================================="
exec ./fishaudio-go-proxy
