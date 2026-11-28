# ToolsMandi Ahrefs Bar — Master Build Plan

**Brand:** ToolsMandi (extension UI can still look like Ahrefs Toolbar; ownership/auth = ToolsMandi)  
**Repo folder:** `oneclickgo/ahrefs/`  
**Panel home:** existing `NewGoCloudSetup/panel` (update-panel + panel-api) — **no separate admin panel**  
**Goal:** Scratch se Ahrefs SEO Toolbar proxy system — aMember Pro verify, multi-account RR, hourly+daily credits, reseller isolation, browser cache, local test → then server deploy.

---

## 0. Conclusion (sab research ka final take)

| Topic | Decision |
|-------|----------|
| Purana `/bar` (custom login) | Reference only — naya system aMember-first |
| Purana `/bar2` | Auth pattern reuse (cookie → verify → `X-TB*-Token`) — Markhor upstream **mat** rakho |
| Markhor `mkhr-ahrefs-bar` | Client thin auth + server metering pattern copy karo; unka upstream/token mat use karo |
| Khan / 1clk forks | Skip — activation-code model nahi |
| Ahrefs token | **Kabhi client pe nahi** — sirf server pool |
| Traffic path | **Har** `ahrefs.com/v4/*` → hamara proxy |
| Limits | Hourly **AND** daily, dono enforce; per-user custom override |
| Reseller | Users + accounts + limits reseller-scoped (jaise tools) |
| Manage UI | Existing update-panel me naya section **Ahrefs Bar** |
| Cache | Server cheap-cache + browser IndexedDB (2–3 din TTL, per-item Refresh) |
| Local first | Local Docker/PHP-or-Go + unpacked Chrome ext; server push baad me |

**Ek line:** Thin Chrome extension + strong Go/PHP proxy API + existing panel CRUD = ToolsMandi Ahrefs Bar v1.

---

## 1. Product behavior (user-facing)

1. User apne reseller ke aMember member area pe login karta hai.
2. Extension portal cookie padh kar proxy session banati hai.
3. Google SERP / page pe toolbar metrics dikhti hain.
4. SERP pe jo domains aaye, unke summary metrics local cache me save.
5. Baad me wahi domain open ho → cache se dikhao (0 credit) jab tak TTL / Refresh na ho.
6. Limit hit → soft error / blocked metrics; Ahrefs pe call nahi.
7. Account dead (401/403) → auto next account (user ko pata nahi).

---

## 2. High-level architecture

```
┌──────────────────────────┐
│ Chrome extension         │
│  guard + DNR + cache     │
│  (no real Ahrefs token)  │
└────────────┬─────────────┘
             │ X-TM-Bar-Token / portal cookie
             ▼
┌──────────────────────────┐     ┌─────────────────────┐
│ Ahrefs Bar API (proxy)   │────▶│ aMember / tm-check   │
│  auth · meters · RR      │     └─────────────────────┘
│  cache · forward v4      │
└────────────┬─────────────┘
             │ Bearer from account pool
             ▼
      https://ahrefs.com/v4/*

┌──────────────────────────┐
│ Existing update-panel    │
│  Ahrefs Bar section      │
│  owner + reseller scoped │
└────────────┬─────────────┘
             │
             ▼
        panel-api / MySQL (gt4rents or dedicated ahrefs_bar DB)
```

### Folder layout (is repo me)

```
ahrefs/
  PLAN.md                 ← yeh document
  README.md               ← short status + how to run local
  docs/                   ← feature specs (per phase)
  extension/              ← Chrome MV3 source (build yahan)
  server/                 ← proxy API (Go preferred, align panel-api)
  sql/                    ← schema migrations
  panel-notes/            ← UI/API hooks into NewGoCloudSetup/panel
```

Panel code changes live in:
- `NewGoCloudSetup/panel/panel-api/`
- `NewGoCloudSetup/panel/update-panel/`

