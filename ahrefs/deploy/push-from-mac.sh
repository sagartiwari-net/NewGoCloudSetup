#!/usr/bin/env bash
# Copy deploy bundle to VPS. Requires working SSH.
# Usage:
#   SSH_HOST=root@65.109.16.196 SSH_KEY=~/.ssh/id_rsa ./deploy/push-from-mac.sh
#   # then on server:
#   ssh $SSH_HOST 'bash /www/wwwroot/gt4rents.com/_upload/ahrefs-bar/install-on-server.sh'
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
HOST="${SSH_HOST:-root@65.109.16.196}"
KEY_ARGS=()
if [[ -n "${SSH_KEY:-}" ]]; then
  KEY_ARGS=(-i "${SSH_KEY}")
fi
REMOTE_DIR="${REMOTE_DIR:-/www/wwwroot/gt4rents.com/_upload/ahrefs-bar}"

echo "Building linux amd64…"
(cd "${ROOT}/server" && GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -o "${ROOT}/deploy/ahrefs-bar-api.linux-amd64" .)

echo "Uploading to ${HOST}:${REMOTE_DIR}"
ssh "${KEY_ARGS[@]}" "${HOST}" "mkdir -p '${REMOTE_DIR}'"
scp "${KEY_ARGS[@]}" \
  "${ROOT}/deploy/ahrefs-bar-api.linux-amd64" \
  "${ROOT}/deploy/run-ahrefs-bar-api.sh" \
  "${ROOT}/deploy/ahrefs-bar-api.service" \
  "${ROOT}/deploy/ahrefs-bar-api.env.example" \
  "${ROOT}/deploy/install-on-server.sh" \
  "${ROOT}/deploy/backup-before-ahrefs-bar.sh" \
  "${ROOT}/deploy/nginx-barapi.upays.in.conf" \
  "${ROOT}/sql/001_ahrefs_bar.sql" \
  "${HOST}:${REMOTE_DIR}/"

# rename sql for install script fallback
ssh "${KEY_ARGS[@]}" "${HOST}" "cp '${REMOTE_DIR}/001_ahrefs_bar.sql' '${REMOTE_DIR}/001_ahrefs_bar.sql' 2>/dev/null; mkdir -p '${REMOTE_DIR}/../sql'; true"

# also sync extension + templates for panel zip
rsync -az -e "ssh ${KEY_ARGS[*]}" \
  --exclude '_metadata' --exclude '.DS_Store' \
  "${ROOT}/extension/" "${HOST}:${REMOTE_DIR}/extension/"
rsync -az -e "ssh ${KEY_ARGS[*]}" \
  "${ROOT}/templates/" "${HOST}:${REMOTE_DIR}/templates/"
# install script looks for ../extension relative to deploy dir — place siblings
ssh "${KEY_ARGS[@]}" "${HOST}" "mkdir -p '${REMOTE_DIR}/../extension' '${REMOTE_DIR}/../templates' '${REMOTE_DIR}/../sql' && \
  rsync -a '${REMOTE_DIR}/extension/' '${REMOTE_DIR}/../extension/' && \
  rsync -a '${REMOTE_DIR}/templates/' '${REMOTE_DIR}/../templates/' && \
  cp '${REMOTE_DIR}/001_ahrefs_bar.sql' '${REMOTE_DIR}/../sql/001_ahrefs_bar.sql' && \
  chmod +x '${REMOTE_DIR}/install-on-server.sh' '${REMOTE_DIR}/backup-before-ahrefs-bar.sh' '${REMOTE_DIR}/run-ahrefs-bar-api.sh'"

echo ""
echo "Uploaded. Safe install (backup first, only ahrefs-bar-api — no tool/panel restart):"
echo "  ssh ${HOST} 'bash ${REMOTE_DIR}/install-on-server.sh'"
