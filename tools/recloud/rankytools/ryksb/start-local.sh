#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "$0")"
go test ./...
go build -o ryksb .
echo "RankyTools Storyblocks local → http://127.0.0.1:6407/"
echo "cookie: cookie.json (same PHPSESSID)"
exec ./ryksb