Proxy can live in `ahrefs/server/` as its own binary/service (recommended), ya baad me panel-api me merge — **Phase 0 decision: alag `ahrefs-bar-api` binary**, panel sirf manage kare.

---

## 3. Auth approach (aMember Pro)

### 3.1 Approach

Markhor/Bar2 style **portal-session gating**:

1. Extension whitelisted portal(s) se session cookie padhe.
2. `POST /bar-api/v1/session/verify` with cookie + portal URL + fingerprint.
3. Server:
   - local `Am_Lite` **ya** reseller `tm-check.php`
   - product ID gate (Ahrefs Bar product)
   - map → `reseller_id` via portal binding table
4. Return short-lived `bar_session_token` (`tb.…`).
5. DNR: har `ahrefs.com/v4/*` → `https://<bar-api-host>/v4/*` + header `X-TM-Bar-Token`.

### 3.2 Why not custom username/password

Tum already aMember se sell karte ho; tools/resellers bhi product IDs se chaltee hain. Alag user DB = sync hell. aMember = source of truth for “paid hai ya nahi”.

### 3.3 Reseller bind

| Table idea | Role |
|------------|------|
| `ahrefs_bar_portals` | site_url, cookie_name, check_url, reseller_operator_id, product_ids |
| Session | always stores `reseller_id` + `amember_user_login` |

Har request pe account pool + limits **usi reseller_id** se. Cross-reseller impossible.

### 3.4 Implementation notes

- Reuse patterns from `ahrefs_ext/bar2_server/amember_helper.php` + `ext-verify.php` (logic port to Go ya PHP).
- Cookie names: `PHPSESSID`, `amember*`, custom (SemrushToolz-style).
- Chrome drops `Authorization` across DNR redirect → **custom header** mandatory (`X-TM-Bar-Token`).

---

## 4. Multi-account pool (update system)

### 4.1 What admin/reseller can do (panel)

- Add / edit / disable Ahrefs accounts (token JSON / cookie export paste).
- Enable/disable, notes, last_error, cooldown_until.
- See health: ok / bad / cooldown.
- Optional: sticky minutes per user.

### 4.2 Round-robin + issue detection

**On each billable upstream call:**

1. Resolve reseller pool (`status=active`, `cooldown` expired).
2. Prefer sticky account for user if still healthy (e.g. 15–30 min).
3. Else pick least-recently-used / RR among healthy.
4. Forward to `ahrefs.com/v4` with that Bearer.
5. On auth/plan failures (401/403, known dead JSON):
   - mark account `bad` or `cooldown`
   - **one** automatic retry on next healthy account
   - log event for panel Analytics
6. If no healthy account → return controlled error; Telegram optional.

### 4.3 Approach

Same mental model as tools accounts page (`update-panel` Accounts) — but data in `ahrefs_bar_accounts` (not mix with website cookie tools unless you explicitly want later).

**Phase 1:** separate tables under same MySQL.  
**Later (optional):** link to existing `accounts` rows — not required for v1.

---

## 5. Credits & limits

### 5.1 Rules (AND)

Example: hourly 50 + daily 150  

- 1 hour sliding **or** calendar hour window → max 50  
- 1 calendar day (IST) → max 150  
- Jo pehle hit → block  

**Resolution order for limit values:**

1. Per-user custom (if `is_custom`)
2. Else reseller default for Ahrefs Bar
3. Else global default

### 5.2 Meters

| key | window | reset |
|-----|--------|-------|
| `credits_hourly` | rolling 60 min **or** clock hour | continuous / hourly |
| `credits_daily` | Asia/Kolkata day | midnight IST |

**Recommendation (implement):**  
- Daily = calendar day IST (panel meters jaisa simple).  
- Hourly = **rolling 60 minutes** (fairer than clock-hour cliff) — document clearly in UI.

### 5.3 What costs credit?

