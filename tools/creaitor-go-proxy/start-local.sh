#!/bin/bash
set -e
cd "$(dirname "$0")"
export CREAITOR_CONFIG="config.local.json"
cp config.local.json config.json

echo "Building Creaitor AI proxy..."
go build -o creaitor-go-proxy .

PORT=$(python3 -c "import json; print(json.load(open('config.local.json')).get('port','4621'))")
if lsof -tiTCP:"$PORT" -sTCP:LISTEN >/dev/null 2>&1; then
  echo "Killing old process on :$PORT ..."
  lsof -tiTCP:"$PORT" -sTCP:LISTEN | xargs kill -9 2>/dev/null || true
  sleep 1
fi

echo "=========================================="
echo "  Creaitor AI LOCAL"
echo "  Open:   http://localhost:$PORT/home"
echo "  Target: https://app.creaitor.ai"
echo "  Cookies: cookie.txt (JSON array + GoAuto)"
echo "  NOTE: use localhost (not 127.0.0.1) — host allowlist"
echo "=========================================="
exec ./creaitor-go-proxy
