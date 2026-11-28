#!/usr/bin/env bash
# Install / update ONLY ahrefs-bar-api.
# - Does NOT restart panel-api, nginx, or any gt4-* tool proxy
# - Schema is CREATE IF NOT EXISTS ahrefs_bar_* only (no DROP, no tool tables)
# - Runs mysqldump backup first unless SKIP_BACKUP=1
set -euo pipefail

BASE="${BASE:-/www/wwwroot/gt4rents.com}"
DIR="${BASE}/ahrefs-bar-api"
SECRETS="${BASE}/_secrets"
MYSQL_ENV="${SECRETS}/mysql.env"
EXTRA_ENV="${SECRETS}/ahrefs-bar-api.env"
CACHE_DIR="${AHREFS_BAR_FILE_CACHE_DIR:-/var/lib/toolsmandi/ahrefs-cache}"
HERE="$(cd "$(dirname "$0")" && pwd)"

if [[ "$(id -u)" -ne 0 ]]; then
  echo "Run as root"
  exit 1
fi
if [[ ! -f "${MYSQL_ENV}" ]]; then
  echo "Missing ${MYSQL_ENV}"
  exit 1
fi

# ---------- 1) Backup (existing DB + service inventory) ----------
if [[ "${SKIP_BACKUP:-0}" != "1" ]]; then
  echo "== Pre-install backup"
  bash "${HERE}/backup-before-ahrefs-bar.sh"
else
  echo "SKIP_BACKUP=1 — not dumping MySQL"
fi

# ---------- 2) Binary + run script (isolated directory) ----------
mkdir -p "${DIR}" "${SECRETS}"
install -m 755 "${HERE}/run-ahrefs-bar-api.sh" "${DIR}/run-ahrefs-bar-api.sh"

BIN_SRC=""
if [[ -f "${HERE}/ahrefs-bar-api.linux-amd64" ]]; then
  BIN_SRC="${HERE}/ahrefs-bar-api.linux-amd64"
elif [[ -f "${HERE}/ahrefs-bar-api" ]]; then
  BIN_SRC="${HERE}/ahrefs-bar-api"
else
  echo "Missing linux binary in ${HERE}"
  exit 1
fi
install -m 755 "${BIN_SRC}" "${DIR}/ahrefs-bar-api"

if [[ ! -f "${EXTRA_ENV}" ]]; then
  install -m 600 "${HERE}/ahrefs-bar-api.env.example" "${EXTRA_ENV}"
  echo "Wrote ${EXTRA_ENV}"
fi

# File cache (disk corpus — NOT MySQL). Separate from tool cdn-cache dirs.
mkdir -p "${CACHE_DIR}/urls" "${CACHE_DIR}/keywords" "${CACHE_DIR}/dict" "${CACHE_DIR}/meta"
chown -R www:www "${CACHE_DIR}" 2>/dev/null || true

# Ensure file-cache env present (idempotent)
ensure_env_line() {
  local key="$1" val="$2"
  if ! grep -q "^${key}=" "${EXTRA_ENV}" 2>/dev/null; then
    echo "${key}=${val}" >>"${EXTRA_ENV}"
  fi
}
ensure_env_line AHREFS_BAR_FILE_CACHE 1
ensure_env_line AHREFS_BAR_FILE_CACHE_DIR "${CACHE_DIR}"
ensure_env_line AHREFS_BAR_FILE_CACHE_URL_GB 35
ensure_env_line AHREFS_BAR_FILE_CACHE_KW_GB 15
ensure_env_line AHREFS_BAR_FILE_CACHE_TTL_HOURS 72
ensure_env_line AHREFS_BAR_ADDR "127.0.0.1:8088"
ensure_env_line AHREFS_BAR_DEV_AUTH 0
ensure_env_line AHREFS_BAR_SEED_DEV_ACCOUNTS 0

