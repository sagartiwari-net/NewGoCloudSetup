# Phase 7 — Panel UI (Ahrefs Bar)

## Done

| Piece | Detail |
|-------|--------|
| Nav | update-panel group **Ahrefs Bar** (owner + reseller) |
| API | `/api/ahrefs-bar/*` on panel-api, scoped by `reseller_id = op.ID` |
| Screens | Overview, Accounts, Portals, Defaults, User limits, Usage |
| Secrets | Account tokens + portal secrets masked in lists; blank on edit keeps current |
| Schema | `ensureAhrefsBarTables` on panel DB open (also apply `ahrefs/sql/001_ahrefs_bar.sql` on MySQL) |

## Routes

| Method | Path | Who |
|--------|------|-----|
| GET | `/api/ahrefs-bar/overview` | both |
| GET/POST/DELETE | `/api/ahrefs-bar/accounts` | both (own scope) |
| GET | `/api/ahrefs-bar/accounts/{id}` | both |
| GET/POST/DELETE | `/api/ahrefs-bar/portals` | both |
| GET/POST | `/api/ahrefs-bar/settings` | both |
| GET | `/api/ahrefs-bar/costs` | both |
| POST | `/api/ahrefs-bar/costs` | master |
| GET/POST/DELETE | `/api/ahrefs-bar/user-limits` | both |
| GET | `/api/ahrefs-bar/usage` | both |

## Local check

1. Restart panel-api (sqlite or MySQL).
2. Open update-panel → sidebar **Ahrefs Bar**.
3. Add account with `MOCK` token, portal `https://local.dev`, save defaults.
4. Reseller login → only own rows.

## Next

Phase 8 done — see `docs/phase-8.md`.
