#!/usr/bin/env bash
# Foreground process for one tool systemd unit. Usage: run-tool.sh <subdomain>
set -euo pipefail
SUB="${1:-}"
if [[ -z "${SUB}" ]]; then
  echo "usage: run-tool.sh <subdomain>" >&2
  exit 1
fi
BASE="/www/wwwroot/gt4rents.com"
ENV_FILE="${BASE}/_secrets/mysql.env"
LOG="${BASE}/${SUB}/app.log"
cd "${BASE}/${SUB}"
touch "${LOG}"
if [[ -f "${ENV_FILE}" ]]; then
  set -a
  # shellcheck disable=SC1090
  source "${ENV_FILE}"
  set +a
fi
export GT4RENTS_MYSQL_HOST="${GT4RENTS_MYSQL_HOST:-127.0.0.1}"
export GT4RENTS_MYSQL_PORT="${GT4RENTS_MYSQL_PORT:-3306}"
export GT4RENTS_MYSQL_USER="${GT4RENTS_MYSQL_USER:-gt4rents}"
export GT4RENTS_MYSQL_DB="${GT4RENTS_MYSQL_DB:-gt4rents}"
exec >>"${LOG}" 2>&1
exec ./app
