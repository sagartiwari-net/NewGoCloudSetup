# Shared DB (panel ↔ bar-api)

Panel CRUD and the toolbar proxy must see the **same** `ahrefs_bar_*` tables.

| Mode | How |
|------|-----|
| Local | `./scripts/run-local.sh` sets `AHREFS_BAR_SQLITE` → `panel-api/data/panel.db` and `AHREFS_BAR_SEED_DEV_ACCOUNTS=0` |
| Prod | Both use MySQL `gt4rents` (`AHREFS_BAR_DSN` + panel `GT4RENTS_MYSQL_*`) + `sql/001_ahrefs_bar.sql` |

Without this, panel “Dev MOCK” accounts never reach the extension proxy (separate sqlite files).
