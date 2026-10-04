#!/usr/bin/env bash
# Build Next.js panel UI → /www/wwwroot/gt4rents.com/panel/ui
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
TOOLS_JSON="${ROOT}/tools.json"
BASE="$(python3 -c "import json; print(json.load(open('${TOOLS_JSON}'))['server_root'])")"
SRC="${ROOT}/panel/update-panel"
OUT="${BASE}/panel/ui"

mkdir -p "${OUT}"
cd "${SRC}"

if [[ ! -f .env.production ]]; then
  cat > .env.production <<'EOF'
# Same-origin: nginx serves UI and proxies /api → 127.0.0.1:18090
NEXT_PUBLIC_PANEL_API=
EOF
fi

if [[ ! -d node_modules ]]; then
  echo "==> npm install"
  npm install
fi

echo "==> next build"
npm run build

# Copy app for running with next start from OUT (or run from SRC)
# Prefer running from SRC after build (.next stays in SRC)
echo "OK: built ${SRC}/.next"
echo "Start UI (port 3000):"
echo "  cd ${SRC} && PORT=3000 npm run start"
echo "Nginx: panel.gt4rents.com / → :3000 , /api → :18090"
echo "Also start API: ${BASE}/panel/panel-api  (cwd must include data/panel.db)"
