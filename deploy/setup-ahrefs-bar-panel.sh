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
  echo "OK: map reloaded"
  grep -n 'barapi' "${MAP_DST}" || echo "WARN: barapi line missing in ${MAP_DST}"
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
echo "== 4) smoke"
echo -n "bar-api local: "
curl -s http://127.0.0.1:8088/v1/portals || true
echo ""
echo -n "nginx Host barapi → : "
curl -sI -H 'Host: barapi.upays.in' http://127.0.0.1/v1/portals | head -3 || true
echo ""
echo "Done. Hard-refresh panel.upays.in — Ahrefs Bar in sidebar."
echo "If browser still 404 on https://barapi.upays.in: check aaPanel SSL vhost uses same map (\$tool_port)."