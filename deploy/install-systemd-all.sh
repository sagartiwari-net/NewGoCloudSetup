#!/usr/bin/env bash
# Boot-start every built one-click tool. Does not touch :8090 or :5001.
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
chmod +x "${ROOT}/deploy/run-tool.sh" "${ROOT}/deploy/install-systemd-tool.sh"
mapfile -t SUBS < <(python3 - <<PY
import json
data=json.load(open("${ROOT}/tools.json"))
for t in data["tools"]:
    print(t["subdomain"])
PY
)
ok=0
skip=0
for sub in "${SUBS[@]}"; do
  if "${ROOT}/deploy/install-systemd-tool.sh" "${sub}"; then
    ok=$((ok + 1))
  else
    echo "FAIL: ${sub}"
    skip=$((skip + 1))
  fi
done
echo "OK: ${ok} tool services processed, ${skip} failed. :8090 and :5001 were not used."
