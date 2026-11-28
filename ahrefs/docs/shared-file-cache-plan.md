# Shared File Cache Plan (URLs + Keywords)

**Status:** LOCAL READY — **packed shards + compact + zstd+dict + O(1) hot LRU**; next = push to Hetzner `65.109.16.196`.  
**Goal:** cut Ahrefs official upstream hits → fewer 429s → higher uptime, shared across all users/resellers.  
**Server disk budget (locked):** **35 GB domains/URLs** + **15 GB keywords** = **50 GB** total → **crore-scale** entries.  
**Lookup:** **O(1)** hash→1 shard→map + in-memory hot cache (~1 ms Hits) — never scan crores.  
**Deploy home:** same tools server as `refs` / panel; cache dir `/var/lib/toolsmandi/ahrefs-cache`.

---

## 1. Why

Today:

- Durable server cache table `ahrefs_bar_server_cache` exists but is **unused** (0 rows).
- Only short **in-memory** v4 cache (≈15 min domain / ≈60 min keyword) — lost on restart.
- Browser IndexedDB (`tm-cache.js`) exists (72h TTL, soft cap ~500 domains) but is per-browser only.
- Usage reality (sample): **73 unique keywords → 16k+ calls**, mostly repeats + many **429**s. Keywords are **billable (1 credit)**; domain header is cheap/free.

Shared backup means: one user’s fetch helps every other user on the same URL/keyword.

---

## 2. What we store (UI fields only — nothing extra)

Only metrics the toolbar already shows. No Content / Indexability / headers / raw Ahrefs dumps.

### 2.1 URL / page entry

**Key:** normalized exact URL (and optionally a separate domain-only key for host-level reuse).

**Domain block**

| Field | UI label |
|-------|----------|
| `dr` | DR (Domain Rating) |
| `ar` | AR (Ahrefs Rank) |
| `bl` | BL (Backlinks) |
| `rd` | RD (Referring domains) |
| `kw` | KW (Organic keywords) |
| `st` | ST (Search traffic) |
| `tv` | Traffic value ($) |
| `paid_kw` | Paid keywords (optional; Metrics panel) |
| `paid_ads` | Ads count (optional) |
| `paid_traffic` | Paid traffic (optional) |
| `paid_cost` | Paid cost $ (optional) |

**Page block**

| Field | UI label |
|-------|----------|
| `ur` | UR (URL Rating) |
| `bl` | BL |
| `rd` | RD |
| `kw` | KW |
| `st` | ST |
| `tv` | Traffic value ($) |
| `words` | Words (SERP row; when present) |
| `paid_*` | Same optional paid fields as domain |

**Meta**

| Field | Purpose |
|-------|---------|
| `fetched_at` | Unix ms / RFC3339 — TTL + “current fetch” |
| `source` | `upstream` \| `refresh` |

### 2.2 Keyword entry

**Key:** `normalized_keyword` + `country` (e.g. `us`).

| Field | UI label |
|-------|----------|
| `keyword` | Keyword string |
| `country` | Country / locale |
| `sv` | SV (Search volume) |
| `kd` | KD (Difficulty) |
| `cpc` | CPC |
| `tp` | Traffic potential (when API provides; under-query bar) |
| `clicks` | Clicks (when API provides) |
| `fetched_at` | Meta |
| `source` | Meta |

**Do not store:** SERP HTML, screenshots, full Ahrefs JSON trees, Content/Social/Images tabs, Web Vitals (unless later product asks).

---

## 3. Storage: files on disk — not the panel database

- **Do not** put this corpus in SQLite/MySQL panel DB (avoid DB bloat).
- Shared folder on the bar-api / ToolsMandi server, readable by the proxy for **all** users and resellers (one global pool → max rate-limit savings).

### 3.1 Layout (locked — packed shards for crore density)

One-file-per-key wastes ~4 KiB/entry on ext4 → only **lakhs** in 50 GB.  
**Packed:** 256 shard files per pool; many compact records inside each zstd blob → **crores**.

