# Phase 1 — Schema + server skeleton

**Status:** Done (2026-11-27)

## Delivered

- MySQL schema: `sql/001_ahrefs_bar.sql`
- Go API: `server/` with SQLite local default + MySQL via `AHREFS_BAR_DSN`
- Auto-migrate on boot + seed endpoint costs + owner defaults (reseller_id=1)
- `GET /healthz` live
- Phase 2/3 routes stubbed as `501`

## Run locally

```bash
cd ahrefs/server
go run .
# → http://127.0.0.1:8088/healthz
```

## Env

| Var | Default | Meaning |
|-----|---------|---------|
| `AHREFS_BAR_ADDR` | `127.0.0.1:8088` | Listen addr |
| `AHREFS_BAR_DSN` | _(empty)_ | MySQL DSN; empty → SQLite |
| `AHREFS_BAR_SQLITE` | `data/ahrefs_bar.db` | SQLite path |
| `AHREFS_BAR_TZ` | `Asia/Kolkata` | Reported timezone |
| `AHREFS_BAR_SESSION_HOURS` | `720` | Future session TTL |
| `AMEMBER_LITE_PATH` | _(empty)_ | Phase 2 |
| `AHREFS_BAR_CORS` | _(empty)_ | Extra CORS origins, comma-separated |

## Next

Phase 2 — `POST /v1/session/verify` (aMember / tm-check).
