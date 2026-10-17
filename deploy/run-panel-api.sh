#!/usr/bin/env bash
# Foreground process for systemd. Do not nohup this.
set -euo pipefail
BIN="/www/wwwroot/gt4rents.com/panel/panel-api"
LOG="/www/wwwroot/gt4rents.com/panel/panel-api.log"
cd /www/wwwroot/gt4rents.com/panel
touch "${LOG}"
exec >>"${LOG}" 2>&1
exec "${BIN}"
