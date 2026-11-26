#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "$0")"
go test ./...
go build -o ryksz02 .
echo "RankyTools SpamZilla local → http://127.0.0.1:6410/private-domains/"
echo "cookie: cookie.json (same PHPSESSID)"
exec ./ryksz02
