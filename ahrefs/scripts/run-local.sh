#!/usr/bin/env bash
# Local bar-api sharing the same SQLite as panel-api (ahrefs_bar_* tables).
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
PANEL_DB="${AHREFS_BAR_SQLITE:-$ROOT/../NewGoCloudSetup/panel/panel-api/data/panel.db}"
cd "$ROOT/server"

export AHREFS_BAR_ADDR="${AHREFS_BAR_ADDR:-127.0.0.1:8088}"
export AHREFS_BAR_SQLITE="$PANEL_DB"
export AHREFS_BAR_DEV_AUTH="${AHREFS_BAR_DEV_AUTH:-1}"
export AHREFS_BAR_MOCK_UPSTREAM="${AHREFS_BAR_MOCK_UPSTREAM:-1}"
export AHREFS_BAR_REQUIRE_FP="${AHREFS_BAR_REQUIRE_FP:-1}"
export AHREFS_BAR_SEED_DEV_ACCOUNTS="${AHREFS_BAR_SEED_DEV_ACCOUNTS:-0}"

echo "bar-api → sqlite $AHREFS_BAR_SQLITE (seed_dev_accounts=$AHREFS_BAR_SEED_DEV_ACCOUNTS fp=$AHREFS_BAR_REQUIRE_FP)"
go build -o ahrefs-bar-api .
exec ./ahrefs-bar-api
