#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "$0")"
cp -f config.json /tmp/nxsm4-config.check.json >/dev/null
go test ./...
go build -o nxsm4 .
echo "NoxTools Semrush 4 local → http://127.0.0.1:6313/analytics/overview/?searchType=domain"
exec ./nxsm4
