#!/usr/bin/env bash
# Start tools in batches (avoid RAM spike from launching ~80 at once).
# Usage:
#   ./deploy/start-batch.sh              # batch size 8 (default)
#   ./deploy/start-batch.sh 10           # batch size 10
#   ./deploy/start-batch.sh 8 0 20       # size 8, skip 0, start only first 20
#   BATCH_SLEEP=15 ./deploy/start-batch.sh 8
set -uo pipefail
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
BATCH="${1:-8}"
OFFSET="${2:-0}"
LIMIT="${3:-0}"   # 0 = all remaining after OFFSET
SLEEP_BETWEEN="${BATCH_SLEEP:-12}"

BASE="$(python3 -c "import json; print(json.load(open('${ROOT}/tools.json'))['server_root'])")"
if [[ -f "${BASE}/_secrets/mysql.env" ]]; then
  # shellcheck disable=SC1090
  source "${BASE}/_secrets/mysql.env"
fi

mapfile -t SUBS < <(python3 - <<PY
import json
subs=[t["subdomain"] for t in json.load(open("${ROOT}/tools.json"))["tools"]]
off=int("${OFFSET}")
lim=int("${LIMIT}")
subs=subs[off:]
if lim>0:
    subs=subs[:lim]
print("\n".join(subs))
PY
)

TOTAL="${#SUBS[@]}"
if [[ "${TOTAL}" -eq 0 ]]; then
  echo "No tools to start (offset=${OFFSET} limit=${LIMIT})"
  exit 0
fi

echo "Starting ${TOTAL} tools in batches of ${BATCH} (sleep ${SLEEP_BETWEEN}s between batches)"
OK=0
FAIL=0
SKIP=0
i=0
batch_num=0
while [[ $i -lt $TOTAL ]]; do
  batch_num=$((batch_num+1))
  end=$((i+BATCH))
  if [[ $end -gt $TOTAL ]]; then end=$TOTAL; fi
  echo ""
  echo "======== BATCH ${batch_num}: tools $((i+1))–${end} / ${TOTAL} ========"
  for ((j=i; j<end; j++)); do
    SUB="${SUBS[$j]}"
    BIN="${BASE}/${SUB}/app"
    if [[ ! -x "${BIN}" ]]; then
      echo "SKIP: ${SUB} (no binary — build failed or not built)"
      SKIP=$((SKIP+1))
      continue
    fi
    if "${ROOT}/deploy/start-tool.sh" "${SUB}"; then
      OK=$((OK+1))
    else
      echo "FAIL: start ${SUB}"
      FAIL=$((FAIL+1))
    fi
  done
  i=$end
  if [[ $i -lt $TOTAL ]]; then
    echo "… sleeping ${SLEEP_BETWEEN}s before next batch …"
    sleep "${SLEEP_BETWEEN}"
  fi
done

echo ""
echo "start-batch done. OK=${OK} FAIL=${FAIL} SKIP=${SKIP} (total listed=${TOTAL})"
if [[ "${FAIL}" -gt 0 ]]; then
  exit 1
fi
