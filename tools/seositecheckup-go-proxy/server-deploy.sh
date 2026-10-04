#!/bin/bash
# SEO Site Checkup production — run on server as root
# Usage: bash server-deploy.sh
set -e

APP_DIR="/www/wwwroot/1clkaccess.store/seosite"
SEC_LIB="/www/wwwroot/toolsmandi.com/proxy-security"
PORT="6031"

echo "=== SEO Site Checkup deploy ==="
cd "$APP_DIR"

# 1) Production config
if [ ! -f config.production.json ]; then
  echo "ERROR: config.production.json missing in $APP_DIR"
  exit 1
fi
cp config.production.json config.json
echo "OK: config.json from production template"
echo "    (If mysql_password is still CHANGE_ME, edit: nano config.json)"

# 2) go.mod replace path
sed -i "s|replace toolsmandi.com/proxy-security => .*|replace toolsmandi.com/proxy-security => ${SEC_LIB}|" go.mod

# 3) Clean old duplicate security files if uploaded by mistake
rm -f security_score.go security_log.go

# 4) Build
if [ ! -f "$SEC_LIB/go.mod" ]; then
  echo "ERROR: proxy-security not found at $SEC_LIB"
  exit 1
fi
go mod tidy
go build -o seositecheckup-proxy .
chmod +x seositecheckup-proxy
echo "OK: binary built $(ls -lh seositecheckup-proxy)"

# 5) systemd service
cat > /etc/systemd/system/seositecheckup-proxy.service << 'EOF'
[Unit]
Description=SEO Site Checkup Go Proxy (ToolsMandi)
After=network.target mysql.service

[Service]
Type=simple
User=root
WorkingDirectory=/www/wwwroot/1clkaccess.store/seosite
ExecStart=/www/wwwroot/1clkaccess.store/seosite/seositecheckup-proxy
Restart=always
RestartSec=5
StandardOutput=journal
StandardError=journal
SyslogIdentifier=seositecheckup-proxy
LimitNOFILE=65536

[Install]
WantedBy=multi-user.target
EOF

systemctl daemon-reload
systemctl enable seositecheckup-proxy
systemctl restart seositecheckup-proxy
sleep 2

echo ""
echo "=== Service status ==="
systemctl status seositecheckup-proxy --no-pager -l || true

echo ""
echo "=== Last 40 log lines ==="
journalctl -u seositecheckup-proxy -n 40 --no-pager

echo ""
echo "=== Port $PORT check ==="
if ss -tlnp | grep -q ":${PORT} "; then
  echo "OK: listening on :$PORT"
  curl -sI "http://127.0.0.1:${PORT}/" | head -5
else
  echo "FAIL: nothing listening on :$PORT"
  echo "Try manual run to see error:"
  echo "  cd $APP_DIR && ./seositecheckup-proxy"
fi

echo ""
echo "=== Done ==="
echo "Nginx reverse proxy must point to: http://127.0.0.1:${PORT}"
echo "Test site: https://seosite.1clkaccess.store"
