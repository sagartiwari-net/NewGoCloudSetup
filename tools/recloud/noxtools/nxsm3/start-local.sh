#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "$0")"
cp -f config.json /tmp/nxsm3-config.check.json >/dev/null
go test ./...
go build -o nxsm3 .
echo "NoxTools Semrush 3 local → http://127.0.0.1:6312/analytics/overview/?searchType=domain"
exec ./nxsm3
