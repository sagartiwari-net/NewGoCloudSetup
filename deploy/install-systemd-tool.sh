#!/usr/bin/env bash
# Install one tool as a boot service: ./deploy/install-systemd-tool.sh refs
# Refuses ports that belong to other apps (payment-hub :8090, smpanel :5001).
set -euo pipefail
SUB="${1:-}"
if [[ -z "$SUB" ]]; then
  echo "Usage: $0 <subdomain>"
  exit 1
fi
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
read -r PORT BASE <<EOF
$(python3 - <<PY
import json
sub="${SUB}"
data=json.load(open("${ROOT}/tools.json"))
for t in data["tools"]:
    if t["subdomain"]==sub:
        print(t["port"], data["server_root"])
        break
else:
    raise SystemExit("unknown subdomain")
PY
)
EOF
if [[ "${PORT}" == "8090" || "${PORT}" == "5001" || "${PORT}" == "18090" || "${PORT}" == "3000" || "${PORT}" == "3306" ]]; then
  echo "SKIP: ${SUB} port ${PORT} is reserved"
  exit 0
fi
if [[ ! -x "${BASE}/${SUB}/app" ]]; then
  echo "SKIP: ${SUB} has no built app"
  exit 0
fi
UNIT="/etc/systemd/system/gt4-${SUB}.service"
sed "s/__SUB__/${SUB}/g; s/__PORT__/${PORT}/g" \
  "${ROOT}/deploy/systemd/gt4-tool.service.template" > "${UNIT}"
PIDF="${BASE}/${SUB}/app.pid"
if [[ -f "${PIDF}" ]] && kill -0 "$(cat "${PIDF}")" 2>/dev/null; then
  kill "$(cat "${PIDF}")" 2>/dev/null || true
  sleep 1
fi
rm -f "${PIDF}"
if command -v fuser >/dev/null 2>&1; then
  fuser -k "${PORT}/tcp" >/dev/null 2>&1 || true
fi
systemctl daemon-reload
systemctl enable "gt4-${SUB}.service"
systemctl restart "gt4-${SUB}.service"
echo "OK: gt4-${SUB} port ${PORT}"
