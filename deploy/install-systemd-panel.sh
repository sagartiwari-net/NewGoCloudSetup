#!/usr/bin/env bash
# Boot-start panel UI (:3000) and panel API (:18090). Leaves :8090 and :5001 alone.
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
BASE="$(python3 -c "import json; print(json.load(open('${ROOT}/tools.json'))['server_root'])")"

stop_pidfile() {
  local pidf="$1"
  if [[ -f "${pidf}" ]] && kill -0 "$(cat "${pidf}")" 2>/dev/null; then
    kill "$(cat "${pidf}")" || true
    sleep 1
  fi
  rm -f "${pidf}"
}

stop_pidfile "${ROOT}/panel/update-panel/panel-ui.pid"
stop_pidfile "${BASE}/panel/panel-api.pid"
if command -v fuser >/dev/null 2>&1; then
  fuser -k 3000/tcp >/dev/null 2>&1 || true
  fuser -k 18090/tcp >/dev/null 2>&1 || true
  sleep 1
fi

chmod +x "${ROOT}/deploy/run-panel-api.sh" "${ROOT}/deploy/run-panel-ui.sh"
cp "${ROOT}/deploy/systemd/gt4-panel-api.service" /etc/systemd/system/gt4-panel-api.service
cp "${ROOT}/deploy/systemd/gt4-panel-ui.service" /etc/systemd/system/gt4-panel-ui.service
systemctl daemon-reload
systemctl enable gt4-panel-api.service gt4-panel-ui.service
systemctl restart gt4-panel-api.service
systemctl restart gt4-panel-ui.service
systemctl --no-pager --lines=8 status gt4-panel-api.service
systemctl --no-pager --lines=8 status gt4-panel-ui.service
echo "OK: panel starts on boot. Restart later with: systemctl restart gt4-panel-api gt4-panel-ui"
echo "NOTE: this does not stop :8090 or :5001"
