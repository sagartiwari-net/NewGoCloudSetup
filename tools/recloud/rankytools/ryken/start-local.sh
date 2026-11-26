#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "$0")"
go test ./...
go build -o ryken .
echo "RankyTools Envato Elements local → http://127.0.0.1:6406/"
echo "cookie: cookie.json (same PHPSESSID)"
exec ./ryken
