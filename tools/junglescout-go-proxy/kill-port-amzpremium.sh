#!/bin/bash
# Emergency: free port 8787 (amzpremium jungle)
set -e
PORT=8787
SERVICE=jungle-scout-amzpremium
DIR="/www/wwwroot/amzpremiumsoftware.com/jungle"

systemctl stop "$SERVICE" 2>/dev/null || true
systemctl reset-failed "$SERVICE" 2>/dev/null || true

pids=$(ss -tlnp 2>/dev/null | grep -E ":${PORT}([^0-9]|$)" | grep -oE 'pid=[0-9]+' | cut -d= -f2 | sort -u || true)
if [ -n "$pids" ]; then
  echo "Killing PIDs: $pids"
  kill -9 $pids 2>/dev/null || true
fi
fuser -k ${PORT}/tcp 2>/dev/null || true
pkill -9 -f "${DIR}/junglescout-go-proxy" 2>/dev/null || true
sleep 2

if ss -tlnp 2>/dev/null | grep -qE ":${PORT}([^0-9]|$)"; then
  echo "ERROR: port $PORT still in use"
  ss -tlnp | grep -E ":${PORT}([^0-9]|$)"
  exit 1
fi

echo "OK: port $PORT free. Start: systemctl start $SERVICE"
