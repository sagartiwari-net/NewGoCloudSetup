#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "$0")"
go test ./...
go build -o rykmj .
echo "RankyTools Majestic local → http://127.0.0.1:6402/"
echo "cookie: cookie.json (same PHPSESSID as Semrush)"
exec ./rykmj
