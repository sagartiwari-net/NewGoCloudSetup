#!/usr/bin/env bash
# Fix wrank 404: ensure wrank.gt4rents.com → 4561 in live nginx maps + MySQL domain.
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/.." && pwd)"

python3 - <<'PY'
import pathlib, re, sys

NEED = "wrank.gt4rents.com"
LINE = "    wrank.gt4rents.com    4561;"
roots = [
    pathlib.Path("/www/wwwroot/gt4rents.com/_repo/deploy/nginx-host-port.map.conf"),
    pathlib.Path("/www/server/nginx/conf"),
    pathlib.Path("/www/server/panel/vhost/nginx"),
    pathlib.Path("/www/wwwroot/gt4rents.com"),
]
files = []
for r in roots:
    if r.is_file():
        files.append(r)
    elif r.is_dir():
        for p in r.rglob("*"):
            if not p.is_file():
                continue
            if p.suffix in {".bak", ".swp"}:
                continue
            try:
                txt = p.read_text(errors="ignore")
            except Exception:
                continue
            if "woorank.gt4rents.com" in txt or "map $host $tool_port" in txt:
                files.append(p)

seen = set()
for f in files:
    key = str(f.resolve()) if f.exists() else str(f)
    if key in seen:
        continue
    seen.add(key)
    try:
        txt = f.read_text()
    except Exception as e:
        print(f"SKIP read {f}: {e}")
        continue
    if re.search(r"(?m)^\s*wrank\.gt4rents\.com\s+", txt):
        print(f"OK already: {f}")
        continue
    if "woorank.gt4rents.com" in txt:
        txt2 = re.sub(
            r"(?m)^(\s*)woorank\.gt4rents\.com\s+4561\s*;",
            LINE + "\n\\1woorank.gt4rents.com    4561;",
            txt,
            count=1,
        )
        if txt2 == txt:
            txt2 = txt.replace("woorank.gt4rents.com    4561;", LINE + "\n    woorank.gt4rents.com    4561;", 1)
    elif "map $host $tool_port" in txt:
        txt2 = txt.replace("map $host $tool_port {", "map $host $tool_port {\n" + LINE, 1)
    else:
        print(f"SKIP no insert point: {f}")
        continue
    bak = f.with_suffix(f.suffix + ".bak-wrank")
    bak.write_text(txt)
    f.write_text(txt2)
    print(f"PATCHED: {f}")
PY

echo "== includes =="
grep -Rn 'nginx-host-port\|host-port.map\|tool_port' \
  /www/server/nginx/conf/nginx.conf \
  /www/server/panel/vhost/nginx/*.conf 2>/dev/null | head -40 || true

echo "== nginx -t && reload =="
nginx -t
nginx -s reload

echo "== MySQL =="
if [[ -f /www/wwwroot/gt4rents.com/_secrets/mysql.env ]]; then
  # shellcheck disable=SC1091
  source /www/wwwroot/gt4rents.com/_secrets/mysql.env
fi
export MYSQL_PWD="${GT4RENTS_MYSQL_PASSWORD:-${MYSQL_PWD:-}}"
mysql -u gt4rents gt4rents -e \
  "UPDATE ahrefs_websites SET domain='wrank.gt4rents.com' WHERE id=7;
   SELECT id, domain FROM ahrefs_websites WHERE id=7;"

echo "== probe (expect 401, not 404) =="
curl -sS -o /dev/null -w "local Host wrank → %{http_code}\n" -H 'Host: wrank.gt4rents.com' "http://127.0.0.1:4561/" || true
curl -sS -o /dev/null -w "https://wrank.gt4rents.com/ → %{http_code}\n" "https://wrank.gt4rents.com/" || true
