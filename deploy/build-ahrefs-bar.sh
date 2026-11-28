#!/usr/bin/env bash
# Same style as build-one.sh — build Ahrefs Bar API from _repo into runtime folder.
# Usage (on server):
#   cd /www/wwwroot/gt4rents.com/_repo
#   git pull origin main
#   source /www/wwwroot/gt4rents.com/_secrets/mysql.env
#   ./deploy/build-ahrefs-bar.sh
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
BASE="${BASE:-/www/wwwroot/gt4rents.com}"
OUTDIR="${BASE}/ahrefs-bar-api"
SRC="${ROOT}/ahrefs/server"
CACHE_DIR="${AHREFS_BAR_FILE_CACHE_DIR:-/var/lib/toolsmandi/ahrefs-cache}"
SECRETS="${BASE}/_secrets"
EXTRA_ENV="${SECRETS}/ahrefs-bar-api.env"

if [[ ! -d "${SRC}" ]]; then
  echo "Missing ${SRC} — git pull origin main pehle"
  exit 1
fi

mkdir -p "${OUTDIR}" "${SECRETS}"
mkdir -p "${CACHE_DIR}/urls" "${CACHE_DIR}/keywords" "${CACHE_DIR}/dict" "${CACHE_DIR}/meta"

echo "Building ahrefs/server → ${OUTDIR}/ahrefs-bar-api"
cd "${SRC}"
GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -o "${OUTDIR}/ahrefs-bar-api" .

install -m 755 "${ROOT}/ahrefs/deploy/run-ahrefs-bar-api.sh" "${OUTDIR}/run-ahrefs-bar-api.sh"

# Extension + templates (panel Download zip)
if [[ -d "${ROOT}/ahrefs/extension" ]]; then
  mkdir -p "${OUTDIR}/extension"
  rsync -a --delete --exclude '_metadata' "${ROOT}/ahrefs/extension/" "${OUTDIR}/extension/"
  echo "Synced extension/ → ${OUTDIR}/extension"
fi
if [[ -d "${ROOT}/ahrefs/templates" ]]; then
  mkdir -p "${OUTDIR}/templates"
  rsync -a "${ROOT}/ahrefs/templates/" "${OUTDIR}/templates/"
  echo "Synced templates/ → ${OUTDIR}/templates"
fi

# Env (create once; do not overwrite secrets)
if [[ ! -f "${EXTRA_ENV}" ]]; then
  install -m 600 "${ROOT}/ahrefs/deploy/ahrefs-bar-api.env.example" "${EXTRA_ENV}"
  echo "Wrote ${EXTRA_ENV}"
fi
ensure_env_line() {
  local key="$1" val="$2"
  if ! grep -q "^${key}=" "${EXTRA_ENV}" 2>/dev/null; then
    echo "${key}=${val}" >>"${EXTRA_ENV}"
  fi
}
ensure_env_line AHREFS_BAR_ADDR "127.0.0.1:8088"
ensure_env_line AHREFS_BAR_DEV_AUTH 0
ensure_env_line AHREFS_BAR_SEED_DEV_ACCOUNTS 0
ensure_env_line AHREFS_BAR_REQUIRE_FP 1
ensure_env_line AHREFS_BAR_FILE_CACHE 1
ensure_env_line AHREFS_BAR_FILE_CACHE_DIR "${CACHE_DIR}"
ensure_env_line AHREFS_BAR_FILE_CACHE_URL_GB 35
ensure_env_line AHREFS_BAR_FILE_CACHE_KW_GB 15
ensure_env_line AHREFS_BAR_FILE_CACHE_TTL_HOURS 72
ensure_env_line AHREFS_BAR_PUBLIC_URL "https://barapi.upays.in"
ensure_env_line AHREFS_BAR_EXT_DIR "${OUTDIR}/extension"
ensure_env_line AHREFS_BAR_TEMPLATE_DIR "${OUTDIR}/templates"

# Schema: ONLY ahrefs_bar_* (CREATE IF NOT EXISTS)
if [[ -f "${BASE}/_secrets/mysql.env" ]]; then
  set -a
  # shellcheck disable=SC1090
  source "${BASE}/_secrets/mysql.env"
  set +a
fi
SQL_FILE="${ROOT}/ahrefs/sql/001_ahrefs_bar.sql"
if [[ -f "${SQL_FILE}" && -n "${GT4RENTS_MYSQL_PASSWORD:-}" ]]; then
  echo "Applying ${SQL_FILE} (ahrefs_bar_* only)"
  mysql -h "${GT4RENTS_MYSQL_HOST:-127.0.0.1}" -P "${GT4RENTS_MYSQL_PORT:-3306}" \
    -u "${GT4RENTS_MYSQL_USER:-gt4rents}" -p"${GT4RENTS_MYSQL_PASSWORD}" \
    "${GT4RENTS_MYSQL_DB:-gt4rents}" < "${SQL_FILE}"
else
  echo "WARN: skip SQL (missing mysql.env password or ${SQL_FILE})"
fi

chown -R www:www "${OUTDIR}" 2>/dev/null || true
chown -R www:www "${CACHE_DIR}" 2>/dev/null || true

echo "OK: build → ${OUTDIR}/ahrefs-bar-api"
echo "Next: ./deploy/start-ahrefs-bar.sh"
