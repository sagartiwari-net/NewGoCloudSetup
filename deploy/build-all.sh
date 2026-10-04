#!/usr/bin/env bash
# Build every tool. Failures are printed; continue.
# Usage: ./deploy/build-all.sh
# Optional: source mysql.env first so overlays get DB password.
set -uo pipefail
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
BASE="$(python3 -c "import json; print(json.load(open('${ROOT}/tools.json'))['server_root'])")"
if [[ -f "${BASE}/_secrets/mysql.env" ]]; then
  # shellcheck disable=SC1090
  source "${BASE}/_secrets/mysql.env"
fi

OK=0
FAIL=0
FAILS=()
N=0
TOTAL="$(python3 -c "import json; print(len(json.load(open('${ROOT}/tools.json'))['tools']))")"
while read -r SUB; do
  N=$((N+1))
  echo ""
  echo "======== BUILD ${N}/${TOTAL}: ${SUB} ========"
  if "${ROOT}/deploy/build-one.sh" "$SUB"; then
    OK=$((OK+1))
  else
    echo "FAIL: $SUB"
    FAIL=$((FAIL+1))
    FAILS+=("$SUB")
  fi
done < <(python3 -c "import json; print('\n'.join(t['subdomain'] for t in json.load(open('${ROOT}/tools.json'))['tools']))")

echo ""
echo "build-all Done. OK=${OK} FAIL=${FAIL} / ${TOTAL}"
if [[ "${FAIL}" -gt 0 ]]; then
  echo "Failed subs: ${FAILS[*]}"
fi
