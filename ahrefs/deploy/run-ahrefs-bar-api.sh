#!/usr/bin/env bash
# Foreground process for systemd. Loads MySQL secrets like panel-api.
set -euo pipefail
BASE="/www/wwwroot/gt4rents.com"
DIR="${BASE}/ahrefs-bar-api"
BIN="${DIR}/ahrefs-bar-api"
LOG="${DIR}/ahrefs-bar-api.log"
ENV_FILE="${BASE}/_secrets/mysql.env"
EXTRA_ENV="${BASE}/_secrets/ahrefs-bar-api.env"

cd "${DIR}"
touch "${LOG}"

if [[ -f "${ENV_FILE}" ]]; then
  set -a
  # shellcheck disable=SC1090
  source "${ENV_FILE}"
  set +a
fi
if [[ -f "${EXTRA_ENV}" ]]; then
  set -a
  # shellcheck disable=SC1090
  source "${EXTRA_ENV}"
  set +a
fi

export AHREFS_BAR_ADDR="${AHREFS_BAR_ADDR:-127.0.0.1:8088}"
export AHREFS_BAR_TZ="${AHREFS_BAR_TZ:-Asia/Kolkata}"
export AHREFS_BAR_DEV_AUTH="${AHREFS_BAR_DEV_AUTH:-0}"
export AHREFS_BAR_MOCK_UPSTREAM="${AHREFS_BAR_MOCK_UPSTREAM:-0}"
export AHREFS_BAR_REQUIRE_FP="${AHREFS_BAR_REQUIRE_FP:-1}"
export AHREFS_BAR_SEED_DEV_ACCOUNTS="${AHREFS_BAR_SEED_DEV_ACCOUNTS:-0}"
export AHREFS_BAR_VERIFY_RATE="${AHREFS_BAR_VERIFY_RATE:-60}"

if [[ -z "${AHREFS_BAR_DSN:-}" && -z "${GT4RENTS_MYSQL_PASSWORD:-}" ]]; then
  echo "ahrefs-bar-api refused: MySQL password / DSN missing" >>"${LOG}"
  exit 1
fi

exec >>"${LOG}" 2>&1
exec "${BIN}"
