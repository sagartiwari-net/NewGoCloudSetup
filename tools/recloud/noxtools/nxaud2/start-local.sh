#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "$0")"
cp -f config.json /tmp/nxaud2-config.check.json >/dev/null
go test ./...
go build -o nxaud2 .
echo "NoxTools Site Audit 2 local → http://127.0.0.1:6316/siteaudit/"
exec ./nxaud2
