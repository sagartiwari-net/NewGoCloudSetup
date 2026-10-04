#!/bin/bash
set -e
sudo sed -i '' '/127\.0\.0\.1[[:space:]]\+members\.erank\.com/d' /etc/hosts 2>/dev/null || \
  sudo sed -i '/127\.0\.0\.1[[:space:]]\+members\.erank\.com/d' /etc/hosts
sudo dscacheutil -flushcache 2>/dev/null || true
sudo killall -HUP mDNSResponder 2>/dev/null || true
echo "✅ Removed members.erank.com from /etc/hosts"
