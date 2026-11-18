#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "$0")"
cp -f config.json /tmp/nxaud3-config.check.json >/dev/null
go test ./...
go build -o nxaud3 .
echo "NoxTools Site Audit 3 local → http://127.0.0.1:6317/siteaudit/"
exec ./nxaud3
