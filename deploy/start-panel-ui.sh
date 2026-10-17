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

if systemctl is-enabled --quiet gt4-panel-ui.service 2>/dev/null; then
  systemctl restart gt4-panel-ui.service
  echo "OK: systemd gt4-panel-ui → 127.0.0.1:${PORT} (log ${LOG})"
else
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

# A previous build prerendered "/" and "/accounts" for a year. Chrome still
# receives that HTML. Drop the on-disk copy before this process starts.
python3 - "${SRC}/.next" <<'PY'
import os, sys
root = sys.argv[1]
removed = 0
for dirpath, _, files in os.walk(root):
    if f"{os.sep}cache{os.sep}webpack{os.sep}" in dirpath + os.sep:
        continue
    for name in files:
        path = os.path.join(dirpath, name)
        try:
            if os.path.getsize(path) > 30_000_000:
                continue
            data = open(path, "rb").read()
        except OSError:
            continue
        if b"turbopack-" in data and b"Redirecting to sign in" in data:
            os.remove(path)
            removed += 1
            print("removed stale prerender", path)
print(f"stale prerender files removed: {removed}")
PY
rm -rf "${SRC}/.next/cache/fetch-cache"

nohup env PORT="${PORT}" NODE_OPTIONS="--max-http-header-size=524288" npm run start >>"${LOG}" 2>&1 &
echo $! >"${PIDF}"
echo "OK: panel-ui pid $(cat "${PIDF}") → 127.0.0.1:${PORT} (log ${LOG})"
fi

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
if [[ "${stale_code}" != "200" ]] || ! grep -q 'searchParams.has("__v")' /tmp/panel-stale.js; then
  echo "missing static file returned HTTP ${stale_code}, not the reload script"
  exit 1
fi
echo "stale chunk returns a page reload"
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

# Chrome's Accept-Encoding was the request that received the year-long prerender.
python3 - "${PORT}" <<'PY'
import gzip, sys, urllib.request
port = sys.argv[1]
ua = "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/154.0.0.0 Safari/537.36"
headers = {
    "User-Agent": ua,
    "Accept": "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8",
    "Accept-Encoding": "gzip, deflate, br, zstd",
    "Sec-Fetch-Dest": "document",
    "Sec-Fetch-Mode": "navigate",
    "Sec-Fetch-Site": "none",
}

def fetch(url):
    req = urllib.request.Request(url, headers=headers)
    with urllib.request.urlopen(req, timeout=20) as res:
        raw = res.read()
        info = {k.lower(): v for k, v in res.headers.items()}
    if raw[:2] == b"\x1f\x8b":
        raw = gzip.decompress(raw)
    return info, raw

for label, url in (
    ("localhost", f"http://127.0.0.1:{port}/"),
    ("public", "https://panel.gt4rents.com/"),
):
    info, body = fetch(url)
    bad = b"Redirecting to sign in" in body or b"turbopack-" in body
    print(
        label,
        "bytes", len(body),
        "cache", info.get("x-nextjs-cache", "-"),
        "stale", bad,
    )
    if bad:
        raise SystemExit(label + " is still serving the previous panel HTML")
print("chrome document request is the current build")
PY
