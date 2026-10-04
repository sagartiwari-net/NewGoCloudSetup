#!/bin/bash
# Server setup — run inside /www/wwwroot/1clkaccess.store/sellamp
set -e

DIR="/www/wwwroot/1clkaccess.store/sellamp"
PORT="5152"
BIN="selleramp-go-proxy"
SERVICE="selleramp-go-proxy"

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
echo "=== SellerAmp SAS SERVER SETUP ==="
echo "Folder: $DIR | Port: $PORT | Domain: sellamp.1clkaccess.store"

git config --global --add safe.directory "$DIR" 2>/dev/null || true
if [ -d .git ]; then
  git pull --ff-only 2>/dev/null && echo "Git pull OK" || echo "WARN: git pull skipped"
fi

systemctl stop "$SERVICE" 2>/dev/null || true
systemctl reset-failed "$SERVICE" 2>/dev/null || true
kill_port_holders
ensure_port_free

if [ -f config.production.json ]; then
  cp config.production.json config.json
  echo "Applied config.production.json → config.json"
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

cp selleramp-go-proxy.service /etc/systemd/system/${SERVICE}.service
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
curl -s "http://127.0.0.1:${PORT}/" -o /tmp/sellamp-health.body -w "HTTP %{http_code}\n" -H "User-Agent: Mozilla/5.0" || true
head -c 180 /tmp/sellamp-health.body 2>/dev/null; echo ""

echo ""
echo "Domain: https://sellamp.1clkaccess.store"
echo "aaPanel reverse proxy → http://127.0.0.1:${PORT}"
echo "Handshake: /api/auth-handshake (secret from ahrefs_websites id=131)"
echo "Cookies: ahrefs_accounts for website_id=131"
echo "Logs: journalctl -u $SERVICE -f"
