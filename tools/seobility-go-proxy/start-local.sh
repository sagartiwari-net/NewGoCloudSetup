#!/bin/bash
set -e
cd "$(dirname "$0")"
export SEOBILITY_CONFIG="config.local.json"
cp config.local.json config.json

echo "Building Seobility proxy..."
go build -o seobility-go-proxy .

PORT=$(python3 -c "import json; print(json.load(open('config.local.json')).get('port','4731'))")
if lsof -tiTCP:"$PORT" -sTCP:LISTEN >/dev/null 2>&1; then
  echo "Killing old process on :$PORT ..."
  lsof -tiTCP:"$PORT" -sTCP:LISTEN | xargs kill -9 2>/dev/null || true
  sleep 1
fi

echo "=========================================="
echo "  Seobility LOCAL"
echo "  Open:   http://localhost:$PORT/dashboard"
echo "  Target: https://app.seobility.net"
echo "  Cookies: cookie.txt (JSON array + GoAuto; referer optional)"
echo "=========================================="
exec ./seobility-go-proxy
