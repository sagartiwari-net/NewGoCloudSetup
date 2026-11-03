# Reseller tools inventory

Living list: kis reseller ko kaun sa tool / URL diya hai, aur kaise add hua.  
Naya tool / extra link add karo to **isi file mein row update karo**.

Server IP: `65.109.16.196`  
Owner public hosts: `*.upays.in` (legacy `*.gt4rents.com` same ports)  
Panel DB: MySQL `gt4rents`

## Model (sab resellers same rule)

| Share (ek process) | Alag (har website row) |
|--------------------|-------------------------|
| Go binary, port, systemd `gt4-<sub>` | Domain, cookie/account, limits, users, sessions, logs, Telegram, access button (`access_slug`) |

Ek tool = ek port. Reseller ka host usi port pe reverse-proxy hota hai; panel mein **alag website** + **apna mapped account**.

---

## Kaise add hota hai (methods)

| Method | Kab use | Repo pieces |
|--------|---------|-------------|
| **A. Friend wildcard / one vhost** | Poora domain `*.friend.com` isi box pe (SeoGroupBuy) | `deploy/nginx-seogbtools.server.conf` + `deploy/nginx-host-port.map.conf` + SQL |
| **B. Per-host aaPanel site** | Har subdomain alag site (ToolWaly, T4, AmzPremium) | DNS A → IP, aaPanel Static site → `127.0.0.1:<port>`, SSL LE |
| **C. Panel seed (auto)** | Panel API start pe rows ensure | `panel/panel-api/seed.go` (`ensureToolWalySites`, `ensureSeoGroupBuyExtraSites`, `ensureAmzPremiumSoftwareSites`, `ensureT4ToolzAhrefs`) |
| **D. One-shot SQL** | Pehli baar / seed se pehle live DB | `deploy/add-seogbtools-reseller.sql`, `deploy/add-seogbtools-extra-smrs-cgpt.sql`, `deploy/add-amzpremiumsoftware-reseller.sql` |

Nginx map (host → port) hamesha update: `deploy/nginx-host-port.map.conf` → server pe copy → `nginx -t && nginx -s reload`.

---

## 1. SeoGroupBuy

| Field | Value |
|-------|--------|
| Panel username | `SeoGroupBuy` |
| Domain | `seogbtools.com` |
| Routing | **Method A** — ek aaPanel site `*.seogbtools.com` → host-port map |
| Base catalog | **Method D** — `deploy/add-seogbtools-reseller.sql`: har `*.upays.in` website → `*.seogbtools.com` copy (`SeoGroupBuy <name>`), product `["seogbtools"]`, slug `sgb` + owner slug |
| Slug fix | `deploy/fix-seogbtools-access-slugs.sql` |
| Name fix | `deploy/fix-seogbtools-website-names.sql` |
| Assign | `operator_websites` → username `SeoGroupBuy` |

### Extra links (Nov 2026) — Semrush ×5, ChatGPT ×3

**Method C+D:** `deploy/add-seogbtools-extra-smrs-cgpt.sql` + `ensureSeoGroupBuyExtraSites` in `seed.go`.  
Product id abhi: **`5`**. Mapped account: **none** (jab ready ho tab panel se).  
Same ports as owner: Semrush `5141`, ChatGPT `5151`.

| Panel name | URL | Access slug | Port | Notes |
|------------|-----|-------------|------|-------|
| SeoGroupBuy Semrush | `https://smrs.seogbtools.com` | `sgbsmrs` | 5141 | Base copy from upays |
| SeoGroupBuy Semrush 2 | `https://smrs2.seogbtools.com` | `sgbsmrs2` | 5141 | Extra |
| SeoGroupBuy Semrush 3 | `https://smrs3.seogbtools.com` | `sgbsmrs3` | 5141 | Extra |
| SeoGroupBuy Semrush 4 | `https://smrs4.seogbtools.com` | `sgbsmrs4` | 5141 | Extra |
| SeoGroupBuy Semrush 5 | `https://smrs5.seogbtools.com` | `sgbsmrs5` | 5141 | Extra |
| SeoGroupBuy ChatGPT | `https://cgpt.seogbtools.com` | `sgbcgpt` | 5151 | Base copy from upays |
| SeoGroupBuy ChatGPT 2 | `https://cgpt2.seogbtools.com` | `sgbcgpt2` | 5151 | Extra |
| SeoGroupBuy ChatGPT 3 | `https://cgpt3.seogbtools.com` | `sgbcgpt3` | 5151 | Extra |

Baaki tools: same short subdomain as upays, e.g. Envato `https://envt.seogbtools.com` → port `5261`, slug `sgbenvt`. Full list = nginx map entries `*.seogbtools.com`.

**Isolation note (2026-11-03):** Envato cookie pehle owner `envt.upays.in` pe gir rahi thi; fix `envato-go-proxy` session `website_id` se load karta hai (`a0a27e4`). Deploy: `./deploy/build-one.sh envt && ./deploy/start-tool.sh envt` (git pull pehle successful hona chahiye).

---

## 2. ToolWaly

| Field | Value |
|-------|--------|
| Panel username | `ToolWaly` |
| Domain | `toolwaly.com` |
| How added | **Method B + C** — alag aaPanel sites; `ensureToolWalySites` in `seed.go` |
| Product placeholder | `["toolwaly"]` (aMember pe asli id badlo) |

