#!/usr/bin/env bash
# Run ON SERVER after: cd /www/wwwroot/gt4rents.com/_repo && git pull
# Only touches NoxTools Home (noxh). Does not restart other tools.
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"

echo "==> SQL seed (idempotent)"
if [[ -f /www/wwwroot/gt4rents.com/_secrets/mysql.env ]]; then
  # shellcheck disable=SC1091
  source /www/wwwroot/gt4rents.com/_secrets/mysql.env
fi
mysql -h127.0.0.1 -ugt4rents -p"${GT4RENTS_MYSQL_PASSWORD}" gt4rents < "${ROOT}/deploy/add-noxh.sql"

echo "==> nginx map (if your http{} includes this file, reload nginx)"
if [[ -f /www/server/nginx/conf/nginx.conf ]] || command -v nginx >/dev/null 2>&1; then
  nginx -t && nginx -s reload || true
fi

echo "==> build + start noxh only"
"${ROOT}/deploy/build-one.sh" noxh
if [[ -x "${ROOT}/deploy/install-systemd-tool.sh" ]]; then
  "${ROOT}/deploy/install-systemd-tool.sh" noxh || "${ROOT}/deploy/start-tool.sh" noxh
else
  "${ROOT}/deploy/start-tool.sh" noxh
fi

# Also drop cookie.txt next to binary as fallback
COOKIE_SRC="${ROOT}/tools/noxtools-go-proxy/cookie.server.txt"
if [[ -f "$COOKIE_SRC" ]]; then
  cp -f "$COOKIE_SRC" /www/wwwroot/gt4rents.com/noxh/cookie.txt
  echo "Copied cookie.server.txt → noxh/cookie.txt"
fi

echo "Done. Open panel → NoxTools Home  or  https://noxh.upays.in/secure/member"
echo "Watch logs: journalctl -u gt4-tool-noxh -f   (or your unit name)"
