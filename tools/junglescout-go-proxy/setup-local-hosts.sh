#!/bin/bash
set -e
ENTRY="127.0.0.1 members.junglescout.com"
if grep -qE '^127\.0\.0\.1[[:space:]]+members\.junglescout\.com(\s|$)' /etc/hosts 2>/dev/null; then
  echo "✅ members.junglescout.com already in /etc/hosts"
else
  echo "Adding: $ENTRY"
  echo "$ENTRY" | sudo tee -a /etc/hosts > /dev/null
fi
sudo dscacheutil -flushcache 2>/dev/null || true
sudo killall -HUP mDNSResponder 2>/dev/null || true
echo ""
echo "✅ Done. Open: http://members.junglescout.com:7853"
echo "   (API/search needs this — 127.0.0.1 alone blocks Datadome cookies)"
