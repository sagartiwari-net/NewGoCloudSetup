#!/usr/bin/env bash
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
BASE="$(python3 -c "import json; print(json.load(open('${ROOT}/tools.json'))['server_root'])")"
PANEL="${BASE}/panel"
BIN="${PANEL}/panel-api"
LOG="${PANEL}/panel-api.log"
PIDF="${PANEL}/panel-api.pid"
# Dedicated port — never steal :8090 from payment-hub / other apps
ADDR="${PANEL_API_ADDR:-127.0.0.1:18090}"
PORT="${ADDR##*:}"

if [[ ! -x "${BIN}" ]]; then
  echo "Missing ${BIN} — run ./deploy/build-panel.sh first"
  exit 1
fi
if [[ ! -f "${PANEL}/data/panel.db" ]]; then
  echo "WARN: ${PANEL}/data/panel.db missing — scp migrate-bundle/panel.db from Mac"
fi

# Refuse to start if OUR chosen port is already taken by someone else
if ss -lptn 2>/dev/null | grep -q ":${PORT} "; then
  # allow restart if it is our previous pid
  if [[ -f "${PIDF}" ]] && kill -0 "$(cat "${PIDF}")" 2>/dev/null; then
    echo "Stopping old panel-api pid $(cat "${PIDF}")"
    kill "$(cat "${PIDF}")" || true
    sleep 1
  else
    echo "ERROR: port ${PORT} already in use by another process. Pick a free PANEL_API_ADDR."
    ss -lptn | grep ":${PORT} " || true
    exit 1
  fi
elif [[ -f "${PIDF}" ]] && kill -0 "$(cat "${PIDF}")" 2>/dev/null; then
  echo "Stopping old panel-api pid $(cat "${PIDF}")"
  kill "$(cat "${PIDF}")" || true
  sleep 1
fi

cd "${PANEL}"
nohup env PANEL_API_ADDR="${ADDR}" ./panel-api >>"${LOG}" 2>&1 &
echo $! >"${PIDF}"
sleep 1
if ! kill -0 "$(cat "${PIDF}")" 2>/dev/null; then
  echo "ERROR: panel-api exited — see ${LOG}"
  tail -20 "${LOG}" || true
  exit 1
fi
echo "OK: panel-api pid $(cat "${PIDF}") → ${ADDR} (log ${LOG})"
echo "NOTE: never kill unrelated listeners on :8090 or other ports"
