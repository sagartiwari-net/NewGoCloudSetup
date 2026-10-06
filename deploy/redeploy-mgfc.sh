#!/usr/bin/env bash
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"
source /www/wwwroot/gt4rents.com/_secrets/mysql.env 2>/dev/null || true
./deploy/build-one.sh mgfc
./deploy/start-tool.sh mgfc
chmod +x deploy/fix-mgfc-nginx.sh
./deploy/fix-mgfc-nginx.sh
echo "mgfc redeployed — open panel access-link on mgfc.gt4rents.com (not magnific)"
