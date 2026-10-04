#!/bin/bash
# Server setup — run on server inside /www/wwwroot/toolsmandi.com/semr
set -e

DIR="/www/wwwroot/toolsmandi.com/semr"
PORT="7850"
BIN="semrush-go-proxy"
BUILD_TAG="semrush-multi-v1"
SERVICE="semrush-go-proxy"

port_in_use() {
  ss -tlnp 2>/dev/null | grep -qE ":${PORT}([^0-9]|$)"
}

kill_port_holders() {
  systemctl stop "$SERVICE" 2>/dev/null || true
  sleep 1
  pids=$(ss -tlnp 2>/dev/null | grep -E ":${PORT}([^0-9]|$)" | grep -oE 'pid=[0-9]+' | cut -d= -f2 | sort -u || true)
  if [ -n "$pids" ]; then
    echo "Killing port ${PORT} PIDs: $pids"
    kill -9 $pids 2>/dev/null || true
  fi
  fuser -k ${PORT}/tcp 2>/dev/null || true
  pkill -9 -f "${DIR}/${BIN}" 2>/dev/null || true
  sleep 2
}

ensure_port_free() {
  for i in 1 2 3 4 5 6; do
    if ! port_in_use; then return 0; fi
    echo "WARN: port $PORT still in use (attempt $i)"
    kill_port_holders
  done
  if port_in_use; then
    echo "ERROR: port $PORT still in use"
    ss -tlnp | grep -E ":${PORT}([^0-9]|$)" || true
    exit 1
  fi
}

cd "$DIR"
export PATH="$PATH:/usr/local/go/bin:/root/go/bin"
echo "=== Semrush SERVER SETUP ==="
echo "Folder: $DIR | Port: $PORT | Multi-domain: same port"

# aaPanel folders are often owned by www-data; root git pull needs this once
git config --global --add safe.directory "$DIR" 2>/dev/null || true
if [ -d .git ]; then
  git pull --ff-only 2>/dev/null && echo "Git pull OK" || echo "WARN: git pull skipped (run: git config --global --add safe.directory $DIR)"
fi

systemctl stop "$SERVICE" 2>/dev/null || true
systemctl reset-failed "$SERVICE" 2>/dev/null || true
kill_port_holders
ensure_port_free

if [ -f config.production.json ]; then
  PRESERVED_MYSQL_PASS=""
  if [ -f config.json ]; then
    PRESERVED_MYSQL_PASS=$(python3 -c 'import json,sys
try:
  print(json.load(open("config.json")).get("mysql_password",""))
except Exception:
  pass' 2>/dev/null || true)
  fi
  cp config.production.json config.json
  echo "Applied config.production.json → config.json"
  if [ -n "$PRESERVED_MYSQL_PASS" ] && [ "$PRESERVED_MYSQL_PASS" != "CHANGE_ME_ON_SERVER" ]; then
    PRESERVED_MYSQL_PASS="$PRESERVED_MYSQL_PASS" python3 - <<'PY'
import json, os
old = os.environ.get("PRESERVED_MYSQL_PASS", "")
cfg = json.load(open("config.json"))
cur = str(cfg.get("mysql_password") or "")
if cur in ("", "CHANGE_ME_ON_SERVER") and old:
    cfg["mysql_password"] = old
    with open("config.json", "w") as f:
        json.dump(cfg, f, indent=2)
        f.write("\n")
    print("Preserved existing mysql_password (production template had placeholder)")
PY
  fi
  if grep -q 'CHANGE_ME_ON_SERVER' config.json 2>/dev/null; then
    echo "ERROR: mysql_password is still CHANGE_ME_ON_SERVER"
    echo "       Edit config.production.json (or config.json) with the real DB password, then re-run."
    exit 1
  fi
elif [ -f config.server.json ]; then
  cp config.server.json config.json
  echo "Applied config.server.json → config.json"
else
  echo "ERROR: config.production.json or config.server.json missing"
  exit 1
fi

if ! command -v go &>/dev/null; then
  echo "ERROR: Go not installed. Try: export PATH=\$PATH:/usr/local/go/bin"
  exit 1
fi

echo "=== Build ==="
go mod tidy
go mod download
CGO_ENABLED=0 go build -buildvcs=false -ldflags="-s -w" -o "$BIN" .
if [ ! -x "$BIN" ]; then
  echo "ERROR: build failed — binary $BIN not created"
  exit 1
fi
chmod +x "$BIN"
file "$BIN"

cp semrush-go-proxy.service /etc/systemd/system/${SERVICE}.service
systemctl daemon-reload
systemctl enable "$SERVICE"

kill_port_holders
ensure_port_free
systemctl start "$SERVICE"
sleep 3

if ! systemctl is-active --quiet "$SERVICE"; then
  echo "ERROR: $SERVICE failed to start"
  journalctl -u "$SERVICE" -n 30 --no-pager
  exit 1
fi

systemctl status "$SERVICE" --no-pager | head -12
echo ""
echo "=== Health ==="
# Auth-gated / returns Access Denied without sem_session — that is NOT Cloudflare.
curl -s "http://127.0.0.1:${PORT}/__healthz" | head -c 300; echo ""
if [ -f proxy.txt ] && grep -qE 'USER:PASS@HOST|@HOST:PORT' proxy.txt 2>/dev/null; then
  echo "⚠️  proxy.txt still has the PLACEHOLDER (USER:PASS@HOST:PORT)."
  echo "   Put a REAL proxy URL, e.g.: echo 'socks5://realuser:realpass@1.2.3.4:1080' > proxy.txt"
  echo "   Then: systemctl restart $SERVICE"
fi
PROXY_LOG=$(journalctl -u "$SERVICE" -n 30 --no-pager 2>/dev/null | grep '\[PROXY\]' | tail -3)
if [ -n "$PROXY_LOG" ]; then
  echo "$PROXY_LOG"
fi

echo ""
echo "=== Multi-domain reverse proxy (same port $PORT) ==="
echo "  semr.toolsmandi.com   → website_id 3"
echo "  smrs.toolsfrog.com    → website_id 4"
echo "  sem.toolcookies.com   → website_id 5"
echo ""
echo "aaPanel: each domain → http://127.0.0.1:${PORT}"
echo "Handshake key: toolsmandi_semrush_secret_xyz123 (per row in ahrefs_websites)"
echo "Cookies: ahrefs_accounts per website_id in MySQL"
echo "Logs: journalctl -u $SERVICE -f"
