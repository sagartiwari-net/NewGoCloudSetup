#!/usr/bin/env bash
# Create /www/wwwroot/gt4rents.com/<subdomain> for every tool + panel
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
TOOLS_JSON="${ROOT}/tools.json"
BASE="$(python3 -c "import json; print(json.load(open('${TOOLS_JSON}'))['server_root'])")"

mkdir -p "${BASE}/panel/data"
mkdir -p "${BASE}/_repo"

python3 - <<PY
import json, os
from pathlib import Path
data=json.load(open("${TOOLS_JSON}"))
base=Path(data["server_root"])
for t in data["tools"]:
    d = base / t["subdomain"]
    d.mkdir(parents=True, exist_ok=True)
    print("dir", d)
print("panel", base / "panel")
PY

echo "OK: directories under ${BASE}"
