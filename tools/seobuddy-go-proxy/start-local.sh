#!/bin/bash
set -e
cd "$(dirname "$0")"
export SEOBUDDY_CONFIG="config.local.json"
cp config.local.json config.json

echo "Building SEO Buddy proxy..."
go build -o seobuddy-go-proxy .

PORT=$(python3 -c "import json; print(json.load(open('config.local.json')).get('port','5131'))")
if lsof -tiTCP:"$PORT" -sTCP:LISTEN >/dev/null 2>&1; then
  echo "Killing old process on :$PORT ..."
  lsof -tiTCP:"$PORT" -sTCP:LISTEN | xargs kill -9 2>/dev/null || true
  sleep 1
fi

echo "=========================================="
echo "  SEO Buddy LOCAL"
echo "  Open:   http://localhost:$PORT/app/dashboard"
echo "  Target: https://seobuddy.com"
echo "  Session: cookie.txt (JSON array or GoAuto cookies; referer optional)"
echo "=========================================="
exec ./seobuddy-go-proxy
