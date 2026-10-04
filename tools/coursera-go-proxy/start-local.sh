#!/bin/bash
# Coursera local — run in Cursor terminal
set -e
cd "$(dirname "$0")"
export PATH="/usr/local/go/bin:$PATH"
export CONFIG_FILE="${CONFIG_FILE:-config.local.json}"
PORT=$(python3 -c "import json;print(json.load(open('$CONFIG_FILE')).get('port','4701'))" 2>/dev/null || echo 4701)
echo "Stopping :$PORT ..."
lsof -ti:"$PORT" | xargs kill -9 2>/dev/null || true
sleep 1
go build -o coursera-go-proxy .
echo "=========================================="
echo "  Coursera LOCAL  http://localhost:$PORT/organizations/reliance-family"
echo "  Config: $CONFIG_FILE"
echo "  Cookies: array + GoAuto (referer optional)"
echo "=========================================="
exec ./coursera-go-proxy
