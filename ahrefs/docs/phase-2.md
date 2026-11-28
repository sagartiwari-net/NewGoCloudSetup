# Phase 2 — aMember / session verify

**Status:** Done (2026-11-27)

## Delivered

### Server (`ahrefs/server`)

| Endpoint | Behavior |
|----------|----------|
| `GET /v1/portals` | Public portal list (no secrets) |
| `POST /v1/session/verify` | Cookie/portal → tm-check → mint `tb.*` session |
| `GET /v1/me` | Session + hourly/daily remaining |
| `GET /v1/dev/tm-check` | Local mock membership check (`AHREFS_BAR_DEV_AUTH=1`) |

Flow:
1. Fast path: existing `tb.*` token looked up in `ahrefs_bar_sessions`
2. Else: portal must be whitelisted → HTTP call to portal `check_url` (real `tm-check.php` or local mock)
3. On success: mint `tb.` + 12h TTL (configurable)

### Extension (`ahrefs/extension`)

Minimal MV3 auth-only extension (no Ahrefs toolbar yet):
- `tm-guard.js` — portals + verify + storage
- `popup.html` / `tm-popup.js` — status + Refresh + limits
- Local portal `https://local.dev` auto-synthesizes a dev session cookie

## Local test (curl)

```bash
# verify
curl -s -X POST http://127.0.0.1:8088/v1/session/verify \
  -H 'Content-Type: application/json' \
  -d '{"token":"dev-abc","portal":"https://local.dev"}'

# me (use session_token from verify)
curl -s http://127.0.0.1:8088/v1/me -H 'X-TM-Bar-Token: tb.…'
```

## Chrome extension

1. Keep API running: `cd ahrefs/server && ./ahrefs-bar-api`
2. Chrome → Extensions → Developer mode → **Load unpacked** → `ahrefs/extension`
3. Open popup → should show **access active** after Refresh
4. Limits show 50/150 remaining

## Prod note

Set `AHREFS_BAR_DEV_AUTH=0` and insert real portals with `check_url` = friend `tm-check.php` + `shared_secret`.

## Next

Phase 3 — `/v4/*` proxy with one Ahrefs account + DNR rules in extension.
