#!/bin/bash
set -euo pipefail
cd "$(dirname "$0")"
export PATH="/usr/bin:/bin:/usr/sbin:/sbin:/opt/homebrew/bin:/usr/local/bin:/usr/local/go/bin:$PATH"

if [[ ! -s cookie.txt ]]; then
  echo "cookie.txt empty — login cookie comes from the panel database"
fi

go build -o searchatlas-go-proxy .
PORT=4711
lsof -tiTCP:${PORT} -sTCP:LISTEN | xargs kill -9 2>/dev/null || true
sleep 1

export CONFIG_FILE="$(pwd)/config.local.json"
echo "=========================================="
echo "  SearchAtlas LOCAL — :${PORT}"
echo "  Open: http://localhost:${PORT}/home"
echo "  Auth: cookie.txt (GoAuto localStorage JSON)"
echo "=========================================="
exec ./searchatlas-go-proxy
