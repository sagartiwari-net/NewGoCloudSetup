#!/bin/bash
set -e
cd "$(dirname "$0")"
export SPEECHIFY_CONFIG="config.local.json"
cp config.local.json config.json

echo "Building Speechify proxy..."
go build -o speechify-go-proxy .

PORT=$(python3 -c "import json; print(json.load(open('config.local.json')).get('port','5011'))")
if lsof -tiTCP:"$PORT" -sTCP:LISTEN >/dev/null 2>&1; then
  echo "Killing old process on :$PORT ..."
  lsof -tiTCP:"$PORT" -sTCP:LISTEN | xargs kill -9 2>/dev/null || true
  sleep 1
fi

echo "=========================================="
echo "  Speechify LOCAL"
echo "  Open:   http://localhost:$PORT/"
echo "  Target: https://app.speechify.com"
echo "  Session: cookie.txt (JSON array or GoAuto cookies; referer optional)"
echo "=========================================="
exec ./speechify-go-proxy
