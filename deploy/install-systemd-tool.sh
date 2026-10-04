#!/usr/bin/env bash
# Install one tool as systemd unit: ./deploy/install-systemd-tool.sh refs
set -euo pipefail
SUB="${1:-}"
if [[ -z "$SUB" ]]; then
  echo "Usage: $0 <subdomain>"
  exit 1
fi
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
read -r PORT <<EOF
$(python3 - <<PY
import json
sub="${SUB}"
for t in json.load(open("${ROOT}/tools.json"))["tools"]:
    if t["subdomain"]==sub:
        print(t["port"]); break
else:
    raise SystemExit("unknown subdomain")
PY
)
EOF
UNIT="/etc/systemd/system/gt4-${SUB}.service"
sed "s/__SUB__/${SUB}/g; s/__PORT__/${PORT}/g" \
  "${ROOT}/deploy/systemd/gt4-tool.service.template" > "${UNIT}"
systemctl daemon-reload
systemctl enable --now "gt4-${SUB}.service"
systemctl --no-pager status "gt4-${SUB}.service" | head -15
echo "OK: systemctl restart gt4-${SUB}   # after rebuild"
