#!/usr/bin/env bash
# Usage: ./deploy/start-tool.sh <subdomain>
set -euo pipefail
SUB="${1:-}"
if [[ -z "$SUB" ]]; then
  echo "Usage: $0 <subdomain>   e.g. refs smrs cgpt"
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
    raise SystemExit("unknown subdomain: "+sub)
PY
)
EOF

OUTDIR="${BASE}/${SUB}"
BIN="${OUTDIR}/app"
LOG="${OUTDIR}/app.log"
PIDF="${OUTDIR}/app.pid"

if [[ ! -x "${BIN}" ]]; then
  echo "Missing ${BIN} — run: ./deploy/build-one.sh ${SUB}"
  exit 1
fi
if [[ ! -f "${OUTDIR}/config.json" ]]; then
  echo "Missing ${OUTDIR}/config.json — run build-one.sh first"
  exit 1
fi

# optional mysql env
if [[ -f "${BASE}/_secrets/mysql.env" ]]; then
  # shellcheck disable=SC1090
  source "${BASE}/_secrets/mysql.env"
fi

if [[ -f "${PIDF}" ]] && kill -0 "$(cat "${PIDF}")" 2>/dev/null; then
  echo "Stopping old ${SUB} pid $(cat "${PIDF}")"
  kill "$(cat "${PIDF}")" || true
  sleep 1
fi

# free port if something else holds it
if command -v fuser >/dev/null 2>&1; then
  fuser -k "${PORT}/tcp" 2>/dev/null || true
fi

cd "${OUTDIR}"
nohup ./app >>"${LOG}" 2>&1 &
echo $! >"${PIDF}"
sleep 1
if ! kill -0 "$(cat "${PIDF}")" 2>/dev/null; then
  echo "FAIL: ${SUB} exited immediately — last log lines:"
  tail -30 "${LOG}" || true
  exit 1
fi
if ! curl -sf -o /dev/null --max-time 3 "http://127.0.0.1:${PORT}/" -H "Host: ${SUB}.gt4rents.com"; then
  # / may 403 Access Denied without cookie — treat any HTTP response as "listening"
  code="$(curl -s -o /dev/null -w '%{http_code}' --max-time 3 "http://127.0.0.1:${PORT}/" -H "Host: ${SUB}.gt4rents.com" || true)"
  if [[ -z "${code}" || "${code}" == "000" ]]; then
    echo "FAIL: nothing answering on 127.0.0.1:${PORT}"
    tail -30 "${LOG}" || true
    exit 1
  fi
fi
echo "OK: ${SUB} pid $(cat "${PIDF}") → 127.0.0.1:${PORT} host ${SUB}.gt4rents.com (log ${LOG})"
