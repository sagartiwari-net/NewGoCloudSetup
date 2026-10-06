#!/usr/bin/env bash
# Fix mgfc nginx host→5121 + MySQL/panel.db domain. ONLY patches nginx *.conf maps.
set -euo pipefail

LINE='    mgfc.gt4rents.com    5121;'

python3 - "$LINE" <<'PY'
import pathlib, re, sys

LINE = sys.argv[1]
candidates = [
    pathlib.Path("/www/wwwroot/gt4rents.com/_repo/deploy/nginx-host-port.map.conf"),
    pathlib.Path("/www/server/panel/vhost/nginx/gt4rents-host-port.map.conf"),
]
for root in (
    pathlib.Path("/www/server/panel/vhost/nginx"),
    pathlib.Path("/www/server/nginx/conf"),
):
    if not root.is_dir():
        continue
    for p in root.rglob("*.conf"):
        if p.name.endswith(".bak") or ".bak-" in p.name:
            continue
        try:
            txt = p.read_text(errors="ignore")
        except Exception:
            continue
        if "map $host $tool_port" in txt:
            candidates.append(p)

seen = set()
for f in candidates:
    try:
        key = str(f.resolve())
    except Exception:
        key = str(f)
    if key in seen or not f.is_file():
        continue
    seen.add(key)
    try:
        txt = f.read_text()
    except Exception as e:
        print("SKIP read", f, e)
        continue
    if "map $host $tool_port" not in txt and "magnific.gt4rents.com" not in txt and "mgfc.gt4rents.com" not in txt:
        print("SKIP unrelated:", f)
        continue
    if re.search(r"(?m)^\s*mgfc\.gt4rents\.com\s+\d+\s*;", txt):
        print("OK already:", f)
        continue
    if re.search(r"(?m)^\s*magnific\.gt4rents\.com\s+5121\s*;", txt):
        txt2 = re.sub(
            r"(?m)^(\s*)magnific\.gt4rents\.com\s+5121\s*;",
            LINE + "\n\\1magnific.gt4rents.com    5121;",
            txt,
            count=1,
        )
    elif "map $host $tool_port" in txt:
        txt2 = txt.replace("map $host $tool_port {", "map $host $tool_port {\n" + LINE, 1)
    else:
        print("SKIP no insert point:", f)
        continue
    if txt2 == txt:
        print("SKIP no change:", f)
        continue
    bak = f.with_name(f.name + ".bak-mgfc")
    bak.write_text(txt)
    f.write_text(txt2)
    print("PATCHED:", f)
PY

echo "== nginx -t && reload =="
nginx -t
nginx -s reload

echo "== MySQL ahrefs_websites =="
if [[ -f /www/wwwroot/gt4rents.com/_secrets/mysql.env ]]; then
  # shellcheck disable=SC1091
  source /www/wwwroot/gt4rents.com/_secrets/mysql.env
fi
export MYSQL_PWD="${GT4RENTS_MYSQL_PASSWORD:-${MYSQL_PWD:-}}"
mysql -u gt4rents gt4rents -e \
  "UPDATE ahrefs_websites SET domain='mgfc.gt4rents.com' WHERE id=62;
   UPDATE websites SET domain='mgfc.gt4rents.com' WHERE id=62;
   SELECT id, domain FROM ahrefs_websites WHERE id=62;" || true

echo "== panel.db websites =="
PANEL_DB="/www/wwwroot/gt4rents.com/panel/data/panel.db"
if [[ -f "$PANEL_DB" ]]; then
  sqlite3 "$PANEL_DB" "UPDATE websites SET domain='mgfc.gt4rents.com' WHERE id=62;
SELECT id, name, domain FROM websites WHERE id=62 OR domain LIKE '%magnific%' OR domain LIKE '%mgfc%';"
else
  echo "WARN: missing $PANEL_DB"
fi

echo "== probe (expect 401/403 — NOT nginx 404) =="
curl -sS -o /dev/null -w "local :5121 → %{http_code}\n" -H 'Host: mgfc.gt4rents.com' "http://127.0.0.1:5121/" || true
curl -sS -o /dev/null -w "https://mgfc.gt4rents.com/ → %{http_code}\n" "https://mgfc.gt4rents.com/" || true
curl -sS -o /dev/null -w "https://mgfc.gt4rents.com/access → %{http_code}\n" "https://mgfc.gt4rents.com/access?token=probe" || true

echo "== config sanity =="
python3 - <<'PY'
import json
from pathlib import Path
p = Path("/www/wwwroot/gt4rents.com/mgfc/config.json")
if not p.exists():
    print("MISSING config.json — run: ./deploy/build-one.sh mgfc")
    raise SystemExit(1)
c = json.loads(p.read_text())
print("target_url=", c.get("target_url"))
print("tool_name=", c.get("tool_name"))
print("public_host=", c.get("public_host"))
print("panel_db=", c.get("panel_db"))
t = (c.get("target_url") or "").lower()
if "magnific" not in t and "syntx" not in t:
    print("WARN: unexpected target_url — confirm Magnific config")
if c.get("public_host") != "mgfc.gt4rents.com":
    print("ERROR: public_host must be mgfc.gt4rents.com")
    raise SystemExit(2)
print("OK mgfc config")
PY
