# ToolsMandi Ahrefs Bar

## Status

| Phase | Status |
|-------|--------|
| 0–9 | Done (9 = deploy docs; apply on server) |
| **FP bind** | **ON** |
| **10 Reseller download** | **Done** — panel Download zip + tm-check |

See [PLAN.md](./PLAN.md) and `docs/phase-*.md`.

## Popup screenshot note

**access active** + credits dikhna = Phase 2–5 auth OK.

`Test v4` pe pehle `Failed to fetch` aata tha kyunki Chrome **https→http localhost DNR** block karta hai. Ab button **seedha** `127.0.0.1:8088/v4` hit karta hai. Dusri baar **browser_cache** dikhega; **Fresh** caches bypass karta hai.

## Run (local — shared panel DB)

```bash
# terminal 1: panel-api (Ahrefs CRUD)
cd NewGoCloudSetup/panel/panel-api && ./panel-api

# terminal 2: bar-api on same SQLite as panel
./ahrefs/scripts/run-local.sh

# self-check
./ahrefs/scripts/smoke-local.sh
```

Load unpacked: `ahrefs/extension` (reload after changes).  
Prod: `SSH_HOST=root@… SSH_KEY=… ./deploy/push-from-mac.sh` then server `install-on-server.sh` — see `docs/phase-9.md`.  
Download zip: panel → Ahrefs Bar → Download.
