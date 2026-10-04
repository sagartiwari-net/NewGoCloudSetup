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

# Base config from tool source.
# Prefer gt4rents panel template / local config.json — NEVER prefer config.production.json
# (that file is often an old toolsmandi deploy with use_database + session_security on).
# Order: config.server.json → config.json → config.production.json → keep OUTDIR → {}
if [[ -f "${SRC}/config.server.json" ]]; then
  cp "${SRC}/config.server.json" "${OUTDIR}/config.json"
elif [[ -f "${SRC}/config.json" ]]; then
  cp "${SRC}/config.json" "${OUTDIR}/config.json"
elif [[ -f "${SRC}/config.production.json" ]]; then
  cp "${SRC}/config.production.json" "${OUTDIR}/config.json"
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
          "mysql_host","mysql_port","mysql_user","mysql_db",
          "target_url","cdn_url","tool_name","cookie_domain_suffix","home_path",
          "use_database","inject_css"):
    if k in ov:
        base[k] = ov[k]
pw = os.environ.get("GT4RENTS_MYSQL_PASSWORD")
if pw:
    base["mysql_password"] = pw
# server defaults
base["port"] = str(base.get("port") or "${PORT}")
base["public_host"] = base.get("public_host") or "${FQDN}"
# Prefer TOOL_PUBLIC_SCHEME from mysql.env; else keep overlay/base (do not force http
# and blank HTTPS tools with mixed-content rewrites).
_scheme = (os.environ.get("TOOL_PUBLIC_SCHEME") or "").strip().lower()
if _scheme in ("http", "https"):
    base["public_scheme"] = _scheme
elif str(base.get("public_scheme") or "").strip().lower() not in ("http", "https"):
    base["public_scheme"] = "http"
base["panel_db"] = base.get("panel_db") or "${BASE}/panel/data/panel.db"
if "local_test_mode" in base:
    base["local_test_mode"] = False
if "bypass_auth" in base:
    base["bypass_auth"] = False
# Panel.db mode: do NOT enable old MySQL session_security / use_database from production templates.
# That combo breaks ChatGPT send (API gated) while HTML shell still loads.
if str(base.get("panel_db") or "").strip():
    base["use_database"] = False
    base["local_test_mode"] = False
    # Neutralize toolsmandi leftovers if a production template was used as base
    if isinstance(base.get("logout_detection"), dict):
        base["logout_detection"]["enabled"] = False
    if isinstance(base.get("automation"), dict):
        base["automation"]["enabled"] = False
    if isinstance(base.get("session_security"), dict):
        base["session_security"]["enabled"] = False
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
