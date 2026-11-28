# Phase 4 — Multi-account RR + issue detection

**Status:** Done (2026-11-27) — verified locally

## Behavior

1. **Healthy pool** — `status=active` and cooldown expired (auto-revive after cooldown).
2. **Sticky** — successful account stored on session for `sticky_minutes` (default 20).
3. **RR** — among healthy accounts, least-recently-used first.
4. **Failover** — on upstream `401/403`: mark account `bad` + 15m cooldown, clear sticky if needed, **retry next** account once (loop until success or pool empty).
5. **Usage log** — every attempt logs `account_id` + `status_code` (failed attempts credits=0).

## Dev seed

| Account | Token | Role |
|---------|-------|------|
| Dev Account A (bad) | `MOCK:401` | always 401 → triggers failover |
| Dev Account B (good) | `MOCK` | success |
| Dev Account (legacy) | disabled | ignored |

`GET /v1/dev/accounts` lists status (dev only).

## Checks passed

- healthz phase=4  
- A → 401 marked bad, B → 200  
- sticky_account_id = good  
- next request uses sticky good only  
- `go test` failover/mock helpers  

## Manual curl

```bash
TOKEN=$(curl -s -X POST http://127.0.0.1:8088/v1/session/verify \
  -H 'Content-Type: application/json' \
  -d '{"token":"x","portal":"https://local.dev"}' | jq -r .session_token)

curl -s "http://127.0.0.1:8088/v4/tbGetIconV3?input=$(python3 -c 'import urllib.parse,json;print(urllib.parse.quote(json.dumps({"target":"x.com"})))')" \
  -H "X-TM-Bar-Token: $TOKEN" | jq .

curl -s http://127.0.0.1:8088/v1/dev/accounts | jq .
```

## Next

Phase 5 — enforce hourly + daily credit limits (atomic) + custom per-user.
