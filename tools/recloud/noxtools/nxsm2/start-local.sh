#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "$0")"
cp -f config.json /tmp/nxsm2-config.check.json >/dev/null
go test ./...
go build -o nxsm2 .
echo "NoxTools Semrush 2 local → http://127.0.0.1:6311/analytics/overview/?searchType=domain"
exec ./nxsm2
