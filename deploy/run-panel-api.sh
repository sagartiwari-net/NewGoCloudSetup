#!/usr/bin/env bash
# Foreground process for systemd. Loads the same secret file as start-panel-api.sh.
# An empty password must not fall through to sqlite.
set -euo pipefail
BASE="/www/wwwroot/gt4rents.com"
BIN="${BASE}/panel/panel-api"
LOG="${BASE}/panel/panel-api.log"
ENV_FILE="${BASE}/_secrets/mysql.env"
cd "${BASE}/panel"
touch "${LOG}"
if [[ -f "${ENV_FILE}" ]]; then
  unset GT4RENTS_AUTOMATION_KEY
  set -a
  # shellcheck disable=SC1090
  source "${ENV_FILE}"
  set +a
fi
export PANEL_API_ADDR="${PANEL_API_ADDR:-127.0.0.1:18090}"
export GT4RENTS_MYSQL_HOST="${GT4RENTS_MYSQL_HOST:-127.0.0.1}"
export GT4RENTS_MYSQL_PORT="${GT4RENTS_MYSQL_PORT:-3306}"
export GT4RENTS_MYSQL_USER="${GT4RENTS_MYSQL_USER:-gt4rents}"
export GT4RENTS_MYSQL_DB="${GT4RENTS_MYSQL_DB:-gt4rents}"
if [[ -z "${GT4RENTS_MYSQL_PASSWORD:-}" ]]; then
  echo "panel-api refused to start: MySQL password was not loaded" >>"${LOG}"
  exit 1
fi
exec >>"${LOG}" 2>&1
exec "${BIN}"
