#!/usr/bin/env bash
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
SRC="${ROOT}/panel/update-panel"
LOG="${SRC}/panel-ui.log"
PIDF="${SRC}/panel-ui.pid"
PORT="${PORT:-3000}"

if [[ ! -d "${SRC}/.next" ]]; then
  echo "No .next build — run ./deploy/build-panel-ui.sh first"
  exit 1
fi
if [[ ! -d "${SRC}/node_modules" ]]; then
  echo "No node_modules — run ./deploy/build-panel-ui.sh first"
  exit 1
fi

if [[ -f "${PIDF}" ]] && kill -0 "$(cat "${PIDF}")" 2>/dev/null; then
  echo "Stopping old panel-ui pid $(cat "${PIDF}")"
  kill "$(cat "${PIDF}")" || true
  sleep 1
fi
# The pid file is npm. Its child can keep the port after npm exits, and
# the next start then dies with EADDRINUSE while the old process keeps serving.
if command -v fuser >/dev/null 2>&1; then
  fuser -k "${PORT}/tcp" >/dev/null 2>&1 || true
  sleep 1
fi

cd "${SRC}"
if [[ ! -f .env.production ]]; then
  echo 'NEXT_PUBLIC_PANEL_API=' > .env.production
fi

nohup env PORT="${PORT}" NODE_OPTIONS="--max-http-header-size=524288" npm run start >>"${LOG}" 2>&1 &
echo $! >"${PIDF}"
echo "OK: panel-ui pid $(cat "${PIDF}") → 127.0.0.1:${PORT} (log ${LOG})"

ready=0
for _ in 1 2 3 4 5 6 7 8 9 10 11 12 13 14 15; do
  if curl -sf -o /tmp/panel-boot.html "http://127.0.0.1:${PORT}/accounts"; then
    ready=1
    break
  fi
  sleep 1
done
if [[ "${ready}" != "1" ]]; then
  echo "panel-ui did not answer on :${PORT}. Last log lines:"
  tail -n 40 "${LOG}" || true
  exit 1
fi
stale_code="$(curl -s -o /tmp/panel-stale.js -w '%{http_code}' "http://127.0.0.1:${PORT}/_next/static/chunks/turbopack-missing-panel.js")"
if [[ "${stale_code}" != "200" ]] || ! grep -q panel-static-reload /tmp/panel-stale.js; then
  echo "missing static file returned HTTP ${stale_code}, not the reload script"
  exit 1
fi
echo "stale chunk returns a one-time page reload"
python3 - "${PORT}" <<'PY'
import re, sys, urllib.request
port = sys.argv[1]
html = open("/tmp/panel-boot.html", encoding="utf-8", errors="replace").read()
if "This page couldn’t load" in html or "This page couldn't load" in html:
    raise SystemExit("accounts HTML is the Next error page")
refs = set(re.findall(r'\s(?:src|href)="(/_next/static/[^"]+)"', html))
bad = []
for ref in sorted(refs):
    url = f"http://127.0.0.1:{port}{ref}"
    try:
        with urllib.request.urlopen(url, timeout=15) as res:
            if res.status != 200:
                bad.append((res.status, ref))
    except Exception as exc:
        bad.append((exc, ref))
if bad:
    print("panel is serving accounts HTML that points at missing files:")
    for item in bad:
        print(" ", item)
    raise SystemExit(1)
print(f"accounts page ok, {len(refs)} static files present, css inlined={('<style' in html)}")
PY
