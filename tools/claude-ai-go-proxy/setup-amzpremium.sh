#!/bin/bash
# Same-server setup: claude.amzpremiumsoftware.com (port 8801 — 8800 is grok-recloud)
set -e

DIR="/www/wwwroot/amzpremiumsoftware.com/claude"
PARENT="/www/wwwroot/amzpremiumsoftware.com"
SRC="/www/wwwroot/1clkaccess.store/claude"
PORT="8801"
BIN="claude-ai-go-proxy"
SERVICE="claude-amzpremium"

find_proxy_security() {
  local p
  for p in \
    "/www/wwwroot/1clkaccess.store/proxy-security" \
    "/www/wwwroot/amzpremiumsoftware.com/proxy-security" \
    "/www/wwwroot/oneclickgo/proxy-security" \
    "/www/wwwroot/shared/proxy-security"; do
    if [ -d "$p" ]; then
      echo "$p"
      return 0
    fi
  done
  find /www/wwwroot -maxdepth 6 -type d -name "proxy-security" 2>/dev/null | head -1
}

echo "=== Claude: amzpremiumsoftware.com (port $PORT) ==="
mkdir -p "$DIR"
cd "$DIR"

# 1) Go source from your working install
if [ -f "$SRC/main.go" ]; then
  echo "Copying source from $SRC ..."
  cp "$SRC"/*.go "$DIR"/
  cp "$SRC/go.mod" "$SRC/go.sum" "$DIR"/
fi

# 2) proxy-security (shared — required for go build)
PROXY_SEC="$(find_proxy_security || true)"
if [ -z "$PROXY_SEC" ]; then
  echo ""
  echo "ERROR: proxy-security folder not found on server."
  echo "Run this ONCE from your Mac (upload shared module):"
  echo "  scp -r ~/Desktop/oneclickgo/proxy-security root@SERVER:/www/wwwroot/1clkaccess.store/"
  echo "Then re-run: ./setup-amzpremium.sh"
  exit 1
fi
echo "Using proxy-security: $PROXY_SEC"
if [ "$PROXY_SEC" != "$PARENT/proxy-security" ]; then
  rm -f "$PARENT/proxy-security"
  ln -sf "$PROXY_SEC" "$PARENT/proxy-security"
fi

# 3) Config
if [ ! -f config.amzpremium.json ]; then
  echo "ERROR: Upload config.amzpremium.json to $DIR"
  exit 1
fi
cp config.amzpremium.json config.json
python3 -c "
import json
c=json.load(open('config.json'))
c.setdefault('session_security',{}).setdefault('ott_ip_validation',{})['enabled']=False
json.dump(c,open('config.json','w'),indent=2)
"
echo "0" > cooldown.txt
touch cookie.txt

# 4) Build
systemctl stop "$SERVICE" 2>/dev/null || true
fuser -k ${PORT}/tcp 2>/dev/null || true
sleep 1

go mod download
CGO_ENABLED=0 go build -ldflags="-s -w" -o "$BIN" .
chmod +x "$BIN"

# 5) Systemd
cp claude-amzpremium.service /etc/systemd/system/${SERVICE}.service
systemctl daemon-reload
systemctl enable "$SERVICE"
systemctl restart "$SERVICE"
sleep 2

systemctl status "$SERVICE" --no-pager | head -10
curl -sI "http://127.0.0.1:${PORT}/new" | grep -i x-proxy-build || true
echo ""
echo "Done. aaPanel: claude.amzpremiumsoftware.com -> http://127.0.0.1:8801"
echo "Note: port 8800 is used by grok-recloud — do not share it with Claude."
