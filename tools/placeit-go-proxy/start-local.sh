#!/bin/bash
set -e
cd "$(dirname "$0")"
export PLACEIT_CONFIG="config.local.json"
cp config.local.json config.json

echo "Building Placeit proxy..."
go build -o placeit-go-proxy .

PORT=$(python3 -c "import json; print(json.load(open('config.local.json')).get('port','4611'))")
if lsof -tiTCP:"$PORT" -sTCP:LISTEN >/dev/null 2>&1; then
  echo "Killing old process on :$PORT ..."
  lsof -tiTCP:"$PORT" -sTCP:LISTEN | xargs kill -9 2>/dev/null || true
  sleep 1
fi

echo "=========================================="
echo "  Placeit LOCAL"
echo "  Open:   http://localhost:$PORT/"
echo "  Target: https://placeit.net"
echo "  Cookies: cookie.txt (JSON array + GoAuto)"
echo "=========================================="
exec ./placeit-go-proxy
