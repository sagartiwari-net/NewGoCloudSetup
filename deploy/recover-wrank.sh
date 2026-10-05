#!/usr/bin/env bash
# One-shot recover for wrank after stash/pull conflicts + wrong ChatGPT binary on :4561.
set -euo pipefail
cd /www/wwwroot/gt4rents.com/_repo

echo "== restore any files corrupted by old fix-wrank scanner =="
git checkout -- ROLLOUT.md deploy/fix-wrank-nginx.sh deploy/configs/wrank.config.overlay.json deploy/recover-wrank.sh 2>/dev/null || true
# remove accidental bak junk in repo (keep nginx baks)
rm -f deploy/fix-wrank-nginx.sh.bak-wrank \
      deploy/configs/wrank.config.overlay.json.bak-wrank \
      ROLLOUT.md.bak-wrank 2>/dev/null || true

echo "== discard local drift + pull =="
git stash push -u -m "wrank-recover-$(date +%s)" -- deploy/fix-wrank-nginx.sh 2>/dev/null || true
git pull --ff-only origin main

source /www/wwwroot/gt4rents.com/_secrets/mysql.env
chmod +x deploy/fix-wrank-nginx.sh deploy/recover-wrank.sh

echo "== rebuild WooRank binary + config =="
./deploy/build-one.sh wrank

# Hard-force WooRank fields (guards against empty target → chatgpt default)
python3 - <<'PY'
import json
from pathlib import Path
p = Path("/www/wwwroot/gt4rents.com/wrank/config.json")
c = json.loads(p.read_text()) if p.exists() else {}
c.update({
    "port": "4561",
    "public_host": "wrank.gt4rents.com",
    "public_scheme": "https",
    "website_id": 7,
    "target_url": "https://www.woorank.com",
    "cdn_url": "https://www.woorank.com",
    "tool_name": "WooRank",
    "home_path": "/en/overview",
    "cookie_domain_suffix": "woorank.com",
    "panel_db": "/www/wwwroot/gt4rents.com/panel/data/panel.db",
    "use_database": False,
    "bypass_auth": False,
    "local_test_mode": False,
})
p.write_text(json.dumps(c, indent=2) + "\n")
print("forced", p)
print("target_url=", c["target_url"], "tool_name=", c["tool_name"])
PY

# Fix legacy /www/wwwroot/gt4rents.com/woorank/config.json if present (do not touch app.log)
if [[ -f /www/wwwroot/gt4rents.com/woorank/config.json ]]; then
  python3 - <<'PY'
import json
from pathlib import Path
p = Path("/www/wwwroot/gt4rents.com/woorank/config.json")
try:
    c = json.loads(p.read_text())
except Exception:
    c = {}
c.update({
    "public_host": "wrank.gt4rents.com",
    "target_url": "https://www.woorank.com",
    "cdn_url": "https://www.woorank.com",
    "tool_name": "WooRank",
})
p.write_text(json.dumps(c, indent=2) + "\n")
print("fixed legacy", p)
PY
fi

./deploy/start-tool.sh wrank
./deploy/fix-wrank-nginx.sh

echo "== expect WooRank in log =="
tail -15 /www/wwwroot/gt4rents.com/wrank/app.log
