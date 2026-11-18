#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "$0")"
cp -f config.json /tmp/nxsm5-config.check.json >/dev/null
go test ./...
go build -o nxsm5 .
echo "NoxTools Semrush 5 local → http://127.0.0.1:6314/analytics/overview/?searchType=domain"
exec ./nxsm5
