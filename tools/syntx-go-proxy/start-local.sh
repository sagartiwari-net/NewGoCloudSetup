#!/bin/bash
set -e
cd "$(dirname "$0")"
export SYNTX_CONFIG="config.local.json"
cp config.local.json config.json

echo "Building SYNTX proxy..."
go build -o syntx-go-proxy .

PORT=$(python3 -c "import json; print(json.load(open('config.local.json')).get('port','4981'))")
if lsof -tiTCP:"$PORT" -sTCP:LISTEN >/dev/null 2>&1; then
  echo "Killing old process on :$PORT ..."
  lsof -tiTCP:"$PORT" -sTCP:LISTEN | xargs kill -9 2>/dev/null || true
  sleep 1
fi

echo "=========================================="
echo "  SYNTX LOCAL"
echo "  Open:   http://localhost:$PORT/video/runway"
echo "  Target: https://syntx.ai"
echo "  Session: cookie.txt (GoAuto localStorage; referer optional)"
echo "=========================================="
exec ./syntx-go-proxy
