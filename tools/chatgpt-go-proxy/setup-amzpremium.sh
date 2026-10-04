#!/bin/bash
# ChatGPT for chat.amzpremiumsoftware.com — run on server as root
# Usage: bash setup-amzpremium.sh
set -e

APP_DIR="/www/wwwroot/amzpremiumsoftware.com/chat"
SEC_LIB="/www/wwwroot/toolsmandi.com/proxy-security"
PORT="6042"
SERVICE="chatgpt-amz-proxy"

echo "=== ChatGPT AMZpremium deploy ==="
cd "$APP_DIR"

BUILD_TAG=$(grep -oE 'proxyBuildTag = "[^"]+"' main.go 2>/dev/null | sed 's/.*"\(.*\)"/\1/' || true)
if [ -z "$BUILD_TAG" ]; then
  echo "WARN: could not read proxyBuildTag from main.go"
else
  echo "Source build tag: $BUILD_TAG"
  if [[ "$BUILD_TAG" != chatgpt-v21* ]]; then
    echo "ERROR: main.go is $BUILD_TAG — expect chatgpt-v21* (git pull / clone latest)"
    exit 1
  fi
fi

for f in main.go security_bridge.go ip_blocklist.go go.mod go.sum; do
  if [ ! -f "$f" ]; then
    echo "ERROR: missing $f in $APP_DIR"
    exit 1
  fi
done

# Prefer amz template; fall back to production template with host rewrite
if [ -f config.amzpremium.json ]; then
  TEMPLATE="config.amzpremium.json"
elif [ -f config.production.json ]; then
  TEMPLATE="config.production.json"
else
  echo "ERROR: missing config.amzpremium.json / config.production.json"
  exit 1
fi

if [ -f config.json ]; then
  cp config.json config.json.bak
  echo "OK: backed up existing config.json → config.json.bak"
fi
cp "$TEMPLATE" config.json

python3 << 'PY'
import json, os, sys

cfg_path = "config.json"
with open(cfg_path, encoding="utf-8") as f:
    cfg = json.load(f)

# Force amz domain + port even if template was 1clk production
cfg["port"] = "6042"
cfg["public_host"] = "chat.amzpremiumsoftware.com"
cfg["public_scheme"] = "https"
cfg["member_area_url"] = cfg.get("member_area_url") or "https://amzpremiumsoftware.com/"
ss = cfg.setdefault("session_security", {})
ss.setdefault("domain_check", {})["expected_host"] = "chat.amzpremiumsoftware.com"
ss.setdefault("headers", {})["cors_allow_origin"] = "https://chat.amzpremiumsoftware.com"

def save():
    with open(cfg_path, "w", encoding="utf-8") as f:
        json.dump(cfg, f, indent=2)
        f.write("\n")

if cfg.get("mysql_password") and cfg.get("mysql_password") != "CHANGE_ME":
    save()
    print("OK: config ready (mysql_password already set)")
    sys.exit(0)

sources = []
if os.path.exists("config.json.bak"):
    sources.append("config.json.bak")
for sibling in (
    "/www/wwwroot/1clkaccess.store/chat/config.json",
    "/www/wwwroot/1clkaccess.store/buzzsumo/config.json",
    "/www/wwwroot/1clkaccess.store/airbrush/config.json",
    "/www/wwwroot/toolsmandi.com/ctrl/config.json",
):
    if os.path.exists(sibling):
        sources.append(sibling)

for src in sources:
    try:
        with open(src, encoding="utf-8") as f:
            s = json.load(f)
        pwd = (s.get("mysql_password") or "").strip()
        if pwd and pwd != "CHANGE_ME":
            cfg["mysql_password"] = pwd
            if s.get("mysql_user"):
                cfg["mysql_user"] = s["mysql_user"]
            if s.get("mysql_db"):
                cfg["mysql_db"] = s["mysql_db"]
            save()
            print(f"OK: mysql credentials restored from {src}")
            sys.exit(0)
    except Exception as e:
        print(f"WARN: could not read {src}: {e}")

save()
print("")
print("ERROR: mysql_password is still CHANGE_ME — fix config.json then restart")
sys.exit(1)
PY

echo "OK: config.json ready for chat.amzpremiumsoftware.com :$PORT"

sed -i "s|replace toolsmandi.com/proxy-security => .*|replace toolsmandi.com/proxy-security => ${SEC_LIB}|" go.mod

if [ ! -f "$SEC_LIB/go.mod" ]; then
  echo "ERROR: proxy-security not found at $SEC_LIB"
  exit 1
fi

# free port if leftover process
if ss -tlnp | grep -q ":${PORT} "; then
  echo "Stopping whatever is on :$PORT ..."
  fuser -k "${PORT}/tcp" 2>/dev/null || true
  sleep 1
fi

go mod tidy
go build -o chatgpt-proxy .
chmod +x chatgpt-proxy
echo "OK: binary built $(ls -lh chatgpt-proxy | awk '{print $5,$9}')"

cat > /etc/systemd/system/${SERVICE}.service << EOF
[Unit]
Description=ChatGPT Go Proxy (AmzPremium)
After=network.target mysql.service

[Service]
Type=simple
User=root
WorkingDirectory=${APP_DIR}
ExecStart=${APP_DIR}/chatgpt-proxy
Restart=always
RestartSec=5
StandardOutput=journal
StandardError=journal
SyslogIdentifier=${SERVICE}
LimitNOFILE=65536

[Install]
WantedBy=multi-user.target
EOF

systemctl daemon-reload
systemctl enable "$SERVICE"
systemctl restart "$SERVICE"
sleep 2

echo ""
echo "=== Service status ==="
systemctl status "$SERVICE" --no-pager -l || true

echo ""
echo "=== Last 25 log lines ==="
journalctl -u "$SERVICE" -n 25 --no-pager

echo ""
echo "=== Port $PORT check ==="
if ss -tlnp | grep -q ":${PORT} "; then
  echo "OK: listening on :$PORT"
  curl -sI "http://127.0.0.1:${PORT}/" | grep -iE 'x-proxy-build|HTTP/' || true
else
  echo "FAIL: nothing listening on :$PORT"
fi

echo ""
echo "=== Done ==="
echo "Nginx: chat.amzpremiumsoftware.com → http://127.0.0.1:${PORT}"
echo "DB: ahrefs_websites.domain must be chat.amzpremiumsoftware.com (secret_key match config)"
echo "Test: curl -sI https://chat.amzpremiumsoftware.com/ | grep -i x-proxy-build"
