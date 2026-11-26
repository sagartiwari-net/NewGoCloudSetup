#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "$0")"
go test ./...
go build -o ryksw .
echo "RankyTools SimilarWeb local → http://127.0.0.1:6411/"
echo "cookie: cookie.json (same PHPSESSID)"
exec ./ryksw
