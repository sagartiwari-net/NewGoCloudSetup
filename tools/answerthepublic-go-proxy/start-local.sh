#!/bin/bash
set -e
cd "$(dirname "$0")"
export ANSWERTHEPUBLIC_CONFIG="config.local.json"
cp config.local.json config.json

echo "Building AnswerThePublic proxy..."
go build -o answerthepublic-go-proxy .

PORT=$(python3 -c "import json; print(json.load(open('config.local.json')).get('port','4931'))")
if lsof -tiTCP:"$PORT" -sTCP:LISTEN >/dev/null 2>&1; then
  echo "Killing old process on :$PORT ..."
  lsof -tiTCP:"$PORT" -sTCP:LISTEN | xargs kill -9 2>/dev/null || true
  sleep 1
fi

echo "=========================================="
echo "  AnswerThePublic LOCAL"
echo "  Open:   http://localhost:$PORT/en/dashboard/hw3o2g/searches"
echo "  Target: https://answerthepublic.com"
echo "  Cookies: cookie.txt (JSON array + GoAuto; referer optional)"
echo "=========================================="
exec ./answerthepublic-go-proxy
