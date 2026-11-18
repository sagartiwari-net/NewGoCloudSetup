#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "$0")"
cp -f config.json /tmp/nxaud1-config.check.json >/dev/null
go test ./...
go build -o nxaud1 .
echo "NoxTools Site Audit 1 local → http://127.0.0.1:6315/siteaudit/"
exec ./nxaud1
