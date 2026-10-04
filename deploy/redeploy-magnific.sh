#!/usr/bin/env bash
# Run on gt4rents server as root (already logged in as root@flayrpro)
set -euo pipefail
ROOT="${ROOT:-/www/wwwroot/gt4rents.com/_repo}"
cd "$ROOT"
git pull --ff-only
./deploy/build-one.sh magnific
./deploy/start-tool.sh magnific
echo "magnific redeployed — paste magnific-cookie-clean.json in panel, hard-refresh stock page"
