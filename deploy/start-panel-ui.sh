#!/usr/bin/env bash
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
SRC="${ROOT}/panel/update-panel"
LOG="${SRC}/panel-ui.log"
PIDF="${SRC}/panel-ui.pid"
PORT="${PORT:-3000}"

if [[ ! -d "${SRC}/.next" ]]; then
  echo "No .next build — run ./deploy/build-panel-ui.sh first"
  exit 1
fi
if [[ ! -d "${SRC}/node_modules" ]]; then
  echo "No node_modules — run ./deploy/build-panel-ui.sh first"
  exit 1
fi

if [[ -f "${PIDF}" ]] && kill -0 "$(cat "${PIDF}")" 2>/dev/null; then
  echo "Stopping old panel-ui pid $(cat "${PIDF}")"
  kill "$(cat "${PIDF}")" || true
  sleep 1
fi

cd "${SRC}"
if [[ ! -f .env.production ]]; then
  echo 'NEXT_PUBLIC_PANEL_API=' > .env.production
fi

nohup env PORT="${PORT}" npm run start >>"${LOG}" 2>&1 &
echo $! >"${PIDF}"
echo "OK: panel-ui pid $(cat "${PIDF}") → 127.0.0.1:${PORT} (log ${LOG})"
