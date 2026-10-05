#!/usr/bin/env bash
# One-shot recover for wrank after stash/pull conflicts + wrong ChatGPT binary on :4561.
set -euo pipefail
cd /www/wwwroot/gt4rents.com/_repo

echo "== discard local fix-wrank-nginx.sh drift + pull =="
git checkout -- deploy/fix-wrank-nginx.sh 2>/dev/null || true
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

./deploy/start-tool.sh wrank
./deploy/fix-wrank-nginx.sh

echo "== expect WooRank in log =="
tail -15 /www/wwwroot/gt4rents.com/wrank/app.log
