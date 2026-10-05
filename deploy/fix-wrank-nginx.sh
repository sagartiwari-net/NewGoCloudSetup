#!/usr/bin/env bash
# Fix wrank: nginx host→4561, MySQL+panel.db domain, probe (expect 401/Access Denied — not nginx 404).
set -euo pipefail

LINE='    wrank.gt4rents.com    4561;'

python3 - "$LINE" <<'PY'
import pathlib, re, sys

LINE = sys.argv[1]
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
            if not p.is_file() or p.suffix in {".bak", ".swp"}:
                continue
            try:
                txt = p.read_text(errors="ignore")
            except Exception:
                continue
            if "woorank.gt4rents.com" in txt or "map $host $tool_port" in txt or "wrank.gt4rents.com" in txt:
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
        print("SKIP read", f, e)
        continue
    if re.search(r"(?m)^\s*wrank\.gt4rents\.com\s+\d+", txt):
        print("OK already:", f)
        continue
    if "woorank.gt4rents.com" in txt:
        txt2 = re.sub(
            r"(?m)^(\s*)woorank\.gt4rents\.com\s+4561\s*;",
            LINE + r"\n\1woorank.gt4rents.com    4561;",
            txt,
            count=1,
        )
        if txt2 == txt:
            txt2 = txt.replace(
                "woorank.gt4rents.com    4561;",
                LINE + "\n    woorank.gt4rents.com    4561;",
                1,
            )
    elif "map $host $tool_port" in txt:
        txt2 = txt.replace("map $host $tool_port {", "map $host $tool_port {\n" + LINE, 1)
    else:
        print("SKIP no insert point:", f)
        continue
    bak = f.with_suffix(f.suffix + ".bak-wrank")
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
  "UPDATE ahrefs_websites SET domain='wrank.gt4rents.com' WHERE id=7;
   SELECT id, domain FROM ahrefs_websites WHERE id=7;" || true

echo "== panel.db websites =="
PANEL_DB="/www/wwwroot/gt4rents.com/panel/data/panel.db"
if [[ -f "$PANEL_DB" ]]; then
  sqlite3 "$PANEL_DB" "UPDATE websites SET domain='wrank.gt4rents.com' WHERE id=7;
SELECT id, name, domain FROM websites WHERE id=7 OR domain LIKE '%woorank%' OR domain LIKE '%wrank%';"
else
  echo "WARN: missing $PANEL_DB"
fi

echo "== probe (expect 401/403 — NOT nginx 404) =="
curl -sS -o /dev/null -w "local :4561 → %{http_code}\n" -H 'Host: wrank.gt4rents.com' "http://127.0.0.1:4561/" || true
curl -sS -o /dev/null -w "https://wrank.gt4rents.com/ → %{http_code}\n" "https://wrank.gt4rents.com/" || true
curl -sS -o /dev/null -w "https://wrank.gt4rents.com/access → %{http_code}\n" "https://wrank.gt4rents.com/access?token=probe" || true

echo "== config sanity =="
python3 - <<'PY'
import json
from pathlib import Path
p = Path("/www/wwwroot/gt4rents.com/wrank/config.json")
if not p.exists():
    print("MISSING config.json — run: ./deploy/build-one.sh wrank")
    raise SystemExit(1)
c = json.loads(p.read_text())
print("target_url=", c.get("target_url"))
print("tool_name=", c.get("tool_name"))
print("public_host=", c.get("public_host"))
print("panel_db=", c.get("panel_db"))
t = (c.get("target_url") or "").lower()
name = (c.get("tool_name") or "").lower()
if "chatgpt" in t or "openai" in t or name in ("", "tool"):
    print("ERROR: bad config — rebuild wrank after git pull")
    raise SystemExit(2)
if "woorank" not in t:
    print("WARN: unexpected target")
else:
    print("OK: WooRank target")
PY
