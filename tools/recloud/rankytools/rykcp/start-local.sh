#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "$0")"
go test ./...
go build -o rykcp .
echo "RankyTools ChatGPT local → http://127.0.0.1:6403/"
echo "cookie: cookie.json (same PHPSESSID)"
exec ./rykcp
