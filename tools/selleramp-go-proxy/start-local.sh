#!/bin/bash
set -e
cd "$(dirname "$0")"
export SELLERAMP_CONFIG="config.local.json"
# also copy for proxies that only read config.json
cp config.local.json config.json
go build -o selleramp-go-proxy .
PORT=$(python3 -c "import json; print(json.load(open('config.local.json')).get('port','7852'))")
if lsof -tiTCP:"$PORT" -sTCP:LISTEN >/dev/null 2>&1; then
  echo "Killing old process on :$PORT ..."
  lsof -tiTCP:"$PORT" -sTCP:LISTEN | xargs kill -9 2>/dev/null || true
  sleep 1
fi
echo "SellerAmp local proxy: http://127.0.0.1:$PORT"
exec ./selleramp-go-proxy
