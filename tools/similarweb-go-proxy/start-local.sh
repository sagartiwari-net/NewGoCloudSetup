#!/bin/bash
set -e
cd "$(dirname "$0")"
export SIMILARWEB_CONFIG="config.local.json"
cp config.local.json config.json

echo "Building Similarweb proxy..."
go build -o similarweb-go-proxy .

PORT=$(python3 -c "import json; print(json.load(open('config.local.json')).get('port','5061'))")
if lsof -tiTCP:"$PORT" -sTCP:LISTEN >/dev/null 2>&1; then
  echo "Killing old process on :$PORT ..."
  lsof -tiTCP:"$PORT" -sTCP:LISTEN | xargs kill -9 2>/dev/null || true
  sleep 1
fi

echo "=========================================="
echo "  Similarweb LOCAL"
echo "  Open:   http://localhost:$PORT/#/dashboard/gallery"
echo "  Target: https://pro.similarweb.com"
echo "  Session: cookie.txt (JSON array or GoAuto cookies; referer optional)"
echo "=========================================="
exec ./similarweb-go-proxy
