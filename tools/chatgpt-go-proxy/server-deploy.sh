#!/bin/bash
# ChatGPT production — run on server as root
# Usage: bash server-deploy.sh
set -e

APP_DIR="/www/wwwroot/1clkaccess.store/chat"
SEC_LIB="/www/wwwroot/toolsmandi.com/proxy-security"
PORT="6041"

echo "=== ChatGPT proxy deploy ==="
cd "$APP_DIR"

# Verify source has expected build tag (upload main.go from dev machine if this fails)
BUILD_TAG=$(grep -oE 'proxyBuildTag = "[^"]+"' main.go 2>/dev/null | sed 's/.*"\(.*\)"/\1/' || true)
if [ -z "$BUILD_TAG" ]; then
  echo "WARN: could not read proxyBuildTag from main.go"
else
  echo "Source build tag: $BUILD_TAG"
  if [[ "$BUILD_TAG" != chatgpt-v21* ]]; then
    echo "ERROR: main.go is $BUILD_TAG — upload latest main.go from your Mac first (expect chatgpt-v21*)"
    exit 1
  fi
fi

# Required source files
for f in main.go security_bridge.go ip_blocklist.go go.mod go.sum; do
  if [ ! -f "$f" ]; then
    echo "ERROR: missing $f in $APP_DIR"
    exit 1
  fi
done

# 1) Production config — merge template but KEEP existing MySQL password
if [ ! -f config.production.json ]; then
  echo "ERROR: config.production.json missing in $APP_DIR"
  exit 1
fi
if [ -f config.json ]; then
  cp config.json config.json.bak
  echo "OK: backed up existing config.json → config.json.bak"
fi
cp config.production.json config.json

python3 << 'PY'
import json, os, sys

cfg_path = "config.json"
with open(cfg_path, encoding="utf-8") as f:
    cfg = json.load(f)

if cfg.get("mysql_password") and cfg.get("mysql_password") != "CHANGE_ME":
    print("OK: config.json from production template (mysql_password already set)")
    sys.exit(0)

sources = []
if os.path.exists("config.json.bak"):
    sources.append("config.json.bak")
for sibling in (
    "/www/wwwroot/1clkaccess.store/buzzsumo/config.json",
    "/www/wwwroot/1clkaccess.store/airbrush/config.json",
    "/www/wwwroot/1clkaccess.store/seosite/config.json",
    "/www/wwwroot/1clkaccess.store/closerscopy/config.json",
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
            with open(cfg_path, "w", encoding="utf-8") as f:
                json.dump(cfg, f, indent=2)
                f.write("\n")
            print(f"OK: mysql_password restored from {src}")
            sys.exit(0)
    except Exception as e:
        print(f"WARN: could not read {src}: {e}")

print("")
print("ERROR: mysql_password is still CHANGE_ME")
print("  Fix: copy from a working proxy on this server, e.g.:")
print("    grep mysql_password /www/wwwroot/1clkaccess.store/buzzsumo/config.json")
print("  Then: nano config.json  → set mysql_password  → systemctl restart chatgpt-proxy")
sys.exit(1)
PY

echo "OK: config.json ready"

# 2) go.mod replace path for proxy-security
sed -i "s|replace toolsmandi.com/proxy-security => .*|replace toolsmandi.com/proxy-security => ${SEC_LIB}|" go.mod

# 3) Build
if [ ! -f "$SEC_LIB/go.mod" ]; then
  echo "ERROR: proxy-security not found at $SEC_LIB"
  exit 1
fi
go mod tidy
go build -o chatgpt-proxy .
chmod +x chatgpt-proxy
echo "OK: binary built $(ls -lh chatgpt-proxy)"

# 4) systemd service
cat > /etc/systemd/system/chatgpt-proxy.service << 'EOF'
[Unit]
Description=ChatGPT Go Proxy (ToolsMandi)
After=network.target mysql.service

[Service]
Type=simple
User=root
WorkingDirectory=/www/wwwroot/1clkaccess.store/chat
ExecStart=/www/wwwroot/1clkaccess.store/chat/chatgpt-proxy
Restart=always
RestartSec=5
StandardOutput=journal
StandardError=journal
SyslogIdentifier=chatgpt-proxy
LimitNOFILE=65536

[Install]
WantedBy=multi-user.target
EOF

systemctl daemon-reload
systemctl enable chatgpt-proxy
systemctl restart chatgpt-proxy
sleep 2

echo ""
echo "=== Service status ==="
systemctl status chatgpt-proxy --no-pager -l || true

echo ""
echo "=== Last 30 log lines ==="
journalctl -u chatgpt-proxy -n 30 --no-pager

echo ""
echo "=== Port $PORT check ==="
if ss -tlnp | grep -q ":${PORT} "; then
  echo "OK: listening on :$PORT"
  HDR=$(curl -sI "http://127.0.0.1:${PORT}/" | grep -i x-proxy-build || true)
  echo "$HDR"
  if echo "$HDR" | grep -qv "chatgpt-v21"; then
    echo "WARN: binary build tag is not chatgpt-v21* — old main.go may still be on disk"
  fi
else
  echo "FAIL: nothing listening on :$PORT"
  echo "Manual debug: cd $APP_DIR && ./chatgpt-proxy"
fi

echo ""
echo "=== Database check ==="
if journalctl -u chatgpt-proxy -n 15 --no-pager | grep -q "Connected to MySQL successfully"; then
  echo "OK: MySQL connected"
elif journalctl -u chatgpt-proxy -n 15 --no-pager | grep -q "Database not reachable"; then
  echo "FAIL: MySQL not connected — auth-handshake will return 500"
  echo "  grep mysql_password config.json"
  echo "  Copy password from: grep mysql_password /www/wwwroot/1clkaccess.store/buzzsumo/config.json"
  echo "  nano config.json && systemctl restart chatgpt-proxy"
else
  echo "WARN: could not confirm DB status — check: journalctl -u chatgpt-proxy -n 20"
fi

echo ""
echo "=== Done ==="
echo "Nginx must reverse-proxy chat.1clkaccess.store -> http://127.0.0.1:${PORT}"
echo "Test: https://chat.1clkaccess.store"
echo "Member area: route_tool.php?tool=chatgpt (needs MySQL connected)"
