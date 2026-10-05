#!/usr/bin/env bash
# Rebuild + restart every tool that still needs the token-only /access binary.
# Skips tools already verified after fad80ac+ redeploy (override with SKIP=...).
#
# Usage:
#   cd /www/wwwroot/gt4rents.com/_repo
#   git pull --ff-only origin main
#   source /www/wwwroot/gt4rents.com/_secrets/mysql.env
#   chmod +x deploy/rebuild-token-access.sh
#   ./deploy/rebuild-token-access.sh
#
# Optional:
#   SKIP="refs smrs cgpt" ./deploy/rebuild-token-access.sh
#   ONLY="grammarly helium10 cnva" ./deploy/rebuild-token-access.sh
set -uo pipefail
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
BASE="$(python3 -c "import json; print(json.load(open('${ROOT}/tools.json'))['server_root'])")"
if [[ -f "${BASE}/_secrets/mysql.env" ]]; then
  # shellcheck disable=SC1090
  source "${BASE}/_secrets/mysql.env"
fi

# Already confirmed OK with token-only open after recent rebuilds.
DEFAULT_SKIP="refs smrs cgpt clud envt cnva"
SKIP="${SKIP:-$DEFAULT_SKIP}"
ONLY="${ONLY:-}"

mapfile -t ALL < <(python3 -c "import json; print('\n'.join(t['subdomain'] for t in json.load(open('${ROOT}/tools.json'))['tools']))")

should_run() {
  local sub="$1"
  if [[ -n "$ONLY" ]]; then
    [[ " $ONLY " == *" $sub "* ]]
    return $?
  fi
  [[ " $SKIP " != *" $sub "* ]]
}

OK=0
FAIL=0
SKIPPED=0
FAILS=()
TARGETS=()
for sub in "${ALL[@]}"; do
  if should_run "$sub"; then
    TARGETS+=("$sub")
  else
    SKIPPED=$((SKIPPED + 1))
  fi
done

TOTAL=${#TARGETS[@]}
echo "rebuild-token-access: ${TOTAL} tools (skip ${SKIPPED}: ${SKIP})"
echo "targets: ${TARGETS[*]}"
echo ""

N=0
for sub in "${TARGETS[@]}"; do
  N=$((N + 1))
  echo ""
  echo "======== ${N}/${TOTAL}: ${sub} ========"
  if "${ROOT}/deploy/build-one.sh" "$sub" && "${ROOT}/deploy/start-tool.sh" "$sub"; then
    OK=$((OK + 1))
  else
    echo "FAIL: $sub"
    FAIL=$((FAIL + 1))
    FAILS+=("$sub")
  fi
done

echo ""
echo "rebuild-token-access Done. OK=${OK} FAIL=${FAIL} SKIP=${SKIPPED} / planned=${TOTAL}"
if [[ "${FAIL}" -gt 0 ]]; then
  echo "Failed subs: ${FAILS[*]}"
  exit 1
fi
