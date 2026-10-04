#!/usr/bin/env bash
# First-draft: build every tool. Failures are printed; continue.
set -uo pipefail
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
OK=0
FAIL=0
while read -r SUB; do
  if "${ROOT}/deploy/build-one.sh" "$SUB"; then
    OK=$((OK+1))
  else
    echo "FAIL: $SUB"
    FAIL=$((FAIL+1))
  fi
done < <(python3 -c "import json; print('\n'.join(t['subdomain'] for t in json.load(open('${ROOT}/tools.json'))['tools']))")

echo "Done. OK=${OK} FAIL=${FAIL}"