# ---------- 3) Schema: ONLY ahrefs_bar_* (CREATE IF NOT EXISTS) ----------
set -a
# shellcheck disable=SC1090
source "${MYSQL_ENV}"
set +a
SQL_FILE="${SQL_FILE:-${HERE}/../sql/001_ahrefs_bar.sql}"
if [[ ! -f "${SQL_FILE}" ]]; then
  SQL_FILE="${HERE}/001_ahrefs_bar.sql"
fi
if [[ -f "${SQL_FILE}" ]]; then
  # Safety: refuse destructive / non-ahrefs DDL
  if grep -Eiq 'drop[[:space:]]+(table|database)|truncate[[:space:]]+table|alter[[:space:]]+table' "${SQL_FILE}"; then
    echo "REFUSING SQL: DROP/TRUNCATE/ALTER not allowed in install (ahrefs_bar uses CREATE IF NOT EXISTS only)"
    exit 1
  fi
  if grep -Eiq 'create[[:space:]]+table' "${SQL_FILE}" && ! grep -Eiq 'ahrefs_bar_' "${SQL_FILE}"; then
    echo "REFUSING SQL: CREATE TABLE without ahrefs_bar_ prefix"
    exit 1
  fi
  echo "Applying isolated schema ${SQL_FILE} (ahrefs_bar_* only)"
  mysql -h "${GT4RENTS_MYSQL_HOST:-127.0.0.1}" -P "${GT4RENTS_MYSQL_PORT:-3306}" \
    -u "${GT4RENTS_MYSQL_USER:-gt4rents}" -p"${GT4RENTS_MYSQL_PASSWORD}" \
    "${GT4RENTS_MYSQL_DB:-gt4rents}" < "${SQL_FILE}"
else
  echo "WARN: schema SQL not found — ensure ahrefs_bar_* tables exist"
fi

# ---------- 4) Extension / templates for panel Download (files only) ----------
if [[ -d "${HERE}/../extension" ]]; then
  mkdir -p "${DIR}/extension"
  rsync -a --delete --exclude '_metadata' "${HERE}/../extension/" "${DIR}/extension/"
elif [[ -d "${HERE}/extension" ]]; then
  mkdir -p "${DIR}/extension"
  rsync -a --delete --exclude '_metadata' "${HERE}/extension/" "${DIR}/extension/"
fi
if [[ -d "${HERE}/../templates" ]]; then
  mkdir -p "${DIR}/templates"
  rsync -a "${HERE}/../templates/" "${DIR}/templates/"
elif [[ -d "${HERE}/templates" ]]; then
  mkdir -p "${DIR}/templates"
  rsync -a "${HERE}/templates/" "${DIR}/templates/"
fi

if ! grep -q 'AHREFS_BAR_PUBLIC_URL' "${EXTRA_ENV}" 2>/dev/null; then
  {
    echo ""
    echo "AHREFS_BAR_PUBLIC_URL=https://barapi.upays.in"
    echo "AHREFS_BAR_EXT_DIR=${DIR}/extension"
    echo "AHREFS_BAR_TEMPLATE_DIR=${DIR}/templates"
  } >>"${EXTRA_ENV}"
fi

chown -R www:www "${DIR}" 2>/dev/null || true

# ---------- 5) systemd: ONLY ahrefs-bar-api (never gt4-* / panel) ----------
install -m 644 "${HERE}/ahrefs-bar-api.service" /etc/systemd/system/ahrefs-bar-api.service
systemctl daemon-reload
systemctl enable ahrefs-bar-api.service
systemctl restart ahrefs-bar-api.service
sleep 1
systemctl --no-pager --lines=25 status ahrefs-bar-api.service || true

echo ""
echo "=== DONE (isolated) ==="
echo "Touched: ${DIR}, ahrefs_bar_* tables, ${CACHE_DIR}, systemd ahrefs-bar-api"
echo "NOT touched: gt4-* tools, panel-api, nginx, payment-hub"
echo ""
echo "Smoke:  curl -s http://127.0.0.1:8088/v1/portals"
echo "Nginx (optional later): barapi.upays.in → 127.0.0.1:8088"
echo "Panel UI merge: separate step (restart gt4-panel-api only when ready)"
