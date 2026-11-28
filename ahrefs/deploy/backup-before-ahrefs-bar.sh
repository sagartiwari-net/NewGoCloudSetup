#!/usr/bin/env bash
# Safe pre-install backup. Does NOT stop tools, panel, or nginx.
# Only dumps MySQL (full gt4rents + ahrefs_bar_* only) and notes live services.
set -euo pipefail

BASE="${BASE:-/www/wwwroot/gt4rents.com}"
SECRETS="${BASE}/_secrets"
MYSQL_ENV="${SECRETS}/mysql.env"
STAMP="$(date +%Y%m%d-%H%M%S)"
OUT="${BACKUP_DIR:-${BASE}/_backups/ahrefs-bar-${STAMP}}"

if [[ "$(id -u)" -ne 0 ]]; then
  echo "Run as root"
  exit 1
fi
if [[ ! -f "${MYSQL_ENV}" ]]; then
  echo "Missing ${MYSQL_ENV}"
  exit 1
fi

set -a
# shellcheck disable=SC1090
source "${MYSQL_ENV}"
set +a

HOST="${GT4RENTS_MYSQL_HOST:-127.0.0.1}"
PORT="${GT4RENTS_MYSQL_PORT:-3306}"
USER="${GT4RENTS_MYSQL_USER:-gt4rents}"
PASS="${GT4RENTS_MYSQL_PASSWORD:?GT4RENTS_MYSQL_PASSWORD required}"
DB="${GT4RENTS_MYSQL_DB:-gt4rents}"

mkdir -p "${OUT}"
chmod 700 "${OUT}"

echo "== Backup dir: ${OUT}"

# 1) Full DB dump (restore point if anything goes wrong)
echo "Dumping full database ${DB}…"
mysqldump -h "${HOST}" -P "${PORT}" -u "${USER}" -p"${PASS}" \
  --single-transaction --routines --triggers --quick \
  "${DB}" | gzip -c > "${OUT}/gt4rents-full.sql.gz"

# 2) Only ahrefs_bar_* tables (isolated slice — empty OK on first install)
TABLES="$(mysql -h "${HOST}" -P "${PORT}" -u "${USER}" -p"${PASS}" -N -e \
  "SELECT table_name FROM information_schema.tables
   WHERE table_schema='${DB}' AND table_name LIKE 'ahrefs\\_bar\\_%'
   ORDER BY table_name;" 2>/dev/null || true)"
if [[ -n "${TABLES}" ]]; then
  echo "Dumping ahrefs_bar_* only…"
  # shellcheck disable=SC2086
  mysqldump -h "${HOST}" -P "${PORT}" -u "${USER}" -p"${PASS}" \
    --single-transaction --quick "${DB}" ${TABLES} \
    | gzip -c > "${OUT}/ahrefs_bar-only.sql.gz"
else
  echo "No ahrefs_bar_* tables yet (first install) — skipping slice dump"
  echo "(none)" > "${OUT}/ahrefs_bar-tables.txt"
fi
if [[ -n "${TABLES}" ]]; then
  echo "${TABLES}" > "${OUT}/ahrefs_bar-tables.txt"
fi

# 3) operators column snapshot (panel may add ahrefs_bar_enabled later — additive only)
mysql -h "${HOST}" -P "${PORT}" -u "${USER}" -p"${PASS}" -N -e \
  "SHOW COLUMNS FROM ${DB}.operators LIKE 'ahrefs_bar%';" \
  > "${OUT}/operators-ahrefs-columns.txt" 2>/dev/null || true

# 4) Inventory of running gt4 / panel (we must not disturb these)
{
  echo "stamp=${STAMP}"
  echo "host=$(hostname)"
  date -u +"utc=%Y-%m-%dT%H:%M:%SZ"
  echo "--- systemctl ---"
  systemctl list-units 'gt4-*' 'ahrefs-bar-api*' --type=service --no-pager 2>/dev/null || true
  echo "--- listening tool ports (sample) ---"
  ss -lntp 2>/dev/null | grep -E ':(45|46|47|48|49|50|51|52|53)[0-9]{2}\b|:18090\b|:8088\b' || true
} > "${OUT}/live-services.txt"

# 5) Checksum
(
  cd "${OUT}"
  sha256sum ./* > SHA256SUMS 2>/dev/null || true
)

ln -sfn "${OUT}" "${BASE}/_backups/ahrefs-bar-latest"
echo ""
echo "OK backup ready:"
echo "  ${OUT}"
echo "  latest → ${BASE}/_backups/ahrefs-bar-latest"
ls -lh "${OUT}"
