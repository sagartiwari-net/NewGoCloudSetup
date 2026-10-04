#!/bin/bash
set -e
cd "$(dirname "$0")"
export LINKEDIN_LEARNING_CONFIG="config.local.json"
# Alias also accepted by main.go
export LYNDA_CONFIG="config.local.json"
cp config.local.json config.json

echo "Building LinkedIn Learning proxy..."
go build -o linkedin-learning-go-proxy .

PORT=$(python3 -c "import json; print(json.load(open('config.local.json')).get('port','4571'))")
if lsof -tiTCP:"$PORT" -sTCP:LISTEN >/dev/null 2>&1; then
  echo "Killing old process on :$PORT ..."
  lsof -tiTCP:"$PORT" -sTCP:LISTEN | xargs kill -9 2>/dev/null || true
  sleep 1
fi

echo "=========================================="
echo "  LinkedIn Learning (Lynda) LOCAL"
echo "  Open:   http://localhost:$PORT/learning/"
echo "  Target: https://www.linkedin.com/learning/"
echo "  Cookies: cookie.txt (JSON array + GoAuto)"
echo "=========================================="
exec ./linkedin-learning-go-proxy
