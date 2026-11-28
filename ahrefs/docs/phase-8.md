# Phase 8 — Polish + local QA

## Done

| Piece | Detail |
|-------|--------|
| Fingerprint | Now default ON — see `docs/fingerprint.md` |
| Verify rate limit | `AHREFS_BAR_VERIFY_RATE` (default 60/min per IP) |
| Telegram | Pool empty + usage spike alerts via bot token/chat env |
| Cache compact | IndexedDB stores drop null/empty fields |
| QA matrix | `docs/qa-matrix.md` |
| Panel verify | CRUD + reseller scope checked on local panel-api |

## Env

```bash
# optional hardening
export AHREFS_BAR_REQUIRE_FP=0
export AHREFS_BAR_VERIFY_RATE=60

# telegram (optional)
export AHREFS_BAR_TELEGRAM_BOT_TOKEN=123:ABC
export AHREFS_BAR_TELEGRAM_CHAT_ID=-100…
export AHREFS_BAR_SPIKE_CREDITS=80
export AHREFS_BAR_SPIKE_WINDOW_MIN=5
```

## Compression notes

- Browser cache: compact JSON entities (no null/empty keys); LRU 500 domains; TTL 72h.
- Server: cheap responses stay small; in-memory cache capped by entry TTL 15m.
- Prod: put Nginx `gzip` / `brotli` in front of bar-api for JSON bodies.

## Next

Phase 9 — deploy bar-api (systemd/Nginx, prod `API_BASE`, seed).
