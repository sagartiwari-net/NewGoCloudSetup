#!/bin/bash
# Restart local ClosersCopy proxy (Mac)
set -e
cd "$(dirname "$0")"
echo "Stopping old proxy on :6022..."
lsof -ti:6022 | xargs kill -9 2>/dev/null || true
sleep 1
echo "Building..."
go build -o closerscopy-proxy .
echo "Starting (build tag: closerscopy-v7)..."
./closerscopy-proxy
