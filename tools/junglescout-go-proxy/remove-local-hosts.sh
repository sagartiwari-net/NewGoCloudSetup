#!/bin/bash
set -e
if ! grep -q 'members\.junglescout\.com' /etc/hosts 2>/dev/null; then
  echo "✅ No members.junglescout.com entries in /etc/hosts"
  exit 0
fi
echo "Found members.junglescout.com in /etc/hosts — removing (sudo required)..."
sudo sed -i.bak '/members\.junglescout\.com/d' /etc/hosts
if grep -q 'members\.junglescout\.com' /etc/hosts 2>/dev/null; then
  echo "❌ Still present — edit manually: sudo nano /etc/hosts"
  exit 1
fi
sudo dscacheutil -flushcache 2>/dev/null || true
sudo killall -HUP mDNSResponder 2>/dev/null || true
echo "✅ Removed. Use: http://127.0.0.1:7852"
