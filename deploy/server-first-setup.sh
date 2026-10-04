#!/usr/bin/env bash
# First-draft server bootstrap (run on Hetzner/Contabo after git clone into _repo)
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "${ROOT}"

echo "==> bootstrap dirs"
chmod +x deploy/*.sh
./deploy/bootstrap-dirs.sh

echo "==> build panel-api"
./deploy/build-panel.sh || echo "WARN: panel-api build failed (fix Go / deps later)"

echo "==> build panel UI (needs node/npm — may take a few minutes)"
./deploy/build-panel-ui.sh || echo "WARN: panel UI build failed (install node 20+ later)"

echo "==> NEXT STEPS (manual / Mac)"
cat <<EOF

1) From Mac, upload DB (NOT in git):
   scp /Users/sagartiwari/Desktop/oneclickgo/NewGoCloudSetup/migrate-bundle/panel.db \\
       root@65.109.16.196:/www/wwwroot/gt4rents.com/panel/data/panel.db

2) Set MySQL password on server:
   export GT4RENTS_MYSQL_PASSWORD='(from local setup.md)'

3) aaPanel nginx:
   - http{} include: $(pwd)/deploy/nginx-host-port.map.conf
   - site *.gt4rents.com → deploy/nginx-wildcard.server.conf
   - site panel.gt4rents.com → deploy/nginx-panel.server.conf
     (UI :3000 + /api → :8090)
   - SSL later

4) Start panel API + UI:
   cd /www/wwwroot/gt4rents.com/panel && ./panel-api
   # API: 127.0.0.1:8090
   cd $(pwd)/panel/update-panel && PORT=3000 npm run start
   # UI: 127.0.0.1:3000 → https://panel.gt4rents.com

5) Build + test ONE tool first (recommended):
   ./deploy/build-one.sh refs

6) Later: ./deploy/build-all.sh  (expect some failures — fix one by one)

EOF
