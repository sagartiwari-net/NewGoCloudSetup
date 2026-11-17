#!/usr/bin/env bash
# Install ONLY the NoxTools cookie keepalive daemon.
# No panel tool, no port, no nginx, no website row.
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
SRC="${ROOT}/tools/nox-cookie-keepalive"
DEST="/www/wwwroot/gt4rents.com/_keepalive/noxtools"

echo "==> build keepalive binary"
mkdir -p "${DEST}"
cd "${SRC}"
go build -o "${DEST}/nox-cookie-keepalive" .

echo "==> install cookie.json (keep existing if present)"
if [[ ! -f "${DEST}/cookie.json" ]]; then
  cp -f "${SRC}/cookie.json" "${DEST}/cookie.json"
  chmod 600 "${DEST}/cookie.json"
else
  echo "    keeping existing ${DEST}/cookie.json"
fi

echo "==> systemd unit + restart (pick up new binary)"
cp -f "${SRC}/nox-cookie-keepalive.service" /etc/systemd/system/nox-cookie-keepalive.service
systemctl daemon-reload
systemctl enable nox-cookie-keepalive.service
systemctl restart nox-cookie-keepalive.service

echo "==> one-shot status"
sleep 1
systemctl --no-pager --full status nox-cookie-keepalive.service | head -20
journalctl -u nox-cookie-keepalive -n 30 --no-pager || true
echo
echo "Done. No tool/port added. Cookie file: ${DEST}/cookie.json"
echo "Logs: journalctl -u nox-cookie-keepalive -f"
echo "Expect log line: session_ok (not bare status=200)"
