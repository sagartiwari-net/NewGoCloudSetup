#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "$0")"
go test ./...
go build -o rykgm .
echo "RankyTools Grammarly local → http://127.0.0.1:6405/"
echo "cookie: cookie.json (same PHPSESSID)"
exec ./rykgm
