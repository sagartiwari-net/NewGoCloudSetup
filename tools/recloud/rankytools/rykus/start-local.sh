#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "$0")"
go test ./...
go build -o rykus .
echo "RankyTools Ubersuggest local → http://127.0.0.1:6404/en/dashboard"
echo "cookie: cookie.json (same PHPSESSID)"
exec ./rykus
