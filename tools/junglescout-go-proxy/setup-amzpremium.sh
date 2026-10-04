#!/bin/bash
# Jungle Scout — amzpremiumsoftware.com (port 8787; 8785 = 1clkaccess jungle on same VPS)
set -e

DIR="/www/wwwroot/amzpremiumsoftware.com/jungle"
PARENT="/www/wwwroot/amzpremiumsoftware.com"
SRC="/www/wwwroot/1clkaccess.store/jungle"
PORT="8787"
BIN="junglescout-go-proxy"
BUILD_TAG="jungle-v11-datadome-fix"
SERVICE="jungle-scout-amzpremium"

port_in_use() {
  ss -tlnp 2>/dev/null | grep -qE ":${PORT}([^0-9]|$)"
}

port_holders() {
  ss -tlnp 2>/dev/null | grep -E ":${PORT}([^0-9]|$)" || true
}

kill_port_holders() {
  systemctl stop "$SERVICE" 2>/dev/null || true
  sleep 1
  local pids
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
  local label="$1"
  for i in 1 2 3 4 5 6; do
    if ! port_in_use; then
      return 0
    fi
    echo "WARN: port $PORT still in use during ${label} (attempt $i)"
    port_holders
    kill_port_holders
  done
  if port_in_use; then
    echo "ERROR: port $PORT still in use. Run: ss -tlnp | grep ${PORT}"
    port_holders
    exit 1
  fi
}

find_proxy_security() {
  local p
  for p in \
    "/www/wwwroot/toolsmandi.com/proxy-security" \
    "/www/wwwroot/1clkaccess.store/proxy-security" \
    "/www/wwwroot/amzpremiumsoftware.com/proxy-security" \
    "/www/wwwroot/oneclickgo/proxy-security"; do
    if [ -d "$p" ]; then
      echo "$p"
      return 0
    fi
  done
  find /www/wwwroot -maxdepth 6 -type d -name "proxy-security" 2>/dev/null | head -1
}

echo "=== Jungle Scout: amzpremiumsoftware.com (port $PORT) ==="
mkdir -p "$DIR"
cd "$DIR"

