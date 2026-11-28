# Phase 6 — Browser + server cache

## Done

| Piece | Detail |
|-------|--------|
| Extension IndexedDB | `tm-cache.js` — `domain:` / `keyword:` / `serp:` keys, TTL 72h, LRU 500 domains |
| Guard wiring | `importScripts('tm-cache.js')`; cache-first `tm:test-proxy`; messages for stats/clear/prewarm |
| Refresh | `X-TM-Bar-Fresh: 1` or `?fresh=1` → skip browser + server mem cache |
| Server mem cache | Existing 15m cheap cache for `tbGetIconV3` / `tbGetHeaderV3` |
| In-flight dedup | Identical concurrent cheap GETs coalesce (`X-TM-Bar-Cache: DEDUP`) |
| Popup | Cache stats, **Test v4** (cache-first), **Fresh**, **Clear cache** |

## Verify

1. Reload unpacked extension (`ahrefs/extension`).
2. **Test v4** once → `mode: "direct_local_api"`, then again → `mode: "browser_cache"`, `cache: true`.
3. **Fresh** → `mode: "direct_local_api_fresh"`, bypasses IndexedDB + server HIT.
4. Popup **Cache:** line shows entity count after first hit.
5. Server response header `X-TM-Bar-Cache`: `HIT` | `MISS` | `FRESH` | `DEDUP`.

## Messages (extension)

| Type | Purpose |
|------|---------|
| `tm:test-proxy` | `{ target, fresh? }` — icon resolve |
| `tm:cache-stats` | entity counts / TTL |
| `tm:cache-clear` | wipe IndexedDB store |
| `tm:cache-prewarm` | `{ engine, query, domains[] }` SERP → domain upsert |
| `tm:cache-get-domain` | `{ target }` |

## Next

Phase 7 done — see `docs/phase-7.md`.
