#!/bin/bash
# Claude proxy deploy + verify — run on server as root
set -e
DIR="/www/wwwroot/1clkaccess.store/claude"
BIN="claude-ai-go-proxy"
cd "$DIR"

echo "=== 1) Apply production config ==="
if [ -f config.production.json ]; then
  cp config.production.json config.json
fi
python3 -c "
import json
c=json.load(open('config.json'))
c.setdefault('session_security',{}).setdefault('ott_ip_validation',{})['enabled']=False
json.dump(c,open('config.json','w'),indent=2)
print('config.json: production config applied, ott_ip_validation off')
"

echo "=== 2) Build (binary name must match systemd ExecStart: $BIN) ==="
go build -o "$BIN" .
chmod +x "$BIN"
# Remove stale wrong binary from old deploy scripts
rm -f claude-recloud 2>/dev/null || true

echo "=== 3) Verify binary build tag ==="
strings "$BIN" | grep -q 'claude-v23-screen' && echo "OK: claude-v23-screen in $BIN" || {
  echo "ERROR: upload main.go + security_accounts.go then rebuild"
  exit 1
}

echo "=== 4) Restart service ==="
systemctl restart claude-recloud
sleep 2
journalctl -u claude-recloud -n 6 --no-pager

echo "=== 5) Verify live header (must show v12, not v8) ==="
HDR=$(curl -sI https://claude.1clkaccess.store/new | grep -i x-proxy-build || true)
echo "$HDR"
echo "$HDR" | grep -q 'claude-v23-screen' && echo "OK: live site updated" || {
  echo "WARN: header still old — check nginx proxy_pass port 8799 and that claude-recloud.service ExecStart points to $DIR/$BIN"
}

echo "Done."
