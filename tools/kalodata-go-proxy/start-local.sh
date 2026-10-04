#!/bin/bash
set -e
cd "$(dirname "$0")"
export KALODATA_CONFIG="config.local.json"
cp config.local.json config.json

echo "Building Kalodata proxy..."
go build -o kalodata-go-proxy .

PORT=$(python3 -c "import json; print(json.load(open('config.local.json')).get('port','4951'))")
if lsof -tiTCP:"$PORT" -sTCP:LISTEN >/dev/null 2>&1; then
  echo "Killing old process on :$PORT ..."
  lsof -tiTCP:"$PORT" -sTCP:LISTEN | xargs kill -9 2>/dev/null || true
  sleep 1
fi

echo "=========================================="
echo "  Kalodata LOCAL"
echo "  Open:   http://localhost:$PORT/explore"
echo "  Target: https://www.kalodata.com"
echo "  Cookies: cookie.txt (JSON array + GoAuto; referer optional)"
echo "=========================================="
exec ./kalodata-go-proxy