```text
/var/lib/toolsmandi/ahrefs-cache/
  urls/
    ab.zst                                # ALL keys whose sha256 starts with ab
  keywords/
    cd.zst                                # ALL keyword keys whose sha256 starts with cd
  dict/
    url-v1.zdict                          # optional later — denser still
    keyword-v1.zdict
  meta/
    VERSION                               # 2-packed-lru
```

Lookup: hash(key) → open one shard → map get. **O(1)** path; no full-corpus scan on read.

### 3.2 Compression

| Method | Role |
|--------|------|
| **zstd + shared dictionary** | **Primary** — best density for repetitive metric JSON |
| zstd alone | Fallback if dict training not ready |
| brotli / gzip | Not preferred for hot path (slower or worse balance) |

Compact JSON field names (`dr`,`ur`,…) keep payloads ~200–300 B before compression.

### 3.3 Capacity targets (packed compact + **zstd shared dictionary — on now**)

| Pool | Cap | Approx with zstd+dict |
|------|-----|------------------------|
| URLs | **35 GB** | **~tens of crore** compact entries |
| Keywords | **15 GB** | **~several crore** |
| **Total** | **50 GB** | crore-scale shared corpus |

Dicts live at `dict/url-v1.zdict` + `dict/keyword-v1.zdict` (trained on open if missing).

Per-entry on disk is tens of bytes (toolbar fields only), not 4 KiB files.

**When over budget — least-recently-USED (not oldest write):**

1. Each record stores `u` = last **Get** time (LRU touch).
2. Cap check after Put; if pool `>` GB limit → drop entries with **oldest `u`** until under cap.
3. Frequently used URLs/keywords stay; cold ones are replaced.
4. URL pool and keyword pool never steal each other’s GB.

---

## 4. Layered read path (Markhor-style)

```text
Request (URL or keyword)
  │
  ├─1─ Browser IndexedDB (10–15k URL LRU, ~5–10k keyword LRU)
  │      HIT → paint immediately (0 credit)
  │
  ├─2─ Server file cache (shared folder)
  │      HIT → return to extension → also write browser cache
  │
  ├─3─ Upstream Ahrefs (official)
  │      MISS → fetch → write server file + browser
  │
  └─4─ Explicit "Current / Refresh / Fresh"
         Bypass 1+2 → upstream → overwrite server file + browser
```

### 4.1 Browser tier

- Keep using IndexedDB (not `chrome.storage.local` — that one is ~**10 MB** hard quota).
- Raise soft caps to ~**10–15k URLs** and ~**5–10k keywords** (still well under typical IDB quota).
- On popup/SERP: prefer local HIT; background-reconcile with server optional later.

### 4.2 Server tier

- Wire proxy (`tbGetHeaderV3`, domain metrics, `tbGetKeywordStatsV3`) to file cache.
- Response headers (or JSON meta): `HIT_BROWSER` / `HIT_SERVER` / `MISS` / `FRESH` / `DEDUP` for debugging.
- **Inflight dedupe:** concurrent identical keys → one upstream call (critical for 100–200 users).

### 4.3 Auth unchanged

File cache is **data**; access still requires valid portal session (`panel_session` / future aMember cookie → `tb.*`). Cached body is not a substitute for login.

---

## 5. Fast lookup & concurrency (**O(1)** / ~1 ms — implemented)

- Never scan “crores of URLs”.
- Key = sha256(normalized) → **exactly one** of 256 shard files → map get (**O(1)**).
- **Hot LRU** (~100k entries in process memory): repeat Hits skip disk → typically **well under 1 ms**.
- Cold miss on a shard loads that shard once into memory; further keys in same shard are map hits.
- 100–200 concurrent users: bottleneck remains **Ahrefs rate limits**, not cache lookup.

### Normalization rules (must be stable)

**URL:** lowercase host; strip default ports; strip tracking params (configurable); consistent trailing slash policy; prefer full page URL for page metrics, registrable/host key for domain-only reuse.

**Keyword:** trim; lowercase; collapse internal spaces; country code lowercase.

---

## 6. TTL & freshness

- Default align with reseller setting `cache_ttl_hours` (currently **72h**) for “serve from cache without upstream”.
- Stale-while-revalidate optional later (serve stale + async refresh).
- User **Refresh / Current fetch** always bypasses TTL and updates the shared file.

