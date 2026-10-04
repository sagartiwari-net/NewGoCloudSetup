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

# Base config from tool source, then always apply server overlay (public_host/port/panel_db)
if [[ -f "${SRC}/config.json" ]]; then
  cp "${SRC}/config.json" "${OUTDIR}/config.json"
elif [[ ! -f "${OUTDIR}/config.json" ]]; then
  echo '{}' > "${OUTDIR}/config.json"
fi

# Load mysql password from server secrets if present
if [[ -f "${BASE}/_secrets/mysql.env" ]]; then
  # shellcheck disable=SC1090
  source "${BASE}/_secrets/mysql.env"
fi

OVERLAY="${ROOT}/deploy/configs/${SUB}.config.overlay.json"
python3 - <<PY
import json, os
from pathlib import Path
out = Path("${OUTDIR}/config.json")
base = json.loads(out.read_text()) if out.exists() else {}
ov = json.loads(Path("${OVERLAY}").read_text()) if Path("${OVERLAY}").exists() else {}
for k in ("port","public_host","public_scheme","panel_db","local_test_mode","bypass_auth",
          "mysql_host","mysql_port","mysql_user","mysql_db"):
    if k in ov:
        base[k] = ov[k]
pw = os.environ.get("GT4RENTS_MYSQL_PASSWORD")
if pw:
    base["mysql_password"] = pw
# server defaults
base["port"] = str(base.get("port") or "${PORT}")
base["public_host"] = base.get("public_host") or "${FQDN}"
# Force http until SSL; set TOOL_PUBLIC_SCHEME=https in mysql.env after LE
# (live asset rewrite still follows X-Forwarded-Proto; this is config fallback only)
_scheme = (os.environ.get("TOOL_PUBLIC_SCHEME") or "http").strip().lower()
base["public_scheme"] = "https" if _scheme == "https" else "http"
base["panel_db"] = base.get("panel_db") or "${BASE}/panel/data/panel.db"
if "local_test_mode" in base:
    base["local_test_mode"] = False
if "bypass_auth" in base:
    base["bypass_auth"] = False
out.write_text(json.dumps(base, indent=2) + "\n")
print("wrote", out)
PY

cat > "${OUTDIR}/start.sh" <<EOF
#!/usr/bin/env bash
cd "\$(dirname "\$0")"
exec ./app
EOF
chmod +x "${OUTDIR}/start.sh"

echo "OK: ${OUTDIR}/app"
echo "Start: ${ROOT}/deploy/start-tool.sh ${SUB}"
