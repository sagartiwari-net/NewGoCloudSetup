#!/bin/bash
# Server setup — run inside /www/wwwroot/1clkaccess.store/hm
set -e

DIR="/www/wwwroot/1clkaccess.store/hm"
PORT="5151"
BIN="helium10-go-proxy"
SERVICE="helium10-go-proxy"

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
echo "=== Helium10 SERVER SETUP ==="
echo "Folder: $DIR | Port: $PORT | Domain: hm.1clkaccess.store"

git config --global --add safe.directory "$DIR" 2>/dev/null || true
if [ -d .git ]; then
  git pull --ff-only 2>/dev/null && echo "Git pull OK" || echo "WARN: git pull skipped"
fi

systemctl stop "$SERVICE" 2>/dev/null || true
systemctl reset-failed "$SERVICE" 2>/dev/null || true
kill_port_holders
ensure_port_free

if [ -f config.production.json ]; then
  PRESERVED_MYSQL_PASS=""
  if [ -f config.json ]; then
    PRESERVED_MYSQL_PASS=$(python3 -c 'import json
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
    print("Preserved existing mysql_password")
PY
  fi
elif [ -f config.server.json ]; then
  cp config.server.json config.json
  echo "Applied config.server.json → config.json"
else
  echo "ERROR: config.production.json missing"
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
chmod +x "$BIN"
file "$BIN"

cp helium10-go-proxy.service /etc/systemd/system/${SERVICE}.service
systemctl daemon-reload
systemctl enable "$SERVICE"

kill_port_holders
ensure_port_free
systemctl start "$SERVICE"
sleep 3

if ! systemctl is-active --quiet "$SERVICE"; then
  echo "ERROR: $SERVICE failed to start"
  journalctl -u "$SERVICE" -n 40 --no-pager
  exit 1
fi

systemctl status "$SERVICE" --no-pager | head -14
echo ""
echo "=== Health ==="
curl -s "http://127.0.0.1:${PORT}/" -o /tmp/hm-health.body -w "HTTP %{http_code}\n" -H "User-Agent: Mozilla/5.0" || true
head -c 180 /tmp/hm-health.body 2>/dev/null; echo ""

echo ""
echo "Domain: https://hm.1clkaccess.store"
echo "aaPanel reverse proxy → http://127.0.0.1:${PORT}"
echo "Handshake: /api/auth-handshake (secret from ahrefs_websites id=130)"
echo "Cookies: ahrefs_accounts for website_id=130"
echo "Logs: journalctl -u $SERVICE -f"
