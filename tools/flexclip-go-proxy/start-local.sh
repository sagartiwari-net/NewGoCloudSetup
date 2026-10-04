#!/bin/bash
# FlexClip local — run in Cursor terminal
set -e
cd "$(dirname "$0")"
export FLEXCLIP_CONFIG="${FLEXCLIP_CONFIG:-config.local.json}"
PORT=$(python3 -c "import json;print(json.load(open('$FLEXCLIP_CONFIG')).get('port','4671'))" 2>/dev/null || echo 4671)
echo "Stopping :$PORT ..."
lsof -ti:"$PORT" | xargs kill -9 2>/dev/null || true
sleep 1
go build -o flexclip-go-proxy .
echo "=========================================="
echo "  FlexClip LOCAL  http://localhost:$PORT/editor/"
echo "  Config: $FLEXCLIP_CONFIG"
echo "  Cookies: array + GoAuto (referer optional)"
echo "=========================================="
exec ./flexclip-go-proxy
