#!/usr/bin/env bash
# Panel + nginx map for Ahrefs Bar — same style as DOMAIN-MIGRATE / ROLLOUT panel rebuild.
# Run on server AFTER bar-api is already up (./deploy/build-ahrefs-bar.sh + start-ahrefs-bar.sh).
#
#   cd /www/wwwroot/gt4rents.com/_repo
#   git pull origin main
#   source /www/wwwroot/gt4rents.com/_secrets/mysql.env
#   ./deploy/setup-ahrefs-bar-panel.sh
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
BASE="${BASE:-/www/wwwroot/gt4rents.com}"
MAP_SRC="${ROOT}/deploy/nginx-host-port.map.conf"
MAP_DST="/www/server/panel/vhost/nginx/gt4rents-host-port.map.conf"

echo "== 1) nginx host→port map (barapi.upays.in → 8088)"
if [[ -f "${MAP_SRC}" ]]; then
  if [[ -f "${MAP_DST}" ]]; then
    cp -a "${MAP_DST}" "${MAP_DST}.bak.$(date +%Y%m%d%H%M%S)"
  fi
  cp "${MAP_SRC}" "${MAP_DST}"
  nginx -t
  nginx -s reload
  echo "OK: map reloaded (barapi.upays.in → 8088)"
else
  echo "WARN: missing ${MAP_SRC}"
fi

echo "== 2) build panel-api + UI"
"${ROOT}/deploy/build-panel.sh"
"${ROOT}/deploy/build-panel-ui.sh"

echo "== 3) restart panel only (not gt4-* tools)"
"${ROOT}/deploy/start-panel-api.sh"
"${ROOT}/deploy/start-panel-ui.sh"

echo ""
echo "Done. Hard-refresh panel.upays.in — sidebar should show Ahrefs Bar."
echo "Smoke: curl -sI https://barapi.upays.in/v1/portals | head -5"
echo "Local: curl -s http://127.0.0.1:8088/v1/portals"
