#!/bin/bash
set -e
cd "$(dirname "$0")"
export MAGNIFIC_CONFIG="config.local.json"
cp config.local.json config.json

echo "Building Magnific proxy..."
go build -o magnific-go-proxy .

PORT=$(python3 -c "import json; print(json.load(open('config.local.json')).get('port','5121'))")
if lsof -tiTCP:"$PORT" -sTCP:LISTEN >/dev/null 2>&1; then
  echo "Killing old process on :$PORT ..."
  lsof -tiTCP:"$PORT" -sTCP:LISTEN | xargs kill -9 2>/dev/null || true
  sleep 1
fi

echo "=========================================="
echo "  Magnific LOCAL"
echo "  Open:   http://localhost:$PORT/app"
echo "  Target: https://www.magnific.com"
echo "  Session: cookie.txt (JSON array or GoAuto cookies; referer optional)"
echo "=========================================="
exec ./magnific-go-proxy
