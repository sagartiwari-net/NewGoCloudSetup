#!/bin/bash
set -e
cd "$(dirname "$0")"
export WRITEHUMAN_CONFIG="${WRITEHUMAN_CONFIG:-config.json}"
go build -o writehuman-proxy .
exec ./writehuman-proxy
