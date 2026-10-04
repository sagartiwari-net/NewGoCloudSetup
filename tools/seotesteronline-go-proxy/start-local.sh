#!/bin/bash
set -euo pipefail
cd "$(dirname "$0")"
export PATH="/usr/bin:/bin:/usr/sbin:/sbin:/opt/homebrew/bin:/usr/local/bin:/usr/local/go/bin:$PATH"

if [[ ! -s cookie.txt ]]; then
  echo "cookie.txt empty — login cookie comes from the panel database"
fi

go build -o seotesteronline-go-proxy .
PORT=$(python3 -c "import json; print(json.load(open('config.local.json')).get('port','4801'))")
lsof -tiTCP:${PORT} -sTCP:LISTEN | xargs kill -9 2>/dev/null || true
sleep 1

export CONFIG_FILE="$(pwd)/config.local.json"
cp config.local.json config.json
echo "=========================================="
echo "  SEO Tester Online LOCAL — :${PORT}"
echo "  Open: http://localhost:${PORT}/"
echo "  Auth: cookie.txt (GoAuto localStorage; referer optional)"
echo "=========================================="
exec ./seotesteronline-go-proxy
