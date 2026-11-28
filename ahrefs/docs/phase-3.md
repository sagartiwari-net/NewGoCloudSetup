# Phase 3 — v4 proxy + single account

**Status:** Done (2026-11-27) — verified with local checks

## Delivered

### Server

| Piece | Detail |
|-------|--------|
| `GET\|POST /v4/{endpoint}` | Session auth → pick account → mock or real Ahrefs |
| `tbGetUserInfoV3` | Sanitized ToolsMandi payload (no real Ahrefs workspace) |
| Account seed | Dev Account with `MOCK` token (or `AHREFS_BAR_ACCOUNT_JSON`) |
| Cheap cache | In-memory 15m for `tbGetIconV3` / `tbGetHeaderV3` |
| Usage log | endpoint, credits, cache_hit, status |

### Extension

| Piece | Detail |
|-------|--------|
| `tm-rules.json` | `ahrefs.com/v4/*` → `http://127.0.0.1:8088/v4/*` |
| Header rule | `X-TM-Bar-Token` on proxy host |
| Popup | **Test v4 proxy** button (fetch via DNR) |
| Storage | also sets `ahIsAuth3` / `ahToken` for future toolbar UI |

## Checks run (all passed)

1. healthz phase=3, mock_upstream=true  
2. session verify → `tb.*`  
3. `/v4` without token → `["Error","Unauthorized"]`  
4. userInfo sanitize → workspace ToolsMandi  
5. icon mock → DR 72  
6. second icon → `cache_hit=1` in usage_log  
7. Dev Account row active  
8. keyword stats → credits=1 logged  
9. `go test` token extract  

## Run

```bash
cd ahrefs/server && go run .
```

Chrome → reload unpacked `ahrefs/extension` → Refresh → **Test v4 proxy**.

## Real Ahrefs token (optional)

```bash
export AHREFS_BAR_MOCK_UPSTREAM=0
export AHREFS_BAR_ACCOUNT_JSON='{"accessToken":"YOUR_TOKEN","expiresIn":3600}'
go run .
```

## Next

Phase 4 — multi-account round-robin + bad-account failover.
