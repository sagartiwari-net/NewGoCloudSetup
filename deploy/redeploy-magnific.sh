#!/usr/bin/env bash
# Magnific now lives on short slug mgfc (Safe Browsing). Prefer redeploy-mgfc.sh.
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
exec "${ROOT}/deploy/redeploy-mgfc.sh"
