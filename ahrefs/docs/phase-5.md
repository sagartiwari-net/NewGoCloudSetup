# Phase 5 — Hourly + daily limits + custom user

**Status:** Done (2026-11-27)

## Rules

- **AND** windows: hourly rolling 60m **and** IST daily.
- Resolution: custom user → reseller defaults → 50/150.
- Only **billable** endpoints charge (`tbGetKeywordStatsV3` = 1; icon/header = 0).
- Over limit → `["Error","LimitReached","hourly"|"daily"]` (HTTP 200 soft).
- Upstream 5xx → optional refund (`refund_on_5xx`).

## Dev helpers

```bash
# set custom limits for session user
curl -s -X POST http://127.0.0.1:8088/v1/dev/limits \
  -H 'Content-Type: application/json' \
  -d '{"token":"tb.…","hourly":5,"daily":150}'

curl -s -X POST http://127.0.0.1:8088/v1/dev/meters/reset \
  -H 'Content-Type: application/json' \
  -d '{"token":"tb.…"}'
```

## Checks passed

- hourly=5 → 6th billable blocked  
- `/v1/me` shows used/remaining  
- cheap icon still works at hourly 0 remaining  
- daily=2 → 3rd billable blocked  
- `go test` meter cap  

## Popup note

**Test v4 proxy** now calls local API directly. Chrome blocks `https://ahrefs.com` → `http://127.0.0.1` DNR (mixed content). Prod HTTPS API will use DNR normally.

## Next

Phase 6 done — see `docs/phase-6.md`.
