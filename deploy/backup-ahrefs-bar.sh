#!/usr/bin/env bash
# Thin wrapper — same backup script under ahrefs/deploy/
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
exec bash "${ROOT}/ahrefs/deploy/backup-before-ahrefs-bar.sh"
