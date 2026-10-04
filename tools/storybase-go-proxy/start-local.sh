#!/bin/bash
set -e
cd "$(dirname "$0")"
export STORYBASE_CONFIG="config.local.json"
cp config.local.json config.json

echo "Building StoryBase proxy..."
go build -o storybase-go-proxy .

PORT=$(python3 -c "import json; print(json.load(open('config.local.json')).get('port','4551'))")
if lsof -tiTCP:"$PORT" -sTCP:LISTEN >/dev/null 2>&1; then
  echo "Killing old process on :$PORT ..."
  lsof -tiTCP:"$PORT" -sTCP:LISTEN | xargs kill -9 2>/dev/null || true
  sleep 1
fi

echo "=========================================="
echo "  StoryBase LOCAL"
echo "  Open:   http://127.0.0.1:$PORT/"
echo "  Target: https://www.storybase.com"
echo "  Cookies: cookie.txt (JSON array + GoAuto)"
echo "=========================================="
exec ./storybase-go-proxy
