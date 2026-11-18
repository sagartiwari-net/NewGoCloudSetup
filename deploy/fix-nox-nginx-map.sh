#!/usr/bin/env bash
# Fix nginx 404 on nxsm*/nxaud*: live map is a COPY of the repo file.
# Also applies product id 5 mapping. Run ON the gt4rents server:
#   cd /www/wwwroot/gt4rents.com/_repo && git pull && chmod +x deploy/fix-nox-nginx-map.sh && ./deploy/fix-nox-nginx-map.sh
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
MAP_SRC="${ROOT}/deploy/nginx-host-port.map.conf"

if [[ ! -f "${MAP_SRC}" ]]; then
  echo "Missing ${MAP_SRC}"
  exit 1
fi
if ! grep -q 'nxsm2.upays.in' "${MAP_SRC}"; then
  echo "Repo map missing nxsm2 — git pull first"
  exit 1
fi

echo "== sync nginx host→port map =="
# Primary live path (DOMAIN-MIGRATE.md). Also refresh any other tool_port maps.
COPIED=0
for dest in \
  /www/server/panel/vhost/nginx/gt4rents-host-port.map.conf \
  /www/server/panel/vhost/nginx/extension/nginx-host-port.map.conf \
  /www/server/nginx/conf/nginx-host-port.map.conf \
  /etc/nginx/nginx-host-port.map.conf
do
  dir="$(dirname "${dest}")"
  if [[ -f "${dest}" ]] || [[ -d "${dir}" && -f /www/server/panel/vhost/nginx/gt4rents-host-port.map.conf ]]; then
    if [[ -d "${dir}" ]]; then
      cp -f "${MAP_SRC}" "${dest}"
      echo "copied → ${dest}"
      COPIED=1
    fi
  elif [[ -d "${dir}" ]]; then
    cp -f "${MAP_SRC}" "${dest}"
    echo "copied → ${dest}"
    COPIED=1
  fi
done

# Patch any other conf that still has the tool_port map but not nxsm2.
python3 - "${MAP_SRC}" <<'PY'
import pathlib, sys
src = pathlib.Path(sys.argv[1]).read_text()
if "nxsm2.upays.in" not in src:
    raise SystemExit("src map incomplete")
roots = [
    pathlib.Path("/www/server/panel/vhost/nginx"),
    pathlib.Path("/www/server/nginx/conf"),
]
seen = set()
for root in roots:
    if not root.is_dir():
        continue
    for p in root.rglob("*.conf"):
        if ".bak" in p.name:
            continue
        try:
            key = str(p.resolve())
        except Exception:
            key = str(p)
        if key in seen or not p.is_file():
            continue
        seen.add(key)
        try:
            txt = p.read_text(errors="ignore")
        except Exception:
            continue
        if "map $host $tool_port" not in txt:
            continue
        if "nxsm2.upays.in" in txt and "nxaud3.upays.in" in txt:
            print("OK already:", p)
            continue
        bak = p.with_name(p.name + ".bak-nox")
        bak.write_text(txt)
        p.write_text(src)
        print("REPLACED map file:", p)
PY

if [[ "${COPIED}" -eq 0 ]]; then
  # Always ensure the documented live path exists for the wildcard include.
  mkdir -p /www/server/panel/vhost/nginx
  cp -f "${MAP_SRC}" /www/server/panel/vhost/nginx/gt4rents-host-port.map.conf
  echo "copied → /www/server/panel/vhost/nginx/gt4rents-host-port.map.conf"
fi

echo "== nginx -t && reload =="
nginx -t
nginx -s reload
echo "nginx reloaded"

echo "== product id 5 mapping =="
if [[ -f /www/wwwroot/gt4rents.com/_secrets/mysql.env ]]; then
  # shellcheck disable=SC1091
  set -a
  source /www/wwwroot/gt4rents.com/_secrets/mysql.env
  set +a
  mysql -h"${GT4RENTS_MYSQL_HOST:-127.0.0.1}" -P"${GT4RENTS_MYSQL_PORT:-3306}" \
    -u"${GT4RENTS_MYSQL_USER:-gt4rents}" -p"${GT4RENTS_MYSQL_PASSWORD}" \
    "${GT4RENTS_MYSQL_DB:-gt4rents}" < "${ROOT}/deploy/map-nox-product-5.sql" \
    && echo "product map OK" || echo "product SQL failed"
else
  echo "mysql.env missing — skip product SQL"
fi

echo "== probe (expect 403/401 — NOT nginx 404) =="
for host in nxsm1.upays.in nxsm2.upays.in nxaud1.upays.in; do
  port="$(python3 - <<PY
import re
from pathlib import Path
txt = Path("${MAP_SRC}").read_text()
m = re.search(r"(?m)^\s*${host.replace('.', r'\.')}\s+(\d+)\s*;", txt)
print(m.group(1) if m else "0")
PY
)"
  local_code="$(curl -s -o /dev/null -w '%{http_code}' --max-time 5 -H "Host: ${host}" "http://127.0.0.1:${port}/" || echo 000)"
  pub_code="$(curl -s -o /dev/null -w '%{http_code}' --max-time 8 "https://${host}/" || echo 000)"
  echo "${host} local:${port}→${local_code} public→${pub_code}"
done

echo "DONE. Public must NOT be 404. 403 = tool up + security OK."
