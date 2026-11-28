#!/usr/bin/env bash
# Self-check: FP bind + proxy + panel Ahrefs APIs (no -f on expected 401s).
set -euo pipefail
BAR="${BAR_API:-http://127.0.0.1:8088}"
PANEL="${PANEL_API:-http://127.0.0.1:18090}"
FP="tmfp.smoke-$(python3 -c 'import uuid;print(uuid.uuid4())')"

echo "== portals =="
curl -sf "$BAR/v1/portals" | python3 -c 'import sys,json;d=json.load(sys.stdin); assert d.get("success"); print("ok", len(d["portals"]))'

echo "== mint without FP =="
code=$(curl -s -o /tmp/smoke_nofp.json -w '%{http_code}' -X POST "$BAR/v1/session/verify" \
  -H 'Content-Type: application/json' -d '{"token":"dev-smoke","portal":"https://local.dev"}')
python3 -c 'import json;d=json.load(open("/tmp/smoke_nofp.json")); assert d.get("error")=="fingerprint_required"'
test "$code" = "401"
echo "ok fingerprint_required"

echo "== mint with FP =="
TOK=$(curl -sf -X POST "$BAR/v1/session/verify" \
  -H 'Content-Type: application/json' -H "X-TM-Bar-FP: $FP" \
  -d "{\"token\":\"dev-smoke2\",\"portal\":\"https://local.dev\",\"fingerprint\":\"$FP\"}" \
  | python3 -c 'import sys,json;d=json.load(sys.stdin); assert d.get("session_token"); print(d["session_token"])')

echo "== wrong FP =="
curl -s -H "X-TM-Bar-Token: $TOK" -H 'X-TM-Bar-FP: tmfp.other' \
  "$BAR/v4/tbGetIconV3?input=%7B%22target%22%3A%22smoke.com%22%7D" \
  | python3 -c 'import sys,json;d=json.load(sys.stdin); assert d[1]=="fingerprint_mismatch"; print("ok mismatch")'

echo "== icon + cache =="
curl -sf -H "X-TM-Bar-Token: $TOK" -H "X-TM-Bar-FP: $FP" \
  "$BAR/v4/tbGetIconV3?input=%7B%22target%22%3A%22smoke.com%22%7D" \
  | python3 -c 'import sys,json;d=json.load(sys.stdin); assert d[0]=="Ok"; print("ok icon")'
hit=$(curl -sD - -o /dev/null -H "X-TM-Bar-Token: $TOK" -H "X-TM-Bar-FP: $FP" \
  "$BAR/v4/tbGetIconV3?input=%7B%22target%22%3A%22smoke.com%22%7D" | tr -d '\r' | awk -F': ' 'tolower($1)=="x-tm-bar-cache"{print $2}')
test "$hit" = "HIT"
echo "ok cache HIT"

echo "== panel =="
curl -sf -c /tmp/smoke_panel.txt -X POST "$PANEL/api/login" \
  -H 'Content-Type: application/json' -d '{"username":"master","password":"toolsmandi"}' >/dev/null
curl -sf -b /tmp/smoke_panel.txt "$PANEL/api/ahrefs-bar/overview" \
  | python3 -c 'import sys,json;d=json.load(sys.stdin); print("accounts", d.get("accounts_total"), "portals", d.get("portals_total"))'
curl -sf -b /tmp/smoke_panel.txt -o /tmp/smoke.zip "$PANEL/api/ahrefs-bar/extension.zip"
python3 - <<'PY'
import zipfile
z=zipfile.ZipFile('/tmp/smoke.zip')
assert 'ahrefs-bar/tm-config.js' in z.namelist()
assert not any('_metadata' in n for n in z.namelist())
print('ok zip', len(z.namelist()))
PY

echo "ALL SMOKE PASSED fp=$FP"
