#!/usr/bin/env bash
# First-draft server bootstrap — run inside _repo after git clone/pull
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "${ROOT}"
chmod +x deploy/*.sh

echo "==> 1) directories"
./deploy/bootstrap-dirs.sh

if [[ -f /www/wwwroot/gt4rents.com/_secrets/mysql.env ]]; then
  # shellcheck disable=SC1091
  source /www/wwwroot/gt4rents.com/_secrets/mysql.env
  echo "==> mysql.env loaded"
else
  echo "WARN: create /www/wwwroot/gt4rents.com/_secrets/mysql.env (see mysql.env.example)"
fi

echo "==> 2) build panel-api"
./deploy/build-panel.sh || echo "WARN: panel-api build failed — install Go"

echo "==> 3) build panel UI (Node 20+)"
./deploy/build-panel-ui.sh || echo "WARN: panel UI build failed — install Node/npm"

echo ""
echo "========== NEXT (see SERVER-SETUP.md) =========="
cat <<EOF
A) Mac → scp panel.db:
   scp .../migrate-bundle/panel.db root@SERVER:/www/wwwroot/gt4rents.com/panel/data/panel.db

B) Secrets:
   cp /www/wwwroot/gt4rents.com/_secrets/mysql.env.example \\
      /www/wwwroot/gt4rents.com/_secrets/mysql.env
   # edit password

C) aaPanel nginx:
   - include map:  ${ROOT}/deploy/nginx-host-port.map.conf   (http{})
   - wildcard:     ${ROOT}/deploy/nginx-wildcard.server.conf
   - panel site:   ${ROOT}/deploy/nginx-panel.server.conf

D) Start panel:
   ${ROOT}/deploy/start-panel-api.sh
   ${ROOT}/deploy/start-panel-ui.sh

E) First tool:
   source /www/wwwroot/gt4rents.com/_secrets/mysql.env
   ${ROOT}/deploy/build-one.sh refs
   ${ROOT}/deploy/start-tool.sh refs

Full guide: ${ROOT}/SERVER-SETUP.md
EOF
