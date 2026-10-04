#!/usr/bin/env bash
# Truncate gt4rents tool/panel logs when over size limit (safe while apps run).
# Usage:
#   ./deploy/trim-logs.sh              # trim if over LOG_MAX_MB (default 80)
#   LOG_MAX_MB=50 ./deploy/trim-logs.sh
#   ./deploy/trim-logs.sh --install-cron   # install every-8h cron (≈3x/day)
set -euo pipefail

ROOT="${GT4RENTS_ROOT:-/www/wwwroot/gt4rents.com}"
MAX_MB="${LOG_MAX_MB:-80}"
MAX_BYTES=$((MAX_MB * 1024 * 1024))
SCRIPT="$(cd "$(dirname "$0")" && pwd)/trim-logs.sh"
CRON_TAG="# gt4rents-trim-logs"
CRON_LINE="0 */8 * * * LOG_MAX_MB=${MAX_MB} ${SCRIPT} >>/var/log/gt4rents-trim-logs.log 2>&1 ${CRON_TAG}"

if [[ "${1:-}" == "--install-cron" ]]; then
  if ! command -v crontab >/dev/null 2>&1; then
    echo "FAIL: crontab not found"
    exit 1
  fi
  tmp="$(mktemp)"
  crontab -l 2>/dev/null | grep -v "${CRON_TAG}" >"${tmp}" || true
  echo "${CRON_LINE}" >>"${tmp}"
  crontab "${tmp}"
  rm -f "${tmp}"
  touch /var/log/gt4rents-trim-logs.log 2>/dev/null || true
  echo "OK: cron installed (every 8h, limit > ${MAX_MB}MB → truncate)"
  echo "    ${CRON_LINE}"
  crontab -l | grep "${CRON_TAG}" || true
  exit 0
fi

trimmed=0
checked=0
bytes_freed=0

trim_one() {
  local f="$1"
  [[ -f "$f" ]] || return 0
  checked=$((checked + 1))
  local sz
  sz="$(stat -c%s "$f" 2>/dev/null || stat -f%z "$f" 2>/dev/null || echo 0)"
  # non-numeric guard
  [[ "${sz}" =~ ^[0-9]+$ ]] || return 0
  if (( sz > MAX_BYTES )); then
    : >"${f}"
    trimmed=$((trimmed + 1))
    bytes_freed=$((bytes_freed + sz))
    echo "$(date '+%Y-%m-%d %H:%M:%S') TRIM ${f} was=$((sz / 1024 / 1024))MB limit=${MAX_MB}MB"
  fi
}

shopt -s nullglob
for f in "${ROOT}"/*/app.log; do
  trim_one "${f}"
done
for f in "${ROOT}/panel/"*.log; do
  trim_one "${f}"
done

echo "$(date '+%Y-%m-%d %H:%M:%S') DONE checked=${checked} trimmed=${trimmed} freed_approx_MB=$((bytes_freed / 1024 / 1024)) limit_MB=${MAX_MB}"
