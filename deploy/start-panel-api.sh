#!/usr/bin/env bash
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
BASE="$(python3 -c "import json; print(json.load(open('${ROOT}/tools.json'))['server_root'])")"
PANEL="${BASE}/panel"
BIN="${PANEL}/panel-api"
LOG="${PANEL}/panel-api.log"
PIDF="${PANEL}/panel-api.pid"

if [[ ! -x "${BIN}" ]]; then
  echo "Missing ${BIN} — run ./deploy/build-panel.sh first"
  exit 1
fi
if [[ ! -f "${PANEL}/data/panel.db" ]]; then
  echo "WARN: ${PANEL}/data/panel.db missing — scp migrate-bundle/panel.db from Mac"
fi

if [[ -f "${PIDF}" ]] && kill -0 "$(cat "${PIDF}")" 2>/dev/null; then
  echo "Stopping old panel-api pid $(cat "${PIDF}")"
  kill "$(cat "${PIDF}")" || true
  sleep 1
fi

cd "${PANEL}"
nohup ./panel-api >>"${LOG}" 2>&1 &
echo $! >"${PIDF}"
echo "OK: panel-api pid $(cat "${PIDF}") → 127.0.0.1:8090 (log ${LOG})"