# 1) Copy latest Go source from working 1clkaccess install (if present)
if [ -f "$SRC/main.go" ]; then
  echo "Copying source from $SRC ..."
  cp "$SRC"/*.go "$DIR"/
  cp "$SRC/go.mod" "$SRC/go.sum" "$DIR"/ 2>/dev/null || true
fi
for f in config.amzpremium.json jungle-scout-amzpremium.service kill-port-amzpremium.sh; do
  if [ ! -f "$DIR/$f" ] && [ -f "$SRC/$f" ]; then
    cp "$SRC/$f" "$DIR/$f"
    echo "Copied $f from $SRC"
  fi
done

# 2) proxy-security
PROXY_SEC="$(find_proxy_security || true)"
if [ -z "$PROXY_SEC" ]; then
  echo "ERROR: proxy-security not found on server."
  exit 1
fi
echo "Using proxy-security: $PROXY_SEC"
if [ "$PROXY_SEC" != "$PARENT/proxy-security" ] && [ ! -d "$PARENT/proxy-security" ]; then
  ln -sf "$PROXY_SEC" "$PARENT/proxy-security"
fi

# 3) Config — use existing config.json if you already created it on server
if [ -f config.json ]; then
  echo "Using existing config.json"
elif [ -f config.amzpremium.json ]; then
  cp config.amzpremium.json config.json
  echo "Applied config.amzpremium.json → config.json"
elif [ -f "$SRC/config.amzpremium.json" ]; then
  cp "$SRC/config.amzpremium.json" config.json
  echo "Copied config from $SRC/config.amzpremium.json"
elif [ -f "$SRC/config.production.json" ]; then
  echo "Generating config.json from $SRC/config.production.json ..."
  sed \
    -e 's/"port": "8785"/"port": "8787"/' \
    -e 's/jungle.1clkaccess.store/jungle.amzpremiumsoftware.com/g' \
    -e 's/toolsmandi_jungle_secret_xyz123/amzpremiumsoftware_jungle_secret_xyz123/g' \
    -e 's|https://toolsmandi.com/|https://amzpremiumsoftware.com/|g' \
    "$SRC/config.production.json" > config.json
else
  echo "ERROR: Put config.json in $DIR (port 8787, public_host jungle.amzpremiumsoftware.com)"
  exit 1
fi

# Quick sanity check
grep -q '"port": "8787"' config.json || echo "WARN: config.json port should be 8787"
grep -q 'jungle.amzpremiumsoftware.com' config.json || echo "WARN: public_host should be jungle.amzpremiumsoftware.com"
touch cookie.txt
chmod +x kill-port-amzpremium.sh 2>/dev/null || true

echo "=== Stop service + free port $PORT ==="
systemctl stop "$SERVICE" 2>/dev/null || true
systemctl reset-failed "$SERVICE" 2>/dev/null || true
kill_port_holders
ensure_port_free "initial cleanup"

if ! command -v go &>/dev/null; then
  echo "ERROR: Go not installed."
  exit 1
fi

sed -i "s|replace toolsmandi.com/proxy-security => .*|replace toolsmandi.com/proxy-security => ${PROXY_SEC}|" go.mod

echo "=== Build ==="
go mod tidy
go mod download
CGO_ENABLED=0 go build -ldflags="-s -w" -o "$BIN" .
chmod +x "$BIN"
file "$BIN"

strings "$BIN" | grep -q "$BUILD_TAG" && echo "OK: $BUILD_TAG in binary" || {
  echo "WARN: build tag $BUILD_TAG not found — upload latest automation_security.go"
}

echo "=== Install systemd ==="
if [ ! -f jungle-scout-amzpremium.service ]; then
  if [ -f "$SRC/jungle-scout-amzpremium.service" ]; then
    cp "$SRC/jungle-scout-amzpremium.service" .
  else
    cat > jungle-scout-amzpremium.service <<EOF
[Unit]
Description=Jungle Scout Go Proxy (amzpremiumsoftware.com)
After=network.target mysql.service

[Service]
Type=simple
User=root
WorkingDirectory=${DIR}
ExecStartPre=/bin/bash -c 'fuser -k ${PORT}/tcp 2>/dev/null || true; sleep 2'
ExecStart=${DIR}/${BIN}
Restart=on-failure
RestartSec=10
StartLimitIntervalSec=120
StartLimitBurst=5
StandardOutput=journal
StandardError=journal
SyslogIdentifier=jungle-amzpremium
LimitNOFILE=65536
KillMode=control-group
TimeoutStopSec=15

[Install]
WantedBy=multi-user.target
EOF
    echo "Wrote jungle-scout-amzpremium.service"
  fi
fi
cp jungle-scout-amzpremium.service /etc/systemd/system/${SERVICE}.service
systemctl daemon-reload
systemctl enable "$SERVICE"

echo "=== Final port check ==="
systemctl stop "$SERVICE" 2>/dev/null || true
kill_port_holders
ensure_port_free "pre-start"

systemctl start "$SERVICE"
sleep 3

if ! systemctl is-active --quiet "$SERVICE"; then
  echo "ERROR: $SERVICE failed to start"
  port_holders
  journalctl -u "$SERVICE" -n 30 --no-pager
  exit 1
fi

systemctl status "$SERVICE" --no-pager | head -12
curl -s -o /dev/null -w "localhost:${PORT}/ → HTTP %{http_code}\n" \
  "http://127.0.0.1:${PORT}/" \
  -H "User-Agent: Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/148.0.0.0 Safari/537.36" || true
curl -sI "http://127.0.0.1:${PORT}/" -H "User-Agent: Mozilla/5.0" | grep -i x-proxy-build || true
curl -s -o /dev/null -w "handshake: HTTP %{http_code}\n" \
  -X POST "http://127.0.0.1:${PORT}/api/auth-handshake" \
  -H "Content-Type: application/json" \
  -d '{"username":"test","secret":"amzpremiumsoftware_jungle_secret_xyz123"}' || true

echo ""
echo "aaPanel reverse proxy: jungle.amzpremiumsoftware.com → http://127.0.0.1:${PORT}"
echo "Panel: website_id=62 | secret=amzpremiumsoftware_jungle_secret_xyz123"
echo "Logs:  journalctl -u $SERVICE -f"
echo "Kill:  fuser -k ${PORT}/tcp && systemctl restart $SERVICE"
