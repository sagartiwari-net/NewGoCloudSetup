#!/bin/bash
set -e
cd "$(dirname "$0")"
export UNCENSOREDCHAT_CONFIG="config.local.json"
cp config.local.json config.json

echo "Building Uncensored Chat proxy..."
go build -o uncensoredchat-go-proxy .

PORT=$(python3 -c "import json; print(json.load(open('config.local.json')).get('port','4891'))")
if lsof -tiTCP:"$PORT" -sTCP:LISTEN >/dev/null 2>&1; then
  echo "Killing old process on :$PORT ..."
  lsof -tiTCP:"$PORT" -sTCP:LISTEN | xargs kill -9 2>/dev/null || true
  sleep 1
fi

echo "=========================================="
echo "  Uncensored Chat LOCAL"
echo "  Open:   http://localhost:$PORT/"
echo "  Target: https://uncensored.chat"
echo "  Cookies: cookie.txt (JSON array + GoAuto; referer optional)"
echo "=========================================="
exec ./uncensoredchat-go-proxy
