#!/bin/bash
set -e
cd "$(dirname "$0")"
export EDUCATIVE_CONFIG="config.local.json"
cp config.local.json config.json

echo "Building Educative proxy..."
go build -o educative-go-proxy .

PORT=$(python3 -c "import json; print(json.load(open('config.local.json')).get('port','4851'))")
if lsof -tiTCP:"$PORT" -sTCP:LISTEN >/dev/null 2>&1; then
  echo "Killing old process on :$PORT ..."
  lsof -tiTCP:"$PORT" -sTCP:LISTEN | xargs kill -9 2>/dev/null || true
  sleep 1
fi

echo "=========================================="
echo "  Educative LOCAL"
echo "  Open:   http://localhost:$PORT/learn/home"
echo "  Target: https://www.educative.io"
echo "  Cookies: cookie.txt (JSON array + GoAuto; referer optional)"
echo "=========================================="
exec ./educative-go-proxy
