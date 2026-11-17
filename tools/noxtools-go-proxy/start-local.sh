#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "$0")"
cp -f config.local.json config.json
go build -o noxtools-go-proxy .
PORT=$(python3 -c "import json; print(json.load(open('config.local.json')).get('port','6309'))")
echo "NoxTools Home local → http://127.0.0.1:${PORT}/secure/member"
exec ./noxtools-go-proxy