| Class | Examples | Credit | Cache |
|-------|----------|--------|-------|
| Free / infra | `tbGetUserInfoV3`, health | 0 | short |
| Cheap | `tbGetHeaderV3`, `tbGetIconV3` | 0 default (server cache) | 12–24h shared |
| Billable | keyword stats, SERP metrics, heavy icon misses after policy | 1 (configurable weight) | browser 2–3d + optional server |

**Config table** `ahrefs_bar_endpoint_costs` so tum baad me tune kar sako bina extension release ke.

### 5.4 Atomic enforce

```
BEGIN;
  lock user meters
  if hourly_used >= hourly_limit OR daily_used >= daily_limit → ROLLBACK + 429
  increment both as needed
COMMIT;
  then upstream call
```

Agar upstream fail (5xx / network) → optional refund credit (policy flag).

### 5.5 Custom per-user

Panel Users (scoped): set custom hourly/daily; `is_custom=1`.  
Reseller sirf apne aMember-mapped users dekhe (portal sessions / usage log se discover, ya manual link).

---

## 6. Browser + server cache (credit bachao)

### 6.1 Browser (extension)

**Store:** IndexedDB `tm-ahrefs-cache`  
**Shape:** keyed entities (not flat array):

```
domain:example.com → { dr, ur, traffic, orgKw, …, cachedAt, source }
keyword:best crm → { volume, kd, …, cachedAt }
serp:google:best crm → { domains: [...], cachedAt }
```

**TTL default:** 72 hours (panel setting 24 / 72 / 168).  
**Cap:** 500–1000 domain entities, LRU eviction.  
**Compress:** light JSON plain; large SERP blobs → `fflate` gzip.  
**UI:** age label + **Refresh** (sirf us entity pe force network).

**SERP prewarm:** SERP response save karte waqt har result domain upsert → domain visit pe hit.

### 6.2 Server

- Shared cache for cheap endpoints (icon/header) by target URL.
- Dedup in-flight identical requests (1–2s).
- Do **not** rely only on browser — multi-device users still hit server; server cache saves Ahrefs account.

### 6.3 Refresh button

Content/popup control → request with `X-TM-Bar-Fresh: 1` or `?fresh=1` → skip caches → still counts credit → overwrite stores.

---

## 7. Panel integration (existing update-panel)

### 7.1 Principle

**Naya panel mat banao.** `NewGoCloudSetup/panel/update-panel` me nav item:

**Ahrefs Bar** (owner + reseller)

### 7.2 Screens (v1)

| Screen | Owner | Reseller |
|--------|-------|----------|
| Overview (usage, healthy accounts, errors) | Yes | Own scope |
| Accounts (Ahrefs tokens pool) | Yes | Own pool |
| Portals (aMember sites bind) | Yes | Own portals |
| Defaults (hourly/daily limits, TTL, costs) | Yes | Own defaults |
| Users / custom limits | Yes | Own users |
| Usage log | Yes | Own |
| Extension build/download | Later phase | Later |
| Setup file upload (tm-check etc.) | Later phase | Later |

### 7.3 AuthZ

Reuse `operators.role` + `operator_websites` style scoping.  
For Ahrefs Bar use `ahrefs_bar_reseller_acl` or `reseller_id` on every row.  
Reseller API queries always `WHERE reseller_id = auth.operator_id` (owner sees all).

### 7.4 Approach for UI

- Mirror patterns from `accounts/page.tsx`, `limits/page.tsx`, `users/page.tsx`, `resellers/page.tsx`.
- New panel-api routes under `/api/ahrefs-bar/*`.
- Keep Ahrefs Bar DB tables clearly prefixed `ahrefs_bar_*` to avoid tool-meter collisions.

---

## 8. Extension design

### 8.1 Base

Official Ahrefs Toolbar bundle (`event.js`, `content.js`, …) + **our** thin layer:

