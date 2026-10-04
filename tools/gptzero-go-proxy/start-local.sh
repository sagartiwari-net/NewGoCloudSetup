#!/bin/bash
set -e
cd "$(dirname "$0")"
export GPTZERO_CONFIG="${GPTZERO_CONFIG:-config.local.json}"
cp -f "$GPTZERO_CONFIG" config.json
go build -o gptzero-proxy .
exec ./gptzero-proxy
