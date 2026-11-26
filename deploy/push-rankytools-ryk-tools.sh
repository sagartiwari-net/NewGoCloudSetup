#!/usr/bin/env bash
# Run ON the gt4rents server after: cd /www/wwwroot/gt4rents.com/_repo && git pull
# Builds + installs RankyTools ryk* with panel security (bypass_auth=false).
# Cookies: cookie.json seeds only empty accounts; later updates stay per-site via Recloud UI.
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "${ROOT}"

if [[ ! -f /www/wwwroot/gt4rents.com/_secrets/mysql.env ]]; then
  echo "Missing /www/wwwroot/gt4rents.com/_secrets/mysql.env"
  exit 1
fi
# shellcheck disable=SC1091
set -a
source /www/wwwroot/gt4rents.com/_secrets/mysql.env
set +a
export TOOL_PUBLIC_SCHEME="${TOOL_PUBLIC_SCHEME:-https}"

TOOLS=(ryksm1 rykmj rykcp rykus rykgm ryken ryksb ryksf rykns ryksz02 ryksw)

echo "== nginx map (live copy — repo file alone does not fix 404) =="
MAP_SRC="${ROOT}/deploy/nginx-host-port.map.conf"
if [[ -f "${MAP_SRC}" ]]; then
  for dest in \
    /www/server/panel/vhost/nginx/gt4rents-host-port.map.conf \
    /www/server/panel/vhost/nginx/extension/nginx-host-port.map.conf \
    /www/server/nginx/conf/nginx-host-port.map.conf
  do
    dir="$(dirname "${dest}")"
    if [[ -f "${dest}" ]] || [[ -d "${dir}" ]]; then
      mkdir -p "${dir}"
      cp -f "${MAP_SRC}" "${dest}"
      echo "copied map → ${dest}"
    fi
  done
else
  echo "WARN: missing ${MAP_SRC}"
fi

echo "== MySQL panel rows =="
if command -v mysql >/dev/null 2>&1; then
  mysql -h"${GT4RENTS_MYSQL_HOST:-127.0.0.1}" -P"${GT4RENTS_MYSQL_PORT:-3306}" \
    -u"${GT4RENTS_MYSQL_USER:-gt4rents}" -p"${GT4RENTS_MYSQL_PASSWORD}" \
    "${GT4RENTS_MYSQL_DB:-gt4rents}" < "${ROOT}/deploy/add-rankytools-ryk-tools.sql" \
    && echo "SQL OK" || echo "SQL skipped/failed (ensureRecloudSite will still create rows on start)"
else
  echo "mysql client missing — skipping SQL; tool start will ensure website rows"
fi

echo "== build + systemd =="
for sub in "${TOOLS[@]}"; do
  echo "---- ${sub} ----"
  "${ROOT}/deploy/build-one.sh" "${sub}"
  FOLDER="$(python3 - <<PY
import json
sub="${sub}"
data=json.load(open("${ROOT}/tools.json"))
for t in data["tools"]:
    if t["subdomain"]==sub:
        print(t["folder"]); break
PY
)"
  SRC="${ROOT}/tools/${FOLDER}"
  OUT="/www/wwwroot/gt4rents.com/${sub}"
  if [[ -f "${SRC}/cookie.json" ]]; then
    cp -f "${SRC}/cookie.json" "${OUT}/cookie.json"
    echo "copied cookie.json → ${OUT}/cookie.json"
  fi
  "${ROOT}/deploy/install-systemd-tool.sh" "${sub}"
done

echo "== nginx reload (if map already wired) =="
if command -v nginx >/dev/null 2>&1; then
  nginx -t && nginx -s reload && echo "nginx reloaded" || echo "nginx reload skipped"
fi

echo "== smoke =="
for sub in "${TOOLS[@]}"; do
  port="$(python3 - <<PY
import json
sub="${sub}"
for t in json.load(open("${ROOT}/tools.json"))["tools"]:
    if t["subdomain"]==sub:
        print(t["port"]); break
PY
)"
  code="$(curl -s -o /dev/null -w '%{http_code}' --max-time 5 "http://127.0.0.1:${port}/" -H "Host: ${sub}.upays.in" || echo 000)"
  echo "${sub} :${port} → HTTP ${code}"
done

echo "DONE. Panel → Recloud: each ryk* row has its own cookie + upstream_url."
echo "Open via panel access link only (direct URL = Access Denied)."
