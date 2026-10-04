#!/bin/bash
set -e
cd "$(dirname "$0")"
export CHATBOT_CONFIG="config.local.json"
cp config.local.json config.json

echo "Building Chatbot App proxy..."
go build -o chatbotapp-go-proxy .

PORT=$(python3 -c "import json; print(json.load(open('config.local.json')).get('port','5001'))")
if lsof -tiTCP:"$PORT" -sTCP:LISTEN >/dev/null 2>&1; then
  echo "Killing old process on :$PORT ..."
  lsof -tiTCP:"$PORT" -sTCP:LISTEN | xargs kill -9 2>/dev/null || true
  sleep 1
fi

echo "=========================================="
echo "  Chatbot App LOCAL"
echo "  Open:   http://localhost:$PORT/?model=auto"
echo "  Target: https://chat.chatbotapp.ai"
echo "  Session: cookie.txt (GoAuto IndexedDB Firebase; referer optional)"
echo "=========================================="
exec ./chatbotapp-go-proxy
