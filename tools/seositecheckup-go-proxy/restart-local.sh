#!/bin/bash
# Restart local SEO Site Checkup proxy — run in Cursor terminal (not Mac Terminal.app)
set -e
cd "$(dirname "$0")"
export SEOSITE_CONFIG="${SEOSITE_CONFIG:-config.local.json}"
PORT=$(python3 -c "import json;print(json.load(open('$SEOSITE_CONFIG')).get('port','4661'))" 2>/dev/null || echo 4661)
echo "Stopping old proxy on :$PORT..."
lsof -ti:"$PORT" | xargs kill -9 2>/dev/null || true
sleep 1
echo "Building..."
go build -o seositecheckup-proxy .
echo "Starting on :$PORT (config=$SEOSITE_CONFIG) — http://localhost:$PORT/dashboard"
exec ./seositecheckup-proxy
