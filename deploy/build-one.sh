#!/usr/bin/env bash
# Usage: ./deploy/build-one.sh <subdomain>
# Builds tools/<folder> → /www/wwwroot/gt4rents.com/<subdomain>/app
set -euo pipefail
SUB="${1:-}"
if [[ -z "$SUB" ]]; then
  echo "Usage: $0 <subdomain>   e.g. refs smrs cgpt"
  exit 1
fi
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
TOOLS_JSON="${ROOT}/tools.json"

read -r FOLDER PORT FQDN BASE <<EOF
$(python3 - <<PY
import json
sub="${SUB}"
data=json.load(open("${TOOLS_JSON}"))
for t in data["tools"]:
    if t["subdomain"]==sub:
        print(t["folder"], t["port"], t["fqdn"], data["server_root"])
        break
else:
    raise SystemExit("unknown subdomain: "+sub)
PY
)
EOF

SRC="${ROOT}/tools/${FOLDER}"
OUTDIR="${BASE}/${SUB}"
mkdir -p "${OUTDIR}"

echo "Building ${FOLDER} → ${OUTDIR}/app (port ${PORT}, host ${FQDN})"
cd "${SRC}"
go build -o "${OUTDIR}/app" .

# Merge overlay if no config.json yet
if [[ ! -f "${OUTDIR}/config.json" ]]; then
  if [[ -f "${SRC}/config.json" ]]; then
    cp "${SRC}/config.json" "${OUTDIR}/config.json"
  fi
fi
OVERLAY="${ROOT}/deploy/configs/${SUB}.config.overlay.json"
if [[ -f "${OVERLAY}" ]]; then
  python3 - <<PY
import json
from pathlib import Path
out = Path("${OUTDIR}/config.json")
base = json.loads(out.read_text()) if out.exists() else {}
ov = json.loads(Path("${OVERLAY}").read_text())
# apply key host/port/panel fields
for k in ("port","public_host","public_scheme","panel_db","local_test_mode","bypass_auth",
          "mysql_host","mysql_port","mysql_user","mysql_db"):
    if k in ov:
        base[k] = ov[k]
# password from env if set
import os
pw = os.environ.get("GT4RENTS_MYSQL_PASSWORD")
if pw:
    base["mysql_password"] = pw
elif "mysql_password" in ov and not str(ov["mysql_password"]).startswith("${"):
    base["mysql_password"] = ov["mysql_password"]
out.write_text(json.dumps(base, indent=2) + "\n")
print("wrote", out)
PY
fi

echo "OK: ${OUTDIR}/app"
echo "Run example:"
echo "  cd ${OUTDIR} && CONFIG_FILE=./config.json ./app"
echo "  # or tool-specific ENV — check main.go for config path"
