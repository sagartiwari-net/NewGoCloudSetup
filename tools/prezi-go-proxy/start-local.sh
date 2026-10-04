#!/bin/bash
set -e
cd "$(dirname "$0")"
export PREZI_CONFIG="config.local.json"
cp config.local.json config.json

echo "Building Prezi proxy..."
go build -o prezi-go-proxy .

PORT=$(python3 -c "import json; print(json.load(open('config.local.json')).get('port','4751'))")
if lsof -tiTCP:"$PORT" -sTCP:LISTEN >/dev/null 2>&1; then
  echo "Killing old process on :$PORT ..."
  lsof -tiTCP:"$PORT" -sTCP:LISTEN | xargs kill -9 2>/dev/null || true
  sleep 1
fi

echo "=========================================="
echo "  Prezi LOCAL"
echo "  Open:   http://localhost:$PORT/dashboard/next/#/all"
echo "  Target: https://prezi.com"
echo "  Cookies: cookie.txt (JSON array + GoAuto; referer optional)"
echo "=========================================="
exec ./prezi-go-proxy
