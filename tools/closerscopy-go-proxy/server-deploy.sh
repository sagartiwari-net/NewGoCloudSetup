#!/bin/bash
# ClosersCopy production — run on server as root
set -e

APP_DIR="/www/wwwroot/1clkaccess.store/closerscopy"
SEC_LIB="/www/wwwroot/toolsmandi.com/proxy-security"
PORT="6012"

echo "=== ClosersCopy deploy ==="
cd "$APP_DIR"

cp config.production.json config.json
sed -i "s|replace toolsmandi.com/proxy-security => .*|replace toolsmandi.com/proxy-security => ${SEC_LIB}|" go.mod
rm -f security_score.go security_log.go

go mod tidy
go build -o closerscopy-proxy .
chmod +x closerscopy-proxy

cat > /etc/systemd/system/closerscopy-proxy.service << 'EOF'
[Unit]
Description=ClosersCopy Go Proxy (ToolsMandi)
After=network.target mysql.service

[Service]
Type=simple
User=root
WorkingDirectory=/www/wwwroot/1clkaccess.store/closerscopy
ExecStart=/www/wwwroot/1clkaccess.store/closerscopy/closerscopy-proxy
Restart=always
RestartSec=5
StandardOutput=journal
StandardError=journal
SyslogIdentifier=closerscopy-proxy
LimitNOFILE=65536

[Install]
WantedBy=multi-user.target
EOF

systemctl daemon-reload
systemctl enable closerscopy-proxy
systemctl restart closerscopy-proxy
sleep 2

systemctl status closerscopy-proxy --no-pager -l || true
journalctl -u closerscopy-proxy -n 30 --no-pager
ss -tlnp | grep ":${PORT} " || echo "WARN: not listening on :${PORT}"
curl -sI "http://127.0.0.1:${PORT}/" | head -5
