#!/bin/bash
# Server setup — run inside /www/wwwroot/1clkaccess.store/erank/
set -e

DIR="/www/wwwroot/1clkaccess.store/erank"
PORT="8786"
BIN="erank-go-proxy"
BUILD_TAG="erank-v4-revert-hide"
SERVICE="erank-go-proxy"

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
echo "=== eRank SERVER SETUP ==="
echo "Folder: $DIR | Port: $PORT"

systemctl stop "$SERVICE" 2>/dev/null || true
systemctl reset-failed "$SERVICE" 2>/dev/null || true
kill_port_holders
ensure_port_free

if [ -f config.production.json ]; then
  cp config.production.json config.json
  echo "Applied config.production.json → config.json"
elif [ -f config.server.json ]; then
  cp config.server.json config.json
  echo "Applied config.server.json → config.json"
fi

PROXY_SEC="/www/wwwroot/toolsmandi.com/proxy-security"
if [ ! -d "$PROXY_SEC" ]; then
  PROXY_SEC="/www/wwwroot/1clkaccess.store/proxy-security"
fi
if [ -d "$PROXY_SEC" ]; then
  sed -i "s|replace toolsmandi.com/proxy-security => .*|replace toolsmandi.com/proxy-security => ${PROXY_SEC}|" go.mod
  echo "Using proxy-security: $PROXY_SEC"
else
  echo "WARN: proxy-security not found — copy before build"
fi

if ! command -v go &>/dev/null; then
  echo "ERROR: Go not installed."
  exit 1
fi

echo "=== Build ==="
go mod tidy
go mod download
CGO_ENABLED=0 go build -ldflags="-s -w" -o "$BIN" .
chmod +x "$BIN"
file "$BIN"
strings "$BIN" | grep -q "$BUILD_TAG" && echo "OK: $BUILD_TAG in binary" || echo "WARN: build tag missing"

cp erank-go-proxy.service /etc/systemd/system/${SERVICE}.service
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
curl -sI "http://127.0.0.1:${PORT}/" -H "User-Agent: Mozilla/5.0" | grep -i x-proxy-build || true

echo ""
echo "aaPanel: erank.1clkaccess.store → http://127.0.0.1:${PORT}"
echo "Panel: website_id=63 | secret=toolsmandi_erank_secret_xyz123"
echo "Logs: journalctl -u $SERVICE -f"
