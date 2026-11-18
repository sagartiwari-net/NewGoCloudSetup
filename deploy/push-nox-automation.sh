#!/usr/bin/env bash
# Deploy NoxTools automation (Home + Semrush 1 + Site Audit 1) like Azad.
# Run ON the gt4rents server:
#   cd /www/wwwroot/gt4rents.com/_repo && git pull && chmod +x deploy/push-nox-automation.sh && ./deploy/push-nox-automation.sh
#
# Prerequisite: create Credentials → provider NoxTools (username/password) in the panel once.
# On panel-api start, jobs auto-seed: NoxTools Home, NoxTools Semrush 1 (nxsm1), NoxTools Site Audit 1 (nxaud1).
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "${ROOT}"

echo "== build panel-api =="
"${ROOT}/deploy/build-panel.sh"

echo "== restart panel-api =="
"${ROOT}/deploy/start-panel-api.sh"

echo "== verify noxtools jobs (after API boot seed) =="
if [[ -f /www/wwwroot/gt4rents.com/_secrets/mysql.env ]]; then
  # shellcheck disable=SC1091
  set -a
  source /www/wwwroot/gt4rents.com/_secrets/mysql.env
  set +a
  sleep 2
  mysql -h"${GT4RENTS_MYSQL_HOST:-127.0.0.1}" -P"${GT4RENTS_MYSQL_PORT:-3306}" \
    -u"${GT4RENTS_MYSQL_USER:-gt4rents}" -p"${GT4RENTS_MYSQL_PASSWORD}" \
    "${GT4RENTS_MYSQL_DB:-gt4rents}" -e "
SELECT c.id AS cred_id, c.name AS cred, j.id AS job_id, j.name, j.job_kind, j.enabled, j.interval_minutes, w.domain
FROM automation_credentials c
LEFT JOIN automation_jobs j ON j.credential_id=c.id
LEFT JOIN websites w ON w.id=j.target_website_id
WHERE c.provider='noxtools'
ORDER BY j.job_kind DESC, j.id;
" || echo "SQL verify skipped"
else
  echo "mysql.env missing — open panel Automation → Jobs after credential exists"
fi

echo "DONE. Panel → Automation → Jobs: enable/run NoxTools Home, Semrush 1, Site Audit 1."
