#!/bin/bash
# Glorify local — run in Cursor terminal
set -e
cd "$(dirname "$0")"
export GLORIFY_CONFIG="${GLORIFY_CONFIG:-config.local.json}"
PORT=$(python3 -c "import json;print(json.load(open('$GLORIFY_CONFIG')).get('port','4681'))" 2>/dev/null || echo 4681)
echo "Stopping :$PORT ..."
lsof -ti:"$PORT" | xargs kill -9 2>/dev/null || true
sleep 1
go build -o glorify-go-proxy .
echo "=========================================="
echo "  Glorify LOCAL  http://localhost:$PORT/dashboard"
echo "  Config: $GLORIFY_CONFIG"
echo "  Cookies: array + GoAuto (referer optional)"
echo "=========================================="
exec ./glorify-go-proxy