---

## 7. Multi-tenant / reseller policy

**Product decision (current plan):** one **global** shared file cache for all resellers/users so rate-limit savings are maximized.

If later isolation is required: prefix keys by `reseller_id` (reduces hit rate — avoid unless legally/product-required).

---

## 8. Ops & config (implement-time)

Suggested env / settings (names TBD):

| Setting | Intent |
|---------|--------|
| `AHREFS_BAR_FILE_CACHE_DIR` | Root folder |
| `AHREFS_BAR_FILE_CACHE_URL_GB` | Cap for URLs/domains (default **35**) |
| `AHREFS_BAR_FILE_CACHE_KW_GB` | Cap for keywords (default **15**) |
| `AHREFS_BAR_FILE_CACHE_TTL_HOURS` | Default 72 |
| Enable/disable file cache | Kill switch |

Metrics to expose later: hit rate, upstream calls saved, disk used, eviction counts, 429 rate before/after.

Backup: copy/rsync the folder; dict files must travel with data.

---

## 9. Implementation status

| Step | Status |
|------|--------|
| 1. Packed shard writer/reader (256×`.zst`/pool, 35/15 GB caps) | **Done (local)** |
| 1b. **Compact toolbar fields only** on disk (`dr`/`sv`/…), expand on read | **Done (local)** |
| 1c. **LRU eviction by last Get** (hot keys kept) | **Done (local)** |
| 2. Proxy integrate Header / Url / KeywordStats → `HIT_SERVER` | **Done (local)** |
| 3. Extension IDB caps raise | Pending |
| 4. Fresh/Refresh overwrites file | Fresh bypasses file; successful upstream Put — **Done** |
| 5. Cap eviction (**least-recently-used**, not oldest write) | **Done** |
| 6. Shared zstd dictionary (`url-v1` / `keyword-v1`) | **Done (train on open)** |
| 6b. O(1) hash→shard + hot LRU (~1 ms) | **Done** |
| 7. Server disk dirs + env on production | **Next (before push)** |

### Server prep (before push)

```bash
sudo mkdir -p /var/lib/toolsmandi/ahrefs-cache/{urls,keywords,dict,meta}
sudo chown -R <bar-api-user>:<bar-api-user> /var/lib/toolsmandi/ahrefs-cache

export AHREFS_BAR_FILE_CACHE=1
export AHREFS_BAR_FILE_CACHE_DIR=/var/lib/toolsmandi/ahrefs-cache
export AHREFS_BAR_FILE_CACHE_URL_GB=35
export AHREFS_BAR_FILE_CACHE_KW_GB=15
export AHREFS_BAR_FILE_CACHE_TTL_HOURS=72
```

Local default dir: `ahrefs/server/data/ahrefs-file-cache`.

---

## 10. Related code (today)

| Piece | Role |
|-------|------|
| `ahrefs/extension/tm-cache.js` | Browser IDB entity cache |
| `ahrefs/server/proxy_v4.go` | In-memory v4 cache + inflight dedupe |
| `ahrefs_bar_server_cache` | DB table present, unused — prefer **files** instead of filling this for the big corpus |
| Reseller `cache_ttl_hours` | Already 72 — reuse as policy |

---

## 11. Decision summary

| Topic | Decision |
|-------|----------|
| Apply now? | **Yes** — implement locally, then push with server dirs |
| DB for corpus? | **No** — shared folder + compressed files |
| Compression | **zstd + shared dictionary** (trained on open) |
| Lookup | **O(1)** hash→shard→map + hot LRU (~1 ms) |
| Space | **35 GB URLs + 15 GB keywords** → crore-scale (packed shards) |
| Fields | **Toolbar metrics only** (domain/page + keyword SV/KD/CPC/…) |
| Full / replace | **Least-recently-used** (last Get); hot keys stay |
| Browser | **10–15k URLs**, keywords LRU alongside |
| Shared across resellers? | **Yes** (global pool) |
| Fresh fetch | User Refresh → upstream + overwrite entry |

---

*Written for ToolsMandi Ahrefs Bar — revisit after current stability fixes.*