| Panel name | URL | Port | Shares process with |
|------------|-----|------|---------------------|
| ToolWaly Ahrefs | `https://refs.toolwaly.com` | 5291 | `refs.upays.in` |
| ToolWaly Semrush | `https://smrs.toolwaly.com` | 5141 | `smrs.upays.in` |
| ToolWaly Canva | `https://cnva.toolwaly.com` | 4501 | `cnva.upays.in` |
| ToolWaly Grammarly | `https://grammarly.toolwaly.com` | 4911 | `grmm.upays.in` |

DNS: grey A records for `refs`, `smrs`, `cnva`, `grammarly` → `65.109.16.196`.  
Is reseller ko owner (`upays`/`gt4rents`) websites assign mat rakho — sirf in char rows.

---

## 3. AmzPremiumToolz (`amzpremiumsoftware.com`)

| Field | Value |
|-------|--------|
| Panel username | `AmzPremiumToolz` |
| Domain | `amzpremiumsoftware.com` |
| How added | **Method B + C + D** — `deploy/add-amzpremiumsoftware-reseller.sql`, `ensureAmzPremiumSoftwareSites`, nginx map + per-host (or friend) vhost |
| Product | `["amzpremium"]` |
| Accounts | SQL **does not** insert accounts — panel se cookie map karo |

| Panel name | URL | Access slug | Port | Owner twin |
|------------|-----|-------------|------|------------|
| AmzPremium ChatGPT | `https://cht.amzpremiumsoftware.com` | `amzcgpt` | 5151 | `cgpt.upays.in` |
| AmzPremium Claude | `https://cld.amzpremiumsoftware.com` | `amzclud` | 5171 | `clud.upays.in` |
| AmzPremium Jungle Scout | `https://jngl.amzpremiumsoftware.com` | `amzjngl` | 5211 | `jngl.upays.in` |
| AmzPremium eRank | `https://ernk.amzpremiumsoftware.com` | `amzernk` | 5191 | `ernk.upays.in` |
| AmzPremium SellerAmp | `https://seller.amzpremiumsoftware.com` | `amzsllr` | 5161 | `sllr.upays.in` |
| AmzPremium Helium Learning | `https://hlearn.amzpremiumsoftware.com` | `amzhlml` | 5221 | `hlml.upays.in` |
| AmzPremium Zik Analytics | `https://zik.amzpremiumsoftware.com` | `amzzik` | 5251 | `zknl.upays.in` |

Seed is reseller ko **sirf** `*.amzpremiumsoftware.com` websites pe rakhta hai (purani shared upays rows hata deta hai).

---

## 4. AmzToolsConsultant (automation)

| Field | Value |
|-------|--------|
| Credential | `AmzToolsConsultant` / provider `amztoolsconsultant` |
| Homepage job | `AmzToolsConsultant Home` (`job_kind=session`) — logs into members portal, stores PHPSESSID |
| Shared cookie tools | Helium10 Diamond + Platinum, SellerAmp, ChatGPT (`amzccg`), Freedom Ticket, Niche Scraper, Envato — same homepage cookie; Diamond is cookie-share source |
| Separate cookie | **Keepa** (`amzckeepa` → browseanywhere) — own Capture path |
| How jobs appear | Panel API seed `ensureAmzToolsConsultantJobs` on start (does not touch other resellers) |

Restart panel-api after deploy to create any missing tool jobs. Interval copies the Home job (default 22 min).

---

## 5. T4 Toolz

| Field | Value |
|-------|--------|
| Domain | `t4toolz.com` |
| How added | **Method B + C** — `ensureT4ToolzAhrefs` in `seed.go` |
| Product | `["t4toolz"]` |

| Panel name | URL | Access slug | Port | Shares |
|------------|-----|-------------|------|--------|
| T4 Toolz Ahrefs | `https://ah2.t4toolz.com` | `t4refs` | 5291 | Ahrefs / `refs.upays.in` |

---

## 6. SemrushToolz / AmzPremiumToolz (legacy shared rows)

Purana seed kabhi-kabhi reseller ko **owner** websites pe assign karta tha (alag domain se pehle). Ab preferred: alag friend domain (upar sections).

| Reseller | Seed-linked tool names (legacy) | Prefer instead |
|----------|----------------------------------|----------------|
| SemrushToolz | Semrush, ChatGPT, Envato, Ubersuggest, Helium 10 | Apna domain + Method B jab ready ho |
| AmzPremiumToolz | Helium 10, ChatGPT, Claude AI | Section 3 `amzpremiumsoftware.com` |

---

## Checklist — naya reseller tool add karte waqt

1. `tools.json` / owner port confirm  
2. `deploy/nginx-host-port.map.conf` mein friend host → same port  
3. DNS grey + aaPanel (Method A ya B) + SSL  
4. Panel website row: unique `domain`, unique `access_slug`, product ids, reseller `operator_websites`  
5. Seed ya SQL idempotent rakho (dobara run safe)  
6. **Mapped account** isi website pe — owner cookie mat share karo  
7. Is file mein table update  
8. `git pull` conflict (local nginx map) pehle stash/commit, phir build tool

Related how-to (older, ToolWaly-focused): `RESELLERS.md`
