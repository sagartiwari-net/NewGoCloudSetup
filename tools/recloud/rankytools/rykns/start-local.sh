#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "$0")"
go test ./...
go build -o rykns .
echo "RankyTools Niche Scraper local → http://127.0.0.1:6409/"
echo "cookie: cookie.json (same PHPSESSID)"
exec ./rykns
