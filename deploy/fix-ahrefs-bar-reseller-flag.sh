#!/usr/bin/env bash
# Ensure operators.ahrefs_bar_enabled exists and optionally turn a reseller On.
#   source /www/wwwroot/gt4rents.com/_secrets/mysql.env
#   bash deploy/fix-ahrefs-bar-reseller-flag.sh              # migrate + list
#   bash deploy/fix-ahrefs-bar-reseller-flag.sh SemrushToolz  # + set On
set -euo pipefail

: "${MYSQL_HOST:=127.0.0.1}"
: "${MYSQL_PORT:=3306}"
: "${MYSQL_USER:?set MYSQL_USER}"
: "${MYSQL_PASSWORD:?set MYSQL_PASSWORD}"
: "${MYSQL_DATABASE:=gt4rents}"

mysql_cmd=(mysql -h "$MYSQL_HOST" -P "$MYSQL_PORT" -u "$MYSQL_USER" "-p${MYSQL_PASSWORD}" "$MYSQL_DATABASE" -N -e)

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