| File | Role |
|------|------|
| `tm-guard.js` | SW: importScripts event.js; portal sync; DNR headers |
| `tm-rules.json` | redirect v4 → our API |
| `tm-cache.js` | IndexedDB cache intercept / message API |
| `tm-popup.js` | status + remaining credits strip |
| `tm-fresh.js` | Refresh control hooks (phase 2) |

### 8.2 Branding

- Internal name: ToolsMandi Ahrefs Bar  
- Manifest display can stay “Ahrefs SEO Toolbar” (compatible) **or** soft-brand — decide at ship; default keep Ahrefs UI for UX familiarity.

### 8.3 Local test

Chrome → Load unpacked → `ahrefs/extension/`  
Hosts: `http://127.0.0.1:PORT` + real aMember staging portal.

---

## 9. Server (proxy API) design

### 9.1 Stack recommendation

**Go** (same family as `panel-api`) for:
- concurrent proxy
- atomic meters
- easy deploy next to existing tools

PHP acceptable if faster to port Bar2 — but long-term Go better. **Plan assumes Go** in `ahrefs/server`.

### 9.2 Core endpoints

| Method | Path | Purpose |
|--------|------|---------|
| POST | `/v1/session/verify` | aMember cookie → bar session |
| GET/POST | `/v4/{endpoint}` | Ahrefs proxy (DNR target) |
| GET | `/v1/me` | limits remaining, reseller label |
| POST | `/v1/cache/bypass` meta | optional |
| GET | `/healthz` | local/prod health |

Panel CRUD stays on **panel-api**, not necessarily on bar-api (bar-api reads same DB).

### 9.3 `tbGetUserInfoV3` sanitize

Return placeholder email + plan flags so toolbar stays “logged in”; never leak real Ahrefs workspace.  
Optional: inject remaining credits into a custom popup field (our strip), not Ahrefs email.

---

## 10. Data model (draft)

```sql
-- sql/001_ahrefs_bar.sql (to be written in Phase 1)

ahrefs_bar_resellers_settings (
  reseller_id PK,  -- operators.id
  hourly_limit INT,
  daily_limit INT,
  cache_ttl_hours INT DEFAULT 72,
  sticky_minutes INT DEFAULT 20,
  ...
)

ahrefs_bar_portals (
  id, reseller_id, site_url, cookie_name, check_url, product_ids, enabled
)

ahrefs_bar_accounts (
  id, reseller_id, name, access_token_json, status, -- active|bad|disabled
  last_error, cooldown_until, last_used_at, created_at
)

ahrefs_bar_sessions (
  token PK, reseller_id, amember_login, portal_id,
  fingerprint, expires_at, last_seen
)

ahrefs_bar_user_limits (
  reseller_id, amember_login, hourly_limit, daily_limit, is_custom
)

ahrefs_bar_usage_hourly (
  reseller_id, amember_login, bucket_start, used
)

ahrefs_bar_usage_daily (
  reseller_id, amember_login, day_ist, used
)

ahrefs_bar_usage_log (
  id, reseller_id, amember_login, account_id, endpoint, target,
  credits, cache_hit, status, created_at
)

ahrefs_bar_endpoint_costs (
  endpoint PK, credits INT, class ENUM('free','cheap','billable')
)

ahrefs_bar_server_cache (
  cache_key PK, body MEDIUMBLOB, status INT, expires_at
)
```

Exact SQL files land in `ahrefs/sql/` during Phase 1.

---

## 11. Phased implementation (kab kya)

### Phase 0 — Bootstrap (day 0) ✅

- [x] Create `ahrefs/` folder  
- [x] Write this `PLAN.md`  
- [x] `README.md` with status  
- [x] Freeze decisions below (section 14) — defaults accepted

### Phase 1 — Schema + empty server skeleton ✅

**Do:**
1. Write `sql/001_ahrefs_bar.sql`
2. Scaffold `server/` Go module: healthz, config, DB connect
3. Local run with empty routes
4. Document env: `AHREFS_BAR_DSN`, `AMEMBER_LITE_PATH`, `LISTEN`

