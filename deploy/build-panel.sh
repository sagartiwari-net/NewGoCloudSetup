#!/usr/bin/env bash
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
TOOLS_JSON="${ROOT}/tools.json"
BASE="$(python3 -c "import json; print(json.load(open('${TOOLS_JSON}'))['server_root'])")"
OUT="${BASE}/panel"
mkdir -p "${OUT}/data"
cd "${ROOT}/panel/panel-api"
go build -o "${OUT}/panel-api" .
echo "OK: ${OUT}/panel-api"
echo "DB path: ${OUT}/data/panel.db  (scp from Mac migrate-bundle if missing)"
echo "Start: ${ROOT}/deploy/start-panel-api.sh  → 127.0.0.1:18090 (not :8090)"
