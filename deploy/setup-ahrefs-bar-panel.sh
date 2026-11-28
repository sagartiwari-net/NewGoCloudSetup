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
# aaPanel may include any of these — sync all (same as fix-nox / fix-t4toolz)
MAP_DSTS=(
  /www/server/panel/vhost/nginx/gt4rents-host-port.map.conf
  /www/server/panel/vhost/nginx/nginx-host-port.map.conf
  /www/server/panel/vhost/nginx/extension/nginx-host-port.map.conf
  /www/server/nginx/conf/nginx-host-port.map.conf
)

echo "== 1) nginx host→port map (barapi.upays.in → 8088)"
if [[ -f "${MAP_SRC}" ]]; then
  for MAP_DST in "${MAP_DSTS[@]}"; do
    dir=$(dirname "${MAP_DST}")
    [[ -d "${dir}" ]] || continue
    if [[ -f "${MAP_DST}" ]]; then
      cp -a "${MAP_DST}" "${MAP_DST}.bak.$(date +%Y%m%d%H%M%S)"
    fi
    cp "${MAP_SRC}" "${MAP_DST}"
    echo "OK → ${MAP_DST}"
  done
  nginx -t
  nginx -s reload
  echo "OK: map reloaded"
  grep -n 'barapi' "${MAP_DSTS[0]}" 2>/dev/null || grep -rn 'barapi' "${MAP_DSTS[@]}" 2>/dev/null | head -5 || echo "WARN: barapi line missing"
else
  echo "WARN: missing ${MAP_SRC}"
fi

echo "== 2) sync extension + templates (zip source for Download)"
OUTDIR="${BASE}/ahrefs-bar-api"
if [[ -d "${ROOT}/ahrefs/extension" ]]; then
  mkdir -p "${OUTDIR}/extension"
  rsync -a --delete --exclude '_metadata' "${ROOT}/ahrefs/extension/" "${OUTDIR}/extension/"
  echo "OK: ${OUTDIR}/extension  (manifest $(grep -o '"version"[[:space:]]*:[[:space:]]*"[^"]*"' "${OUTDIR}/extension/manifest.json" 2>/dev/null || echo '?'))"
else
  echo "WARN: missing ${ROOT}/ahrefs/extension"
fi
if [[ -d "${ROOT}/ahrefs/templates" ]]; then
  mkdir -p "${OUTDIR}/templates"
  rsync -a "${ROOT}/ahrefs/templates/" "${OUTDIR}/templates/"
  echo "OK: ${OUTDIR}/templates"
fi

echo "== 3) build panel-api + UI"
"${ROOT}/deploy/build-panel.sh"
"${ROOT}/deploy/build-panel-ui.sh"

echo "== 4) restart panel only (not gt4-* tools)"
"${ROOT}/deploy/start-panel-api.sh"
"${ROOT}/deploy/start-panel-ui.sh"

echo ""
echo "== 5) smoke"
echo -n "bar-api local: "
curl -s http://127.0.0.1:8088/v1/portals || true
echo ""
echo -n "nginx Host barapi → : "
curl -sI -H 'Host: barapi.upays.in' http://127.0.0.1/v1/portals | head -3 || true
echo ""
echo "Done. Hard-refresh panel.upays.in — Ahrefs Bar in sidebar."
echo "If browser still 404 on https://barapi.upays.in: run deploy/fix-barapi-nginx.sh (multi-map + dedicated vhost)."