**Done when:** `curl localhost/healthz` → ok + migrations apply clean.  
**Completed:** 2026-11-27 — see `docs/phase-1.md`

### Phase 2 — aMember session verify ✅

**Do:**
1. Port verify logic (Am_Lite / tm-check)
2. `POST /v1/session/verify`
3. Session table + token mint (`tb.`)
4. Minimal extension `tm-guard.js` against local API (no real Ahrefs yet)

**Done when:** login on staging aMember → extension shows authed; logout → parked.  
**Completed:** 2026-11-27 — local.dev mock + curl + extension auth UI; see `docs/phase-2.md`

### Phase 3 — v4 proxy + single account ✅

**Do:**
1. Panel/SQL: insert one Ahrefs account manually
2. `/v4/*` forward with Bearer
3. DNR rules to local/dev host
4. Sanitize `tbGetUserInfoV3`

**Done when:** toolbar shows DR on a test domain via our proxy (1 account).  
**Completed:** 2026-11-27 — mock DR via `/v4` + DNR + popup Test button; see `docs/phase-3.md`  
*(Full official toolbar UI bundle still Phase later; proxy path verified end-to-end.)*

### Phase 4 — Multi-account RR + issue detection ✅

**Do:**
1. Account picker + sticky
2. Bad/cooldown marking + retry
3. Usage log account_id

**Done when:** kill token on account A → auto switch B without extension change.  
**Completed:** 2026-11-27 — MOCK:401→MOCK failover + sticky; see `docs/phase-4.md`

### Phase 5 — Hourly + daily limits + custom user ✅

**Do:**
1. Meter tables + atomic check
2. Endpoint cost map
3. 429 / soft toolbar errors
4. `/v1/me` remaining for popup strip

**Done when:** set hourly 5 → 6th billable blocked; daily still independent; custom user override works.  
**Completed:** 2026-11-27 — see `docs/phase-5.md`

### Phase 6 — Caching (server + browser)

**Do:**
1. Server cheap cache + dedup
2. Extension IndexedDB entity store
3. SERP prewarm → domain reuse
4. TTL 72h + Refresh header
5. LRU cap

**Done when:** refresh same SERP twice → second pass near-zero billable; domain open uses SERP cache; Refresh forces 1 credit.  
**Completed:** 2026-11-27 — see `docs/phase-6.md`

### Phase 7 — Panel UI (manage everything)

**Do:**
1. Nav **Ahrefs Bar** in update-panel
2. panel-api CRUD routes + reseller scoping
3. Accounts, portals, defaults, users custom limits, usage
4. Owner vs reseller permissions

**Done when:** reseller logs into panel → only own pools/users; can update token without SSH.  
**Completed:** 2026-11-27 — see `docs/phase-7.md`

### Phase 8 — Polish + local QA matrix

**Do:**
1. Fingerprint optional
2. Telegram on pool empty / spike
3. QA checklist (section 13)
4. Compression tuning

**Completed:** 2026-11-27 — see `docs/phase-8.md` + `docs/qa-matrix.md`

### Phase 9 — Deploy server

**Do:**
1. systemd / aaPanel deploy bar-api
2. Nginx host e.g. `barapi.toolsmandi.com` or path on existing
3. Extension API_BASE → prod
4. Seed owner portal + first accounts
5. Smoke test

**Completed (repo):** 2026-11-27 — `deploy/*` + `docs/phase-9.md` + FP bind default ON (`docs/fingerprint.md`). Server apply still operator step.

### Phase 10 — Reseller distribution

- Reseller extension download (branded build / bound portal list)
- Upload `tm-check.php` / setup files from panel
- Auto-build CRX / zip per reseller
- Optional merge meters into global tools UI

**Completed (v1):** 2026-11-27 — zip + tm-check download from panel (`docs/phase-10.md`). CRX store publish / meters merge still later.

---

## 12. Local testing plan

