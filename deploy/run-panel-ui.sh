#!/usr/bin/env bash
# Foreground process for systemd. Do not nohup this.
set -euo pipefail
cd /www/wwwroot/gt4rents.com/_repo/panel/update-panel
LOG="/www/wwwroot/gt4rents.com/_repo/panel/update-panel/panel-ui.log"
touch "${LOG}"
if [[ -x /usr/bin/npm ]]; then
  NPM=/usr/bin/npm
elif [[ -x /usr/local/bin/npm ]]; then
  NPM=/usr/local/bin/npm
else
  NPM="$(command -v npm || true)"
fi
if [[ -z "${NPM}" ]]; then
  echo "npm not found" >>"${LOG}"
  exit 1
fi
exec >>"${LOG}" 2>&1
exec "${NPM}" run start
