#!/bin/bash
set -e
ENTRY="127.0.0.1 members.erank.com"
if grep -qE '^127\.0\.0\.1[[:space:]]+members\.erank\.com(\s|$)' /etc/hosts 2>/dev/null; then
  echo "✅ members.erank.com already in /etc/hosts"
else
  echo "Adding: $ENTRY"
  echo "$ENTRY" | sudo tee -a /etc/hosts > /dev/null
fi
sudo dscacheutil -flushcache 2>/dev/null || true
sudo killall -HUP mDNSResponder 2>/dev/null || true
echo ""
echo "✅ Done. Open: http://members.erank.com:7853"
echo "   (API calls use same origin — /etc/hosts maps members.erank.com → 127.0.0.1)"