| # | Test | Expect |
|---|------|--------|
| 1 | No aMember cookie | Extension not authed |
| 2 | Valid cookie + product | Authed + `/v1/me` limits |
| 3 | Wrong product | Reject |
| 4 | Proxy icon/header | 200 via our host |
| 5 | Billable × N | meters increase |
| 6 | Hourly cap | block |
| 7 | Daily cap | block even if hourly remaining |
| 8 | Custom user limits | override defaults |
| 9 | Account 401 | switch + log |
| 10 | SERP then open domain | cache hit 0 credit |
| 11 | Refresh button | credit++ / fresh data |
| 12 | Reseller A vs B | no data leak |
| 13 | Page spam refresh | cheap/cached; billable not exploding |

---

## 13. Security & ops

- Tokens encrypted at rest (or restricted DB perms)
- Admin paste UI never echo full token in lists (mask)
- Rate-limit verify endpoint
- CORS only extension + panel origins in prod
- IST timezone consistent
- Backups for `ahrefs_bar_accounts`
- Never log full Bearer in usage_log

---

## 14. Open decisions (freeze before Phase 1 code)

Answer these once, then implement:

1. **Proxy stack:** Go (recommended) vs PHP port of Bar2? → default **Go**
2. **DB:** same `gt4rents` MySQL vs new `ahrefs_bar` database? → default **same MySQL, prefixed tables**
3. **Hourly window:** rolling 60m vs clock hour? → default **rolling 60m**
4. **Cheap endpoints credit:** 0 or 1? → default **0** with server cache
5. **Extension display name:** keep Ahrefs vs ToolsMandi branding? → default **keep Ahrefs UI**
6. **Dev API host:** `http://127.0.0.1:8088` ?

Update this section when decided.

---

## 15. Mapping to existing code (reuse, don’t rewrite blindly)

| Need | Steal / learn from |
|------|--------------------|
| aMember verify | `Samsung/.../ahrefs_ext/bar2_server/amember_helper.php` |
| DNR + guard | `bar2-extension/bar2-guard.js`, Downloads `mkhr-guard.js` |
| Direct Ahrefs forward | `own_ext_server/v4/index.php` `forwardAhrefsRequest` |
| Panel meters UX | `panel-api` user_meters + `update-panel` limits/users |
| Reseller isolation | `RESELLER-TOOLS.md` + operator scoping in panel-api |
| Account failover UX | tools logout→switch patterns in ROLLOUT.md |

---

## 16. Immediate next actions (start order)

1. ~~Confirm Section 14 defaults~~ ✅  
2. ~~README + Phase 1 skeleton~~ ✅  
3. ~~Phase 2 session verify + minimal extension~~ ✅  
4. ~~Phase 3 `/v4` proxy + DNR~~ ✅  
5. ~~Phase 4 multi-account RR + failover~~ ✅  
6. ~~Phase 5 hourly + daily limits~~ ✅  
7. **Phase 6:** browser cache + SERP prewarm + Refresh.

---

## 17. Status tracker

| Phase | Status | Notes |
|-------|--------|-------|
| 0 Plan + folder | **Done** | 2026-11-27 |
| 1 Schema + skeleton | **Done** | SQLite local + MySQL SQL; healthz on :8088 |
| 2 aMember verify | **Done** | `tb.*` mint + tm-check + ext popup |
| 3 Single-account proxy | **Done** | `/v4` + MOCK/real + DNR + checks |
| 4 Multi-account RR | **Done** | sticky + 401 failover A→B |
| 5 Limits | **Done** | hourly+daily AND + custom |
| 6 Cache | Next | |
| 7 Panel UI | Not started | |
| 8 QA | Done | fingerprint optional, telegram alerts, QA matrix |
| 9 Deploy | Not started | |
| 10 Reseller download/upload | Deferred | |

---

*Document owner: ToolsMandi Ahrefs Bar build*  
*Last updated: 2026-11-27*
