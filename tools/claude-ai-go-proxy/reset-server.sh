#!/bin/bash
# Server setup — run inside /www/wwwroot/1clkaccess.store/claude/
set -e

DIR="/www/wwwroot/1clkaccess.store/claude"
PORT="8799"

cd "$DIR"
echo "=== CLAUDE SERVER SETUP ==="
echo "Folder: $DIR | Port: $PORT"
echo ""

systemctl stop claude-recloud 2>/dev/null || true
fuser -k ${PORT}/tcp 2>/dev/null || true
sleep 1

echo "0" > cooldown.txt
chmod 644 cooldown.txt config.json cookie.txt 2>/dev/null || true

if [ -f config.server.json ]; then
  cp config.server.json config.json
  echo "Applied config.server.json → config.json"
fi

if [ -x ./claude-ai-go-proxy ]; then
  echo "Using existing binary: ./claude-ai-go-proxy"
elif command -v go &>/dev/null; then
  go mod download
  CGO_ENABLED=0 go build -ldflags="-s -w" -o claude-ai-go-proxy .
  chmod +x claude-ai-go-proxy
else
  echo "ERROR: No binary found and Go not installed."
  echo "Upload claude-ai-go-proxy (Linux amd64) or install Go on server."
  exit 1
fi

file claude-ai-go-proxy

cp claude-recloud.service /etc/systemd/system/claude-recloud.service
systemctl daemon-reload
systemctl enable claude-recloud
systemctl restart claude-recloud
sleep 2

systemctl status claude-recloud --no-pager | head -12
curl -s -o /dev/null -w "localhost:${PORT}/new → HTTP %{http_code}\n" \
  "http://127.0.0.1:${PORT}/new" \
  -H "User-Agent: Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36" || true

echo ""
echo "aaPanel reverse proxy: http://127.0.0.1:${PORT}"
echo "Test: https://claude.1clkaccess.store/new"
echo "Logs: journalctl -u claude-recloud -f"
