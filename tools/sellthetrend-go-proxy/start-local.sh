#!/bin/bash
set -e
cd "$(dirname "$0")"
export SELLTHETREND_CONFIG="config.local.json"
export STT_CONFIG="config.local.json"
cp config.local.json config.json

echo "Building Sell The Trend proxy..."
go build -o sellthetrend-go-proxy .

PORT=$(python3 -c "import json; print(json.load(open('config.local.json')).get('port','4591'))")
if lsof -tiTCP:"$PORT" -sTCP:LISTEN >/dev/null 2>&1; then
  echo "Killing old process on :$PORT ..."
  lsof -tiTCP:"$PORT" -sTCP:LISTEN | xargs kill -9 2>/dev/null || true
  sleep 1
fi

echo "=========================================="
echo "  Sell The Trend LOCAL"
echo "  Open:   http://localhost:$PORT/dashboard/desk"
echo "  Target: https://www.sellthetrend.com"
echo "  Cookies: cookie.txt (JSON array + GoAuto)"
echo "=========================================="
exec ./sellthetrend-go-proxy
