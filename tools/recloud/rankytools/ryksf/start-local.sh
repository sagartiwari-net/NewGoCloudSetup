#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "$0")"
go test ./...
go build -o ryksf .
echo "RankyTools SpyFu local → http://127.0.0.1:6408/"
echo "cookie: cookie.json (same PHPSESSID)"
exec ./ryksf
