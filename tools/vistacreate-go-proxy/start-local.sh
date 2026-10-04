#!/bin/bash
set -e
cd "$(dirname "$0")"
export VISTACREATE_CONFIG="config.local.json"
export VISTA_CONFIG="config.local.json"
cp config.local.json config.json

echo "Building VistaCreate proxy..."
go build -o vistacreate-go-proxy .

PORT=$(python3 -c "import json; print(json.load(open('config.local.json')).get('port','4581'))")
if lsof -tiTCP:"$PORT" -sTCP:LISTEN >/dev/null 2>&1; then
  echo "Killing old process on :$PORT ..."
  lsof -tiTCP:"$PORT" -sTCP:LISTEN | xargs kill -9 2>/dev/null || true
  sleep 1
fi

echo "=========================================="
echo "  VistaCreate LOCAL"
echo "  Open:   http://localhost:$PORT/home/"
echo "  Target: https://create.vista.com"
echo "  Cookies: cookie.txt (JSON array + GoAuto)"
echo "=========================================="
exec ./vistacreate-go-proxy
