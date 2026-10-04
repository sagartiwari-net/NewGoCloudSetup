#!/bin/bash
set -e
cd "$(dirname "$0")"
export MINVO_CONFIG="config.local.json"
cp config.local.json config.json

echo "Building Minvo proxy..."
go build -o minvo-go-proxy .

PORT=$(python3 -c "import json; print(json.load(open('config.local.json')).get('port','4871'))")
if lsof -tiTCP:"$PORT" -sTCP:LISTEN >/dev/null 2>&1; then
  echo "Killing old process on :$PORT ..."
  lsof -tiTCP:"$PORT" -sTCP:LISTEN | xargs kill -9 2>/dev/null || true
  sleep 1
fi

echo "=========================================="
echo "  Minvo LOCAL"
echo "  Open:   http://localhost:$PORT/84368/episodes"
echo "  Target: https://app.minvo.pro"
echo "  Session: cookie.txt (GoAuto localStorage; referer optional)"
echo "=========================================="
exec ./minvo-go-proxy
