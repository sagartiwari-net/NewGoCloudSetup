#!/bin/bash
set -e
cd "$(dirname "$0")"
export MERCHINFORMER_CONFIG="config.local.json"
cp config.local.json config.json

echo "Building Merch Informer proxy..."
go build -o merchinformer-go-proxy .

PORT=$(python3 -c "import json; print(json.load(open('config.local.json')).get('port','4651'))")
if lsof -tiTCP:"$PORT" -sTCP:LISTEN >/dev/null 2>&1; then
  echo "Killing old process on :$PORT ..."
  lsof -tiTCP:"$PORT" -sTCP:LISTEN | xargs kill -9 2>/dev/null || true
  sleep 1
fi

echo "=========================================="
echo "  Merch Informer LOCAL"
echo "  Open:   http://localhost:$PORT/tutorials"
echo "  Target: https://members.merchinformer.com"
echo "  Cookies: cookie.txt (JSON array + GoAuto)"
echo "  NOTE: prefer localhost (not 127.0.0.1)"
echo "=========================================="
exec ./merchinformer-go-proxy
