#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "$0")"
cp -f config.json /tmp/nxsm1-config.check.json >/dev/null
go test ./...
go build -o nxsm1 .
echo "NoxTools Semrush 1 local → http://127.0.0.1:6310/analytics/overview/?searchType=domain"
exec ./nxsm1
