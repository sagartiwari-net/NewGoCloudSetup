#!/bin/bash
set -e
cd "$(dirname "$0")"
export WORDHERO_CONFIG="${WORDHERO_CONFIG:-config.json}"
go build -o wordhero-proxy .
exec ./wordhero-proxy
