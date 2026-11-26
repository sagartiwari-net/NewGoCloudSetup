#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "$0")"
go test ./...
go build -o ryksm1 .
echo "RankyTools Semrush local → http://127.0.0.1:6401/home/"
echo "cookie: cookie.json (PHPSESSID)"
exec ./ryksm1
