#!/bin/bash
set -e
cd "$(dirname "$0")"
export RIVALFLOW_CONFIG="config.local.json"
cp config.local.json config.json

echo "Building RivalFlow proxy..."
go build -o rivalflow-go-proxy .

PORT=$(python3 -c "import json; print(json.load(open('config.local.json')).get('port','4811'))")
if lsof -tiTCP:"$PORT" -sTCP:LISTEN >/dev/null 2>&1; then
  echo "Killing old process on :$PORT ..."
  lsof -tiTCP:"$PORT" -sTCP:LISTEN | xargs kill -9 2>/dev/null || true
  sleep 1
fi

echo "=========================================="
echo "  RivalFlow LOCAL"
echo "  Open:   http://localhost:$PORT/actionflow"
echo "  Target: https://app.rivalflow.com"
echo "  Cookies: cookie.txt (JSON array + GoAuto)"
echo "  NOTE: use localhost (not 127.0.0.1)"
echo "=========================================="
exec ./rivalflow-go-proxy
