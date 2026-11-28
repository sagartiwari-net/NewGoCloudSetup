#!/usr/bin/env bash
# Ensure operators.ahrefs_bar_enabled exists and optionally turn a reseller On.
#   source /www/wwwroot/gt4rents.com/_secrets/mysql.env
#   bash deploy/fix-ahrefs-bar-reseller-flag.sh              # migrate + list
#   bash deploy/fix-ahrefs-bar-reseller-flag.sh SemrushToolz  # + set On
set -euo pipefail

HOST="${GT4RENTS_MYSQL_HOST:-${MYSQL_HOST:-127.0.0.1}}"
PORT="${GT4RENTS_MYSQL_PORT:-${MYSQL_PORT:-3306}}"
USER="${GT4RENTS_MYSQL_USER:-${MYSQL_USER:-gt4rents}}"
PASS="${GT4RENTS_MYSQL_PASSWORD:-${MYSQL_PASSWORD:-}}"
DB="${GT4RENTS_MYSQL_DB:-${MYSQL_DATABASE:-gt4rents}}"

if [[ -z "${PASS}" ]]; then
  echo "ERROR: set GT4RENTS_MYSQL_PASSWORD (source _secrets/mysql.env first)"
  exit 1
fi

mysql_cmd=(mysql -h "$HOST" -P "$PORT" -u "$USER" "-p${PASS}" "$DB" -N -e)

echo "== column =="
"${mysql_cmd[@]}" "SHOW COLUMNS FROM operators LIKE 'ahrefs_bar_enabled';" || true

echo "== ensure column =="
"${mysql_cmd[@]}" "ALTER TABLE operators ADD COLUMN ahrefs_bar_enabled TINYINT(1) NOT NULL DEFAULT 0;" 2>/dev/null \
  || echo "(column already exists or alter skipped)"
"${mysql_cmd[@]}" "UPDATE operators SET ahrefs_bar_enabled=1 WHERE role='master';"

if [[ "${1:-}" != "" ]]; then
  user="$1"
  echo "== set On: ${user} =="
  "${mysql_cmd[@]}" "UPDATE operators SET ahrefs_bar_enabled=1 WHERE role='reseller' AND username='${user}';"
fi

echo "== resellers =="
"${mysql_cmd[@]}" "SELECT id, username, status, ahrefs_bar_enabled FROM operators WHERE role='reseller' ORDER BY username;"
