#!/usr/bin/env bash
# Same style as start-tool.sh — restart ONLY ahrefs-bar-api (never gt4-* / panel).
# Usage:
#   ./deploy/start-ahrefs-bar.sh
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
BASE="${BASE:-/www/wwwroot/gt4rents.com}"
OUTDIR="${BASE}/ahrefs-bar-api"
BIN="${OUTDIR}/ahrefs-bar-api"
UNIT_SRC="${ROOT}/ahrefs/deploy/ahrefs-bar-api.service"

if [[ ! -x "${BIN}" ]]; then
  echo "Missing ${BIN} — run: ./deploy/build-ahrefs-bar.sh"
  exit 1
fi

install -m 644 "${UNIT_SRC}" /etc/systemd/system/ahrefs-bar-api.service
systemctl daemon-reload
systemctl enable ahrefs-bar-api.service
systemctl restart ahrefs-bar-api.service
sleep 1
systemctl --no-pager --lines=20 status ahrefs-bar-api.service || true

echo ""
echo "OK: systemd ahrefs-bar-api → 127.0.0.1:8088"
echo "Smoke: curl -s http://127.0.0.1:8088/v1/portals"
echo "NOT restarted: gt4-* tools, gt4-panel-api, nginx"